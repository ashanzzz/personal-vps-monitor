"""
NodeSeek & Forum Market Scanner with LVSS Valuation Model.
Directly delivers high-value bargain alerts to @ashan_notice_bot.
Strict 0-Emoji discipline, ultra-concise 1-3 lines, completely silent when no bargains found.
"""
from __future__ import annotations

import os
import re
import json
import logging
import subprocess
import urllib.parse
from datetime import datetime
import httpx

logging.basicConfig(level=logging.WARNING)

AUTH_FILE = "/opt/data/projects/akile-monitor/forum_auth.json"
SEEN_FILE = "/opt/data/projects/akile-monitor/seen_posts.json"
PROXY = os.environ.get("HTTPS_PROXY") or "http://192.168.8.11:7890"

SEARCH_KEYWORDS = ["懒猫", "家宽", "0.1折", "永久免费"]

# Intent triggers
BUY_TRIGGERS = ["收", "求", "收个", "求购", "收购", "收一台", "想收", "人找车", "找车"]
SOLD_TRIGGERS = ["已出", "无了", "封贴", "结贴", "出完", "已秒", "收到了", "已收", "暂停"]
NON_TRADE_TRIGGERS = [
    "拼车", "快讯", "测评", "区别", "怎么样", "请教", "咨询", "讨论",
    "爽翻了", "翻车", "升级", "降权", "断网", "失联", "挂了", "维护", "避坑"
]

NOTICE_CHAT_ID = "6213495524"


def get_notice_token() -> str:
    env_path = "/opt/data/profiles/notice/.env"
    if os.path.exists(env_path):
        try:
            with open(env_path, "r", encoding="utf-8") as f:
                for line in f:
                    if "TELEGRAM_BOT_TOKEN=" in line:
                        return line.strip().split("=", 1)[1].strip('"').strip("'")
        except Exception:
            pass
    return "9230381349:AAEXUfL0K_Bqg4K0QxWj3fXfVlU1l0K_Bqg"


def load_auth() -> dict:
    if os.path.exists(AUTH_FILE):
        try:
            with open(AUTH_FILE, "r", encoding="utf-8") as f:
                return json.load(f)
        except Exception:
            return {}
    return {}


def load_seen() -> set[str]:
    if os.path.exists(SEEN_FILE):
        try:
            with open(SEEN_FILE, "r", encoding="utf-8") as f:
                return set(json.load(f))
        except Exception:
            return set()
    return set()


def save_seen(seen_set: set[str]) -> None:
    l = list(seen_set)[-1000:]
    with open(SEEN_FILE, "w", encoding="utf-8") as f:
        json.dump(l, f, ensure_ascii=False)


def fetch_nodeseek_search(keyword: str, cookie_str: str) -> list[dict]:
    encoded_kw = urllib.parse.quote(keyword)
    url = f"https://www.nodeseek.com/search?q={encoded_kw}"
    cmd = [
        "curl", "-s", "-x", PROXY,
        "-H", "User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36",
        "-H", f"Cookie: {cookie_str}",
        url
    ]
    try:
        p = subprocess.run(cmd, capture_output=True, text=True, timeout=15)
        html = p.stdout
    except Exception:
        return []

    pattern = r'href="(/post-\d+-1)"[^>]*>(.*?)</a>'
    matches = re.findall(pattern, html)
    posts = []
    seen_href = set()
    for href, title_raw in matches:
        clean_t = re.sub(r'<[^>]+>', '', title_raw).strip()
        if not clean_t or clean_t.isdigit():
            continue
        if href in seen_href:
            continue
        seen_href.add(href)
        post_id = href.replace("/post-", "").replace("-1", "")
        posts.append({
            "id": post_id,
            "href": href,
            "url": f"https://www.nodeseek.com{href}",
            "title": clean_t,
        })
    return posts


def is_strictly_selling(title: str) -> bool:
    t = title.strip()
    for nt in NON_TRADE_TRIGGERS:
        if nt in t:
            return False
    for s in SOLD_TRIGGERS:
        if s in t:
            return False
    for b in BUY_TRIGGERS:
        if b in t and "出" not in t:
            return False

    has_sell = False
    if re.search(r'(?:^|[「【\[\s])(?:出|出个|明盘|转)(?:$|[」】\]\s\d])', t):
        has_sell = True
    elif re.search(r'\d+\s*(?:出|包push|元出|刀出)', t):
        has_sell = True
    elif any(trig in t for trig in ["包push", "自砍", "降价出", "明盘出", "平价出"]):
        has_sell = True
    elif t.startswith("出") or " 出 " in t or "【出】" in t or "「出」" in t or "[出]" in t:
        has_sell = True

    return has_sell


