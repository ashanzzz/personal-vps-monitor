"""
Test intent classification on sample posts.
"""
import re

TEST_TITLES = [
    ("爽翻了，Blossom的NetWest家宽免费升级配置:1T流量变为2T，cpu免费升级", False),
    ("[DE 拼车] AKKO 和 Ciallo 拼车，都是配好了UK的伪家宽落地", False),
    ("[NQ] 蔭雲 香港家宽 ＄10/月", False), # merchant ad or test?
    ("快讯！DMIT断网，家宽连不上，手机流量不受影响", False),
    ("这两款家宽有什么区别啊", False),
    ("出 66云 gtt伪家宽 200半年", True),
    ("人找车，Novixlink COX 家宽落地，找长期稳定的车", False),
    ("「出」总价60出 懒猫 3折 BIGLOBE Mini 动态家宽 6元/月 400 GB 300 Mbps", True),
    ("【出】150总价出懒猫云DE9929大杯 1折 月续费2元 2027.11.24到期", True),
    ("出懒猫- 新加坡 StarHubnano小杯家宽 循环三折", True),
    ("500 出个懒猫云 0.1折全球家宽大杯", True),
    ("[180出]懒猫云日本家宽大杯 BigLobe Basic 大地球 免费", True),
]

SELL_TRIGGERS = ["出", "出个", "明盘", "降价", "自砍", "骨折", "转", "出让", "退坑", "包push", "包转移"]
BUY_TRIGGERS = ["收", "求", "收个", "求购", "收购", "收一台", "想收", "人找车", "找车"]
NON_TRADE_TRIGGERS = ["拼车", "快讯", "测评", "区别", "怎么样", "请教", "咨询", "讨论", "爽翻了", "翻车", "升级", "降权", "断网", "失联"]

def is_strictly_selling(title: str) -> bool:
    t = title.strip()
    
    # 1. Non-trade / discussion / announcement filter
    for nt in NON_TRADE_TRIGGERS:
        if nt in t:
            return False
            
    # 2. Buy filter
    for b in BUY_TRIGGERS:
        if b in t and "出" not in t:
            return False
            
    # 3. Explicit sell check
    has_sell = False
    # Check prefixes or patterns
    if re.search(r'(?:^|[「【\[\s])(?:出|出个|明盘|转)(?:$|[」】\]\s\d])', t):
        has_sell = True
    elif re.search(r'\d+\s*(?:出|包push|元出|刀出)', t):
        has_sell = True
    elif any(trig in t for trig in ["包push", "自砍", "降价出", "明盘出", "平价出"]):
        has_sell = True
    elif t.startswith("出") or " 出 " in t or "【出】" in t or "「出」" in t or "[出]" in t:
        has_sell = True
        
    return has_sell

for title, expected in TEST_TITLES:
    actual = is_strictly_selling(title)
    status = "OK" if actual == expected else "MISMATCH"
    print(f"[{status}] Expected:{expected} Got:{actual} -> {title}")
