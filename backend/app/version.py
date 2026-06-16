"""
Application version metadata.

Bumped on each release. Exposed via GET /api/version and logged at startup so
that a log file pasted by the user always reveals which build produced it.
"""
from __future__ import annotations

import os

__version__ = "0.9.0"


def get_version_info() -> dict:
    return {
        "version": __version__,
        "git_sha": os.getenv("GIT_SHA", ""),
        "build_time": os.getenv("BUILD_TIME", ""),
    }
