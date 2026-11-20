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
inst = r.json().get("instance") or {}
pms = inst.get("port_mappings") or []
print(f"Total mappings: {len(pms)} / {inst.get('port_limit')}")
for p in pms:
    print(f"ID: {p.get('id')} | Proto: {p.get('protocol')} | Public: {inst.get('host_public_ip')}:{p.get('public_port')} -> Private: {p.get('private_port')} | Remark: {p.get('remark')}")
