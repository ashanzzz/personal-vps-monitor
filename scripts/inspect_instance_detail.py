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
    "Origin": "https://dash.lazycat.wiki",
    "Referer": "https://dash.lazycat.wiki/",
    "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/126.0 Safari/537.36"
}

r = requests.get("https://dash.lazycat.wiki/api/instances/11433", headers=headers, proxies=proxies, timeout=10)
print("status:", r.status_code)
data = r.json()
inst = data.get("instance") or data
print("Keys in instance:", list(inst.keys()))
print("port_mappings count:", len(inst.get("port_mappings") or []))
if inst.get("port_mappings"):
    print("Sample port_mapping:", inst.get("port_mappings")[0])
print("port_limit:", inst.get("port_limit"))
print("host_public_ip:", inst.get("host_public_ip") or inst.get("nat_public_ip") or inst.get("ip"))
