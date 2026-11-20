import json
import requests

with open('data/tokens.json', 'r', encoding='utf-8') as f:
    cfg = json.load(f)

proxies = {'http': cfg.get('proxy_url'), 'https': cfg.get('proxy_url')} if cfg.get('proxy_url') else None

print("=== 1. ACCK SERVER LIST & DETAILS ===")
acck_token = cfg.get('acck_token')
if acck_token:
    headers = {
        'Authorization': f'Bearer {acck_token}',
        'Content-Type': 'application/json',
        'Origin': 'https://acck.io',
        'Referer': 'https://acck.io/',
        'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/126.0 Safari/537.36'
    }
    r = requests.get('https://acck.io/api/auth/user/server/list', headers=headers, proxies=proxies, timeout=10)
    print("Status:", r.status_code)
    try:
        data = r.json()
        servers = data.get('servers') or data.get('data') or data
        print(f"Total servers found: {len(servers)}")
        for s in servers:
            print("--- Server ---")
            print("Keys:", list(s.keys()))
            print(json.dumps(s, ensure_ascii=False, indent=2))
            # Test if there is a detail endpoint or password endpoint
            sid = s.get('id') or s.get('server_id')
            if sid:
                for sub in [f'/auth/user/server/{sid}', f'/auth/user/server/detail/{sid}', f'/auth/user/server/info?id={sid}', f'/auth/user/server/{sid}/password', f'/auth/user/server/control/{sid}']:
                    r_sub = requests.get(f'https://acck.io/api{sub}', headers=headers, proxies=proxies, timeout=5)
                    if r_sub.status_code != 404:
                        print(f"Subpath {sub} -> HTTP {r_sub.status_code}: {r_sub.text[:200]}")
    except Exception as e:
        print("Error parsing acck:", e)

print("\n=== 2. LAZYCAT INSTANCE DETAILS ===")
lz_token = cfg.get('lazycat_token')
if lz_token:
    headers = {
        'Authorization': f'Bearer {lz_token}',
        'Content-Type': 'application/json',
        'Origin': 'https://dash.lazycat.wiki',
        'Referer': 'https://dash.lazycat.wiki/',
        'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/126.0 Safari/537.36'
    }
    r = requests.get('https://dash.lazycat.wiki/api/instances/11433', headers=headers, proxies=proxies, timeout=10)
    print("Status:", r.status_code)
    try:
        inst = r.json().get('instance') or r.json()
        # Look for password, ssh, terminal info
        print(json.dumps(inst, ensure_ascii=False, indent=2))
    except Exception as e:
        print("Error:", e)
