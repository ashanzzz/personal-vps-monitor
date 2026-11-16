"""Token 清洗工具 —— 三平台通用。

问题背景：各平台登录响应里的 token 常被包成 JSON 字符串（形如 `"eyJ..."`），
带一个首尾引号。原样拼进 Authorization 头会得到 `Bearer "eyJ...` 或 `"eyJ...`，
服务端一律 401，而前端表现是「登录成功但数据全空」——静默失败最难查。

各平台对 Authorization 格式要求不同：
  - 懒猫云：`Bearer <jwt>`
  - ACCK：  `Bearer <jwt>`
  - Akile： `<jwt>`（裸 JWT，加 Bearer 反而 401）
"""
from __future__ import annotations

from typing import Any

BEARER = "bearer "


def clean_token(token: Any) -> str:
    """剥掉 Bearer 前缀、首尾引号与空白，只留裸 token。

    顺序很重要：先剥 Bearer 再剥引号，否则 `Bearer "xxx"` 会漏掉引号。
    """
    if not isinstance(token, str):
        return ""
    t = token.strip()
    if t.lower().startswith(BEARER):
        t = t[len(BEARER):].strip()
    while len(t) >= 2 and t[0] in ('"', "'") and t[-1] == t[0]:
        t = t[1:-1].strip()
    t = t.strip().strip('"').strip("'").strip()
    # 极端情况：`Bearer "xxx"` 剥前缀后仍带引号
    if t.lower().startswith(BEARER):
        t = t[len(BEARER):].strip().strip('"').strip("'").strip()
    return t


def bearer_header(token: Any) -> str:
    """返回 `Bearer <jwt>` 形式的完整 Authorization 值（懒猫云 / ACCK）。"""
    t = clean_token(token)
    return f"Bearer {t}" if t else ""


def bare_header(token: Any) -> str:
    """返回裸 JWT（Akile 要求）。"""
    return clean_token(token)
