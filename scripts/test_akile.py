import json
import requests

with open('data/tokens.json', 'r', encoding='utf-8') as f:
    cfg = json.load(f)

token = cfg.get('akile_token', '').strip().strip('"').strip("'")
proxy_url = cfg.get('proxy_url')
proxies = {'http': proxy_url, 'https': proxy_url} if proxy_url else None

print(f"Token length: {len(token)}, prefix: {token[:20]}...")

headers = {
    "Authorization": token,
    "token": token,
    "Content-Type": "application/json",
    "Origin": "https://akile.ai",
    "Referer": "https://akile.ai/",
    "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/126.0 Safari/537.36",
    "Accept": "application/json, text/plain, */*"
}

try:
    r = requests.post("https://api.akile.ai/api/v1/pushshop/GetPushProductList", json={"page_num": 1, "page_size": 10}, headers=headers, proxies=proxies, timeout=10)
    print("With token pushshop:", r.status_code, r.text[:200])
except Exception as e:
    print("pushshop error:", e)

try:
    r2 = requests.post("https://api.akile.ai/api/v1/server/GetServerV2List", json={}, headers=headers, proxies=proxies, timeout=10)
    print("With token GetServerV2List:", r2.status_code, r2.text[:200])
except Exception as e:
    print("GetServerV2List error:", e)

# Test without token for pushshop
try:
    r3 = requests.post("https://api.akile.ai/api/v1/pushshop/GetPushProductList", json={"page_num": 1, "page_size": 10}, headers={"Content-Type": "application/json"}, proxies=proxies, timeout=10)
    print("Without token pushshop:", r3.status_code, r3.text[:200])
except Exception as e:
    print("Without token pushshop error:", e)
