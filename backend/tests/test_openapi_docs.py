from __future__ import annotations

from app.main import app


def test_openapi_exposes_admin_read_and_control_security_and_tags():
    schema = app.openapi()
    security_schemes = schema.get("components", {}).get("securitySchemes", {})
    tags = {item.get("name") for item in schema.get("tags", [])}

    assert "AdminBearer" in security_schemes
    assert "PublicReadToken" in security_schemes
    assert "PublicControlToken" in security_schemes
    assert {"admin", "read-api", "control-api"}.issubset(tags)
