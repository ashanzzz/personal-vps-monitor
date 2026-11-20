import json
import os
import subprocess

with open('data/tokens.json', 'r', encoding='utf-8') as f:
    cfg = json.load(f)

token = cfg.get('lazycat_token', '').strip().strip('"').strip("'")
proxy_url = cfg.get('proxy_url', 'http://192.168.8.11:7890')

script_path = r"C:\Users\ashan\.gemini\antigravity\brain\4ba73e78-1914-4090-a86c-811ff3bdc661\scratch\lazycat_webshell\01_access\lzterm.py"

env = os.environ.copy()
env["PROXY_URL"] = proxy_url

cmd = ["python", script_path, token, "head -n 40 /root/nodes.json", "11433"]
res = subprocess.run(cmd, capture_output=True, text=True, timeout=20, env=env)
lines = res.stdout.splitlines()
print(f"Total lines: {len(lines)}")
for ln in lines[-25:]:
    print(ln)
