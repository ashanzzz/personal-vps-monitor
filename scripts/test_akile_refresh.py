import json
import requests

with open('data/tokens.json', 'r', encoding='utf-8') as f:
    cfg = json.load(f)

token = cfg.get('akile_token', '').strip().strip('"').strip("'")
proxy_url = cfg.get('proxy_url')
proxies = {'http': proxy_url, 'https': proxy_url} if proxy_url else None

headers = {
    "Authorization": token,
    "Origin": "https://akile.ai",
    "Referer": "https://akile.ai/",
    "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/126.0 Safari/537.36",
    "Accept": "application/json, text/plain, */*"
}

try:
    r = requests.get("https://api.akile.ai/api/v1/user/refreshToken", headers=headers, proxies=proxies, timeout=10)
    print("refresh result:", r.status_code, r.text)
except Exception as e:
    print("refresh err:", e)
