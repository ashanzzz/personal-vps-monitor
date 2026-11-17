import json, subprocess, re

with open("/opt/data/projects/akile-monitor/forum_auth.json") as f:
    auth = json.load(f)

cookies = auth.get("nodeseek", {}).get("cookies", {})
pairs = [f"{k}={v}" for k, v in cookies.items()]
cookie_str = "; ".join(pairs)

cmd = [
    "curl", "-s", "-x", "http://192.168.8.11:7890",
    "-H", "User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36",
    "-H", f"Cookie: {cookie_str}",
    "https://www.nodeseek.com/search?q=%E6%87%92%E7%8C%AB"
]
p = subprocess.run(cmd, capture_output=True, text=True)
html = p.stdout

# Extract post items
# In NodeSeek SSR: post-list-item or similar
pattern = r'href="(/post-\d+-1)"[^>]*>(.*?)</a>'
matches = re.findall(pattern, html)
print(f"Total post matches: {len(matches)}")
seen = set()
for href, title in matches:
    clean_t = re.sub(r'<[^>]+>', '', title).strip()
    if href not in seen and clean_t and not clean_t.isdigit():
        seen.add(href)
        print(f"https://www.nodeseek.com{href} -> {clean_t}")
        if len(seen) >= 15:
            break
