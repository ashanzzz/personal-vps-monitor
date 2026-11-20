"""
Geo-Location & Google 'SongZhong' (送中 / China-routing) Detector.

Detects whether an IP or proxy outbound is:
  1. 'SongZhong' (Google incorrectly routes IP to Google China / google.cn or flags location as CN)
  2. Native Residential vs Datacenter Hosting IP
  3. YouTube Premium / Google Services available region
  4. Actual ISP and Country information
"""
from __future__ import annotations

import asyncio
import logging
import re
from typing import Any, Dict, Optional

import httpx

logger = logging.getLogger(__name__)


async def check_ip_location(ip: str, timeout: float = 6.0) -> Dict[str, Any]:
    """
    Query open GeoIP database for detailed IP country, city, ISP, ASN, and hosting flag.
    """
    if not ip:
        return {"error": "Empty IP"}

    url = f"http://ip-api.com/json/{ip}?fields=status,message,country,countryCode,region,regionName,city,zip,lat,lon,timezone,isp,org,as,mobile,proxy,hosting,query"
    try:
        async with httpx.AsyncClient(timeout=timeout) as client:
            resp = await client.get(url)
            if resp.status_code == 200:
                data = resp.json()
                if data.get("status") == "success":
                    is_hosting = bool(data.get("hosting"))
                    is_proxy = bool(data.get("proxy"))
                    is_native = not is_hosting and not is_proxy
                    return {
                        "ip": ip,
                        "country": data.get("country", ""),
                        "country_code": data.get("countryCode", ""),
                        "city": data.get("city", ""),
                        "region": data.get("regionName", ""),
                        "isp": data.get("isp", ""),
                        "org": data.get("org", ""),
                        "as": data.get("as", ""),
                        "is_native": is_native,
                        "is_hosting": is_hosting,
                        "is_proxy": is_proxy,
                        "ip_type": "住宅家宽 (Residential)" if is_native else "机房商用 (Datacenter)",
                    }
    except Exception as e:
        logger.debug("ip-api query failed for %s: %s", ip, e)

    # Fallback to ip.sb
    try:
        async with httpx.AsyncClient(timeout=timeout) as client:
            resp = await client.get(f"https://api.ip.sb/geoip/{ip}")
            if resp.status_code == 200:
                data = resp.json()
                return {
                    "ip": ip,
                    "country": data.get("country", ""),
                    "country_code": data.get("country_code", ""),
                    "city": data.get("city", ""),
                    "region": data.get("region", ""),
                    "isp": data.get("isp", ""),
                    "org": data.get("organization", ""),
                    "as": f"AS{data.get('asn', '')}",
                    "is_native": False,
                    "is_hosting": True,
                    "ip_type": "标准公网",
                }
    except Exception as e:
        return {"ip": ip, "error": f"GeoIP probe failed: {e}"}

    return {"ip": ip, "error": "GeoIP service unavailable"}


async def check_google_songzhong(proxy_url: Optional[str] = None, timeout: float = 8.0) -> Dict[str, Any]:
    """
    Actively probe Google's actual routing for this network or proxy.
    Returns:
      - is_songzhong: True if Google flags this IP as China (redirects to google.cn or shows location: CN)
      - google_region: detected country/region by Google
      - timeline_allowed: whether Google Maps Timeline is enabled (disabled on SongZhong IPs)
      - youtube_region: detected YouTube country code
    """
    headers = {
        "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36",
        "Accept-Language": "en-US,en;q=0.9",
    }

    is_songzhong = False
    google_region = "Unknown"
    reasons = []

    try:
        async with httpx.AsyncClient(timeout=timeout, proxy=proxy_url, follow_redirects=False, verify=False) as client:
            # 1. Test google.com/ncr and inspect response
            r1 = await client.get("https://www.google.com/ncr", headers=headers)
            loc_header = r1.headers.get("location") or ""
            if "google.cn" in loc_header.lower():
                is_songzhong = True
                reasons.append("Google NCR 重定向到了 google.cn")

            # 2. Check Google search result page for footer region
            r2 = await client.get("https://www.google.com/search?q=ip", headers=headers)
            body = r2.text.lower()
            if "google.cn" in body:
                is_songzhong = True
                reasons.append("搜索页面包含 google.cn 域名标记")

            # Look for country code pattern in google HTML or cookies
            m_cc = re.search(r'data-country="([a-zA-Z]{2})"', body)
            if m_cc:
                google_region = m_cc.group(1).upper()
            elif "china" in body and "location" in body:
                google_region = "CN"
                is_songzhong = True
                reasons.append("页面位置声明包含 China")
            elif "hong kong" in body:
                google_region = "HK"
            elif "united states" in body:
                google_region = "US"
            elif "japan" in body:
                google_region = "JP"

            # 3. Test Google Maps Timeline (blocked in CN)
            timeline_allowed = True
            try:
                r3 = await client.get("https://www.google.com/maps/timeline", headers=headers)
                if r3.status_code in (403, 451) or "not available in your region" in r3.text.lower():
                    timeline_allowed = False
                    if not is_songzhong:
                        is_songzhong = True
                        reasons.append("Google 时间线 Timeline 在此 IP 被封锁 (典型送中特征)")
            except Exception:
                pass

            # 4. Check YouTube region
            youtube_region = ""
            try:
                ry = await client.get("https://www.youtube.com/premium", headers=headers)
                if ry.status_code == 200:
                    my = re.search(r'"countryCode":"([A-Z]{2})"', ry.text)
                    if my:
                        youtube_region = my.group(1)
            except Exception:
                pass

            if is_songzhong:
                status_text = "已送中 🇨🇳⚠️ (Google判定为中国区，部分服务受限)"
            else:
                status_text = f"正常未送中 ✅ (Google定位: {google_region or '国际'})"

            return {
                "tested": True,
                "is_songzhong": is_songzhong,
                "status_text": status_text,
                "google_region": google_region,
                "youtube_region": youtube_region,
                "timeline_allowed": timeline_allowed,
                "reasons": reasons,
            }
    except Exception as e:
        return {
            "tested": False,
            "is_songzhong": False,
            "status_text": f"测试失败 ({e})",
            "google_region": "Unknown",
            "error": str(e),
        }
