"""
Group Health Checker Engine for VPS Nodes and Proxy Groups.

Enables grouping instances by:
  - Platform (Akile, ACCK, LazyCat)
  - Region (Hong Kong, Japan, US, Singapore, etc.)
  - Network Route Tier (S-GIA, A-Optimized, B-BGP, C-Standard)

Runs concurrent batch health probes, calculates availability SLA,
average latency RTT, and highlights degraded or offline groups.
"""
from __future__ import annotations

import asyncio
import logging
from typing import Any, Dict, List, Optional

from .latency import batch_tcp_ping

logger = logging.getLogger(__name__)


def group_instances(
    instances: List[Dict[str, Any]],
    by: str = "region"
) -> Dict[str, List[Dict[str, Any]]]:
    """
    Cluster instances into groups by platform, region, or tier.
    """
    groups: Dict[str, List[Dict[str, Any]]] = {}
    for inst in instances:
        if by == "platform":
            key = str(inst.get("platform") or "unknown").upper()
        elif by == "region":
            # Extract standard region code or name
            key = str(inst.get("area_name") or inst.get("area") or inst.get("region") or "OTHER").strip().upper()
            if any(k in key for k in ("HK", "HONG KONG", "香港")):
                key = "香港 (HK)"
            elif any(k in key for k in ("JP", "JAPAN", "日本", "TOKYO")):
                key = "日本 (JP)"
            elif any(k in key for k in ("US", "USA", "美国", "LAX", "SJC")):
                key = "美国 (US)"
            elif any(k in key for k in ("SG", "SINGAPORE", "新加坡")):
                key = "新加坡 (SG)"
            elif any(k in key for k in ("TW", "TAIWAN", "台湾")):
                key = "台湾 (TW)"
            elif any(k in key for k in ("KR", "KOREA", "韩国")):
                key = "韩国 (KR)"
            elif any(k in key for k in ("DE", "GERMANY", "德国", "FRA")):
                key = "德国 (DE)"
            elif any(k in key for k in ("UK", "GB", "英国", "LON")):
                key = "英国 (UK)"
        elif by == "tier":
            cost_eff = inst.get("cost_effectiveness") or {}
            key = cost_eff.get("grade_label") or "标准节点"
        else:
            key = "DEFAULT"

        if key not in groups:
            groups[key] = []
        groups[key].append(inst)

    return groups


async def check_groups_health(
    instances: List[Dict[str, Any]],
    by: str = "region",
    timeout: float = 3.0
) -> Dict[str, Any]:
    """
    Run concurrent health and latency probe across all instance groups.
    """
    grouped = group_instances(instances, by=by)
    # Flatten targets for batch TCP ping
    targets = []
    for g_name, items in grouped.items():
        for it in items:
            host = it.get("ipv4") or it.get("ip") or it.get("host") or ""
            port = int(it.get("ssh_port") or 22)
            targets.append({
                "id": it.get("uid") or f"{it.get('platform')}:{it.get('id')}",
                "name": it.get("name"),
                "group": g_name,
                "host": host,
                "port": port,
                "raw_instance": it
            })

    # Execute all pings concurrently
    ping_results = await batch_tcp_ping(targets, timeout=timeout)
    results_by_id = {r["target_id"]: r for r in ping_results}

    # Summarize per group
    group_summaries = []
    total_nodes = len(targets)
    total_alive = 0

    for g_name, items in grouped.items():
        group_pings = []
        for it in items:
            uid = it.get("uid") or f"{it.get('platform')}:{it.get('id')}"
            res = results_by_id.get(uid, {"alive": False, "rtt_ms": -1})
            merged_node = {
                "uid": uid,
                "name": it.get("name"),
                "platform": it.get("platform"),
                "host": res.get("host"),
                "port": res.get("port"),
                "alive": res.get("alive", False),
                "rtt_ms": res.get("rtt_ms", -1),
                "error": res.get("error"),
            }
            group_pings.append(merged_node)
            if res.get("alive"):
                total_alive += 1

        alive_nodes = [p for p in group_pings if p["alive"]]
        alive_count = len(alive_nodes)
        node_count = len(group_pings)
        health_pct = round((alive_count / node_count) * 100, 1) if node_count > 0 else 0.0

        rtts = [p["rtt_ms"] for p in alive_nodes if p["rtt_ms"] > 0]
        avg_rtt = round(sum(rtts) / len(rtts), 1) if rtts else -1.0
        min_rtt = min(rtts) if rtts else -1.0
        max_rtt = max(rtts) if rtts else -1.0

        if alive_count == node_count:
            health_status = "healthy"
            status_text = "全部在线 ✅"
            badge_color = "success"
        elif alive_count > 0:
            health_status = "degraded"
            status_text = f"部分存活 ({alive_count}/{node_count}) ⚠️"
            badge_color = "warning"
        else:
            health_status = "offline"
            status_text = "全组离线 ❌"
            badge_color = "error"

        group_summaries.append({
            "group_name": g_name,
            "by": by,
            "total_nodes": node_count,
            "alive_nodes": alive_count,
            "offline_nodes": node_count - alive_count,
            "health_percent": health_pct,
            "health_status": health_status,
            "status_text": status_text,
            "badge_color": badge_color,
            "avg_rtt_ms": avg_rtt,
            "min_rtt_ms": min_rtt,
            "max_rtt_ms": max_rtt,
            "nodes": group_pings,
        })

    # Sort groups by health ratio descending, then name
    group_summaries.sort(key=lambda g: (-g["health_percent"], g["group_name"]))

    global_health_pct = round((total_alive / total_nodes) * 100, 1) if total_nodes > 0 else 0.0

    return {
        "by": by,
        "total_groups": len(group_summaries),
        "total_nodes": total_nodes,
        "total_alive": total_alive,
        "global_health_percent": global_health_pct,
        "groups": group_summaries,
    }
