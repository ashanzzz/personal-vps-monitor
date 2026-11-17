"""
Forum Sentinel Watchdog for NodeSeek and LINUX DO.
Extracts live RSS and topics, filters with LVSS scoring, and notifies Telegram.
"""
from __future__ import annotations

import os
import json
import logging
import asyncio
import xml.etree.ElementTree as ET
import httpx

logger = logging.getLogger("forum_watchdog")
logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] %(message)s")

AUTH_FILE = "/opt/data/projects/akile-monitor/forum_auth.json"
CACHE_FILE = "/opt/data/projects/akile-monitor/seen_posts.json"
PROXY = os.environ.get("HTTPS_PROXY") or "http://192.168.8.11:7890"

# Target search keywords
KEYWORDS_VENDOR = ["懒猫", "lazycat", "akile", "acck", "dmit", "lisahost", "搬瓦工", "bandwagon"]
KEYWORDS_TRAFFIC = ["家宽", "双isp", "softbank", "biglobe", "0.1折", "1折", "免费", "9929", "cmin2", "4837"]
KEYWORDS_INTENT = ["出", "出个", "明盘", "降价", "自砍", "骨折"]


def load_auth() -> dict:
    if os.path.exists(AUTH_FILE):
        try:
            with open(AUTH_FILE, "r", encoding="utf-8") as f:
                return json.load(f)
        except Exception:
            return {}
    return {}


def load_seen() -> set[str]:
    if os.path.exists(CACHE_FILE):
        try:
            with open(CACHE_FILE, "r", encoding="utf-8") as f:
                return set(json.load(f))
        except Exception:
            return set()
    return set()


def save_seen(seen_set: set[str]) -> None:
    l = list(seen_set)[-500:]
    with open(CACHE_FILE, "w", encoding="utf-8") as f:
        json.dump(l, f)


async def check_nodeseek_rss() -> list[dict]:
    seen = load_seen()
    auth = load_auth()
    ns_cookies = auth.get("nodeseek", {}).get("cookies", {})

    headers = {
        "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36",
        "Referer": "https://www.nodeseek.com/",
    }

    hits: list[dict] = []
    try:
        async with httpx.AsyncClient(proxy=PROXY, headers=headers, cookies=ns_cookies, timeout=12) as client:
            r = await client.get("https://www.nodeseek.com/rss.xml", follow_redirects=True)
            if r.status_code != 200:
                logger.warning("NodeSeek RSS HTTP %s", r.status_code)
                return []

            root = ET.fromstring(r.text)
            items = root.findall(".//item")
            for it in items:
                title_node = it.find("title")
                link_node = it.find("link")
                pub_node = it.find("pubDate")
                desc_node = it.find("description")

                title = title_node.text if title_node is not None and title_node.text else ""
                link = link_node.text if link_node is not None and link_node.text else ""
                pub_date = pub_node.text if pub_node is not None and pub_node.text else ""
                desc = desc_node.text if desc_node is not None and desc_node.text else ""

                if not link:
                    continue

                post_id = link.rstrip("/").split("/")[-1].split("?")[0]
                if not post_id or post_id in seen:
                    continue

                title_lower = title.lower()
                desc_lower = desc.lower()
                combined = f"{title_lower} {desc_lower}"

                has_vendor = any(k in combined for k in KEYWORDS_VENDOR)
                has_feature = any(k in combined for k in KEYWORDS_TRAFFIC)
                has_intent = any(k in title_lower for k in KEYWORDS_INTENT)

                if (has_vendor and has_feature) or (has_vendor and has_intent):
                    hits.append({
                        "source": "NodeSeek",
                        "post_id": post_id,
                        "title": title,
                        "link": link,
                        "pub_date": pub_date,
                        "content_preview": desc[:150],
                    })
                    seen.add(post_id)

            save_seen(seen)
    except Exception as e:
        logger.error("Error checking NodeSeek RSS: %s", e)

    return hits


if __name__ == "__main__":
    hits = asyncio.run(check_nodeseek_rss())
    print(f"Watchdog scan complete. Discovered {len(hits)} matching target deals.")
    for h in hits:
        print(f"[{h['source']}] {h['title']} -> {h['link']}")
