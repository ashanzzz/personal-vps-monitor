import json
import os
import subprocess

with open('data/tokens.json', 'r', encoding='utf-8') as f:
    cfg = json.load(f)

token = cfg.get('lazycat_token', '').strip().strip('"').strip("'")
proxy_url = cfg.get('proxy_url', 'http://192.168.8.11:7890')
print(f"Testing lzterm. Token length: {len(token)}, proxy: {proxy_url}")

script_path = r"C:\Users\ashan\.gemini\antigravity\brain\4ba73e78-1914-4090-a86c-811ff3bdc661\scratch\lazycat_webshell\01_access\lzterm.py"

env = os.environ.copy()
env["PROXY_URL"] = proxy_url

cmd = ["python", script_path, token, "uname -a; uptime; id; ip -brief addr", "11433"]
try:
    res = subprocess.run(cmd, capture_output=True, text=True, timeout=20, env=env)
    print("--- STDOUT ---")
    print(res.stdout)
    print("--- STDERR ---")
    print(res.stderr)
    print("Returncode:", res.returncode)
except Exception as e:
    print("Error:", e)
