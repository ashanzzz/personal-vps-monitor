import json
import requests

with open('data/tokens.json', 'r', encoding='utf-8') as f:
    cfg = json.load(f)

proxy_url = cfg.get('proxy_url')
proxies = {'http': proxy_url, 'https': proxy_url} if proxy_url else None

# Test store without token
r = requests.get("https://api.akile.ai/api/v1/store/GetVpsStoreV3?areaId=1", proxies=proxies, timeout=10)
print("Akile store without token:", r.status_code, r.text[:300])

# Test store with token
token = cfg.get('akile_token', '').strip().strip('"').strip("'")
r2 = requests.get("https://api.akile.ai/api/v1/store/GetVpsStoreV3?areaId=1", headers={"Authorization": token}, proxies=proxies, timeout=10)
print("Akile store with token:", r2.status_code, r2.text[:300])
