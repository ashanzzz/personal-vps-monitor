"""
Centralized logging setup with persistent rotating file + structured events.

The log file lives in the data volume (/app/data) so it survives container
recreation. A user can paste it back for debugging; events use a flat
``key=value`` shape that is greppable and self-explanatory.

Usage:
    from .logging_setup import setup_logging, log_event
    setup_logging()                       # once, at startup
    log_event("poll.fetch", category="secondhand", list_len=0)
"""
from __future__ import annotations

import logging
import os
from logging.handlers import RotatingFileHandler

LOG_DIR = os.getenv("DATA_DIR", "/app/data")
LOG_FILE = os.path.join(LOG_DIR, "akile-monitor.log")

_configured = False


def setup_logging(level: str | None = None) -> None:
    """Configure root logging: stdout + rotating file. Idempotent."""
    global _configured
    if _configured:
        return

    lvl = (level or os.getenv("LOG_LEVEL", "INFO")).upper()
    numeric = getattr(logging, lvl, logging.INFO)

    fmt = logging.Formatter(
        "%(asctime)s %(levelname)s %(name)s %(message)s",
        datefmt="%Y-%m-%d %H:%M:%S",
    )

    root = logging.getLogger()
    root.setLevel(numeric)

    # Clear any handlers installed by basicConfig so we don't double-log.
    for h in list(root.handlers):
        root.removeHandler(h)

    stream = logging.StreamHandler()
    stream.setFormatter(fmt)
    root.addHandler(stream)

    try:
        os.makedirs(LOG_DIR, exist_ok=True)
        fileh = RotatingFileHandler(
            LOG_FILE, maxBytes=5 * 1024 * 1024, backupCount=5, encoding="utf-8"
        )
        fileh.setFormatter(fmt)
        root.addHandler(fileh)
    except OSError as e:  # pragma: no cover - disk/permission edge case
        root.warning("logging.file_disabled error=%s path=%s", e, LOG_FILE)

    _configured = True


def set_level(level: str) -> None:
    """Adjust the live log level (driven by the log_level setting)."""
    numeric = getattr(logging, (level or "INFO").upper(), logging.INFO)
    logging.getLogger().setLevel(numeric)


_logger = logging.getLogger("akile")


def _fmt_val(v: object) -> str:
    s = str(v)
    # Quote values containing spaces so key=value parsing stays clean.
    if " " in s or "=" in s:
        return '"' + s.replace('"', "'") + '"'
    return s


def log_event(event: str, level: int = logging.INFO, **fields: object) -> None:
    """Emit a structured one-line event: ``<event> k1=v1 k2=v2``."""
    parts = [event]
    for k, v in fields.items():
        if v is None:
            continue
        parts.append(f"{k}={_fmt_val(v)}")
    _logger.log(level, " ".join(parts))


def read_log_tail(lines: int = 500) -> str:
    """Return the last ``lines`` lines of the persistent log file."""
    try:
        with open(LOG_FILE, "r", encoding="utf-8", errors="replace") as f:
            return "".join(f.readlines()[-lines:])
    except FileNotFoundError:
        return ""
