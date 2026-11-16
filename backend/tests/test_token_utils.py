"""Token 清洗与 Authorization 头格式测试（三平台通用）。"""
from __future__ import annotations

import pytest

from app import token_utils
from app.token_utils import bare_header, bearer_header, clean_token


@pytest.mark.parametrize(
    "raw,expected",
    [
        ('"eyJhbGciOiJIUzI1NiJ9.payload.sig"', "eyJhbGciOiJIUzI1NiJ9.payload.sig"),
        ("'eyJabc.def.ghi'", "eyJabc.def.ghi"),
        ("  eyJabc.def.ghi  ", "eyJabc.def.ghi"),
        ("Bearer eyJabc.def.ghi", "eyJabc.def.ghi"),
        ("bearer eyJabc.def.ghi", "eyJabc.def.ghi"),
        ('Bearer "eyJabc.def.ghi"', "eyJabc.def.ghi"),
        ('"Bearer eyJabc.def.ghi"', "eyJabc.def.ghi"),
        ("", ""),
        (None, ""),
        (12345, ""),
    ],
)
def test_clean_token(raw, expected):
    assert clean_token(raw) == expected


def test_bearer_header_for_lazycat_and_acck():
    """懒猫云 / ACCK 要求 Bearer 前缀，且引号必须被剥掉。"""
    assert bearer_header('"eyJabc.def.ghi"') == "Bearer eyJabc.def.ghi"
    assert bearer_header("") == ""
    assert '"' not in bearer_header('"eyJabc"')


def test_bare_header_for_akile():
    """Akile 要裸 JWT：加 Bearer 前缀反而 401，绝对不能带。"""
    h = bare_header('"eyJabc.def.ghi"')
    assert h == "eyJabc.def.ghi"
    assert not h.startswith("Bearer")


def test_all_three_clients_use_shared_cleaner():
    """三个客户端的 header 出口都必须过清洗，脏 token 不能直接拼进 Authorization。"""
    from app import acck_client, akile_client, lazycat_client

    dirty = '"eyJabc.def.ghi"'

    assert akile_client._headers(dirty)["Authorization"] == "eyJabc.def.ghi"
    assert akile_client._ai_headers(dirty)["Authorization"] == "eyJabc.def.ghi"

    assert acck_client._headers(dirty)["Authorization"] == "Bearer eyJabc.def.ghi"

    assert lazycat_client._headers(dirty)["Authorization"] == "Bearer eyJabc.def.ghi"
    assert lazycat_client.clean_token(dirty) == "eyJabc.def.ghi"


def test_lazycat_clean_token_delegates_to_shared():
    """懒猫云的 clean_token 必须与共享实现结果一致（避免三份实现漂移）。"""
    for sample in ['"eyJx"', "Bearer eyJx", "  eyJx ", None, "", 42]:
        assert token_utils.clean_token(sample) == __import__(
            "app.lazycat_client", fromlist=["clean_token"]
        ).clean_token(sample)
