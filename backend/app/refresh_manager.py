"""
Dual-Rate Refresh Manager: manages slow background polling vs. timed high-speed burst/sniper polling.
"""
from __future__ import annotations

import time
import logging
from typing import Any

logger = logging.getLogger(__name__)


class RefreshManager:
    _instance: RefreshManager | None = None

    def __init__(self) -> None:
        self.mode: str = "slow"  # "slow" | "burst"
        self.slow_interval: int = 30  # seconds
        self.burst_interval: int = 2   # seconds
        self.burst_duration: int = 300  # seconds
        self.burst_started_at: float | None = None
        self.burst_expires_at: float | None = None
        self.last_poll_at: float | None = None
        self.total_bursts: int = 0

    @classmethod
    def get_instance(cls) -> RefreshManager:
        if cls._instance is None:
            cls._instance = cls()
        return cls._instance

    def get_state(self) -> dict[str, Any]:
        now = time.time()
        # Auto-expire burst mode if duration has passed
        if self.mode == "burst" and self.burst_expires_at:
            if now >= self.burst_expires_at:
                logger.info("Burst refresh mode auto-expired back to slow mode.")
                self.mode = "slow"
                self.burst_started_at = None
                self.burst_expires_at = None

        remaining = 0
        if self.mode == "burst" and self.burst_expires_at:
            remaining = max(0, int(self.burst_expires_at - now))

        current_interval = self.burst_interval if self.mode == "burst" else self.slow_interval

        return {
            "mode": self.mode,
            "current_interval": current_interval,
            "slow_interval": self.slow_interval,
            "burst_interval": self.burst_interval,
            "burst_duration": self.burst_duration,
            "remaining_seconds": remaining,
            "expires_at": self.burst_expires_at,
            "last_poll_at": self.last_poll_at,
            "total_bursts": self.total_bursts,
        }

    def start_burst(self, duration: int | None = None, interval: int | None = None) -> dict[str, Any]:
        now = time.time()
        if duration is not None and duration > 0:
            self.burst_duration = duration
        if interval is not None and interval > 0:
            self.burst_interval = max(1, interval)

        self.mode = "burst"
        self.burst_started_at = now
        self.burst_expires_at = now + self.burst_duration
        self.total_bursts += 1
        logger.info(
            "Burst refresh activated: interval=%ds for %ds (until %s)",
            self.burst_interval,
            self.burst_duration,
            time.strftime("%H:%M:%S", time.localtime(self.burst_expires_at)),
        )
        return self.get_state()

    def stop_burst(self) -> dict[str, Any]:
        self.mode = "slow"
        self.burst_started_at = None
        self.burst_expires_at = None
        logger.info("Burst refresh stopped manually, reverted to slow mode (%ds).", self.slow_interval)
        return self.get_state()

    def update_settings(self, slow_interval: int | None = None, burst_interval: int | None = None) -> dict[str, Any]:
        if slow_interval is not None and slow_interval >= 5:
            self.slow_interval = slow_interval
        if burst_interval is not None and burst_interval >= 1:
            self.burst_interval = burst_interval
        return self.get_state()

    def record_poll(self) -> None:
        self.last_poll_at = time.time()


refresh_mgr = RefreshManager.get_instance()
