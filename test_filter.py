import re

def is_buying_post(title: str) -> bool:
    t = title.strip()
    # Explicit prefix tags
    for tag in ["「收」", "【收】", "[收]", "收:", "收：", "收个", "求购", "收购", "收一台"]:
        if tag in t:
            return True
    # If "收" is in title and "出" is not in title
    if "收" in t and "出" not in t:
        return True
    # If both "收" and "出" in title, see context (e.g. "350收个...400收")
    if "收" in t and "出" in t:
        if re.search(r'\d+收', t) or "收个" in t:
            return True
    return False

tests = [
    "「收」 懒猫云 台湾家宽hinet-2 mini 1折",
    "出懒猫- 新加坡 StarHubnano小杯家宽 循环三折",
    "明盘 350收个懒猫云0.1折日本狗妈 免费的400收",
    "「出」摆摊出小🐔 小小🐔（懒猫/taxi sg 10🔪）",
    "出懒猫云BIGLOBE Basic永久免费",
    "【出】40包push出 懒猫 GLBB 5折小杯",
    "[怒了]收懒猫全球家宽1折或0.1或免费大杯",
    "500 出个懒猫云 0.1折全球家宽大杯",
    "「出」总价90出 懒猫 3折 SB SoftBank Nano 家宽 4.5元/月 200 GB 150Mbps",
    "「出」总价60出 懒猫 3折 BIGLOBE Mini 动态家宽 6元/月 400 GB 300 Mbps（总价30出5折 10元/月） 剩余时间30天",
    "[180出]懒猫云日本家宽大杯 BigLobe Basic 大地球 免费"
]

for t in tests:
    is_buy = is_buying_post(t)
    print(f"[{'BUY (IGNORE)' if is_buy else 'SELL (KEEP)'}] {t}")
