from __future__ import annotations

import os
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))

# Keep test imports/fixtures isolated from the container's production DB path.
os.environ.setdefault(
    "DATABASE_URL",
    f"sqlite+aiosqlite:///{(ROOT / '.test-akile-monitor.db').as_posix()}"
)
