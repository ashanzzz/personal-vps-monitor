import json
import requests

with open('data/tokens.json', 'r', encoding='utf-8') as f:
    cfg = json.load(f)

token = cfg.get('lazycat_token', '').strip().strip('"').strip("'")
proxy_url = cfg.get('proxy_url')
proxies = {'http': proxy_url, 'https': proxy_url} if proxy_url else None

headers = {
    "Authorization": f"Bearer {token}",
    "Content-Type": "application/json",
    "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/126.0 Safari/537.36"
}

r = requests.get("https://dash.lazycat.wiki/api/instance-market/listings?page=1&pageSize=5", headers=headers, proxies=proxies, timeout=10)
data = r.json()
print("Keys:", list(data.keys()))
if "items" in data:
    print("Items count:", len(data["items"]))
    if len(data["items"]) > 0:
        print("First item keys:", list(data["items"][0].keys()))
        print("First item sample:", json.dumps(data["items"][0], ensure_ascii=False, indent=2))
if "total" in data:
    print("Total:", data["total"])
