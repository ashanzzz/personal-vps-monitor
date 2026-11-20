import requests

r = requests.get("https://api.akile.ai/api/v1/store/GetVpsStoreV3?areaId=1", timeout=10)
data = r.json()
areas = data.get("data", {}).get("areas", [])
print(f"Total areas: {len(areas)}")
total_plans = 0
for a in areas:
    area_name = a.get("area_name")
    nodes = a.get("nodes") or []
    for n in nodes:
        plans = n.get("plans") or []
        total_plans += len(plans)
        for p in plans[:2]:
            price_datas = p.get("price_datas") or []
            price = price_datas[0].get("price") if price_datas else 0
            print(f"[{area_name}] {p.get('plan_name')} - {price} / month (cpu: {p.get('cpu')}C, mem: {p.get('memory')}M)")
print(f"Total authentic plans across all areas: {total_plans}")