def evaluate_post(title: str) -> dict:
    if not is_strictly_selling(title):
        return {"score": 0, "grade": "IGNORE", "tags": [], "warnings": []}

    t_lower = title.lower()
    score = 55.0
    tags = []
    warnings = []

    # Residential / ISP
    if any(k in t_lower for k in ["家宽", "双isp", "大地球", "softbank", "biglobe", "starhub", "hinet"]):
        score += 20.0
        tags.append("住宅家宽双ISP")

    # Discount level
    if "0.1折" in t_lower or "0.1" in t_lower:
        score += 18.0
        tags.append("0.1折")
    elif "1折" in t_lower:
        score += 15.0
        tags.append("1折")
    elif "3折" in t_lower:
        score += 12.0
        tags.append("3折")

    # Renewal fee
    if "免费" in t_lower or "0元" in t_lower or "永久免费" in t_lower:
        score += 18.0
        tags.append("0元月供")
    elif any(f"{x}元" in t_lower for x in ["2", "3", "4", "4.5", "5", "6"]):
        score += 12.0
        tags.append("月供<=6元")

    # KDDI penalty (SLA 50%)
    if "kddi" in t_lower:
        score -= 30.0
        warnings.append("KDDI一区SLA仅50%")

    # Price estimation
    price_match = re.search(r'(\d+)\s*(?:出|包push|总价|明盘|元)', title)
    price = int(price_match.group(1)) if price_match else None
    if price:
        tags.append(f"标价¥{price}")
        if price <= 60:
            score += 10.0
        elif price > 350 and "免费" not in title:
            score -= 15.0

    score = min(100.0, max(0.0, score))

    if score >= 85:
        grade = "S级神机"
    elif score >= 75:
        grade = "A级优质"
    elif score >= 60:
        grade = "B级平价"
    else:
        grade = "D级避坑"

    return {
        "score": score,
        "grade": grade,
        "tags": tags,
        "warnings": warnings,
    }


def send_to_notice_bot(text: str) -> bool:
    token = get_notice_token()
    url = f"https://api.telegram.org/bot{token}/sendMessage"
    payload = {
        "chat_id": NOTICE_CHAT_ID,
        "text": text,
        "disable_web_page_preview": True
    }
    try:
        with httpx.Client(proxy=PROXY, timeout=10) as client:
            r = client.post(url, json=payload)
            return r.status_code == 200
    except Exception:
        return False


def scan_and_notify():
    # 晚上10点(22:00)至转天早上7点(07:00)静音保护
    current_hour = datetime.now().hour
    if current_hour >= 22 or current_hour < 7:
        return

    auth = load_auth()
    cookies = auth.get("nodeseek", {}).get("cookies", {})
    cookie_str = "; ".join([f"{k}={v}" for k, v in cookies.items()])

    seen = load_seen()
    all_deals = {}

    for kw in SEARCH_KEYWORDS:
        posts = fetch_nodeseek_search(kw, cookie_str)
        for p in posts:
            pid = p["id"]
            if pid not in all_deals:
                eval_res = evaluate_post(p["title"])
                if eval_res["grade"] != "IGNORE":
                    p.update(eval_res)
                    all_deals[pid] = p

    new_bargains = []
    for pid, deal in all_deals.items():
        if pid not in seen:
            seen.add(pid)
            if deal["score"] >= 75.0:
                new_bargains.append(deal)

    save_seen(seen)
    if not new_bargains:
        return

    new_bargains.sort(key=lambda x: x["score"], reverse=True)

    # Format 1-3 lines per bargain and send strictly to Ashan Notice Bot
    for d in new_bargains:
        tag_str = " / ".join(d["tags"]) if d["tags"] else "未明确参数"
        warn_str = f" [注意: {', '.join(d['warnings'])}]" if d["warnings"] else ""
        msg = (
            f"【NodeSeek捡漏巡检】[{d['grade']} | {int(d['score'])}分]\n"
            f"标题：{d['title']} ({tag_str}){warn_str}\n"
            f"直达：{d['url']}"
        )
        send_to_notice_bot(msg)


if __name__ == "__main__":
    scan_and_notify()
