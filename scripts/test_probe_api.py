import os
import subprocess

script_path = r"C:\Users\ashan\.gemini\antigravity\brain\4ba73e78-1914-4090-a86c-811ff3bdc661\scratch\lazycat_webshell\02_api\lazycat_probe_api.py"

cmd = ["python", script_path, "--residential"]
res = subprocess.run(cmd, capture_output=True, text=True, timeout=15)
print("--- STDOUT ---")
print(res.stdout[:500])
print("Returncode:", res.returncode)
