"""Orchestrator control channel."""

from __future__ import annotations

import asyncio
from dataclasses import dataclass

import aiohttp

from .config import ControlConfig


@dataclass(slots=True)
class HeartbeatPayload:
    step: int
    policy_loss: float
    value_loss: float
    checkpoint_step: int | None


class ControlClient:
    """Thin wrapper around the orchestrator HTTP API."""

    def __init__(self, config: ControlConfig) -> None:
        self._config = config
        self._session: aiohttp.ClientSession | None = None
        self._lock = asyncio.Lock()

    async def ensure_session(self) -> aiohttp.ClientSession:
        async with self._lock:
            if self._session is None:
                self._session = aiohttp.ClientSession()
            return self._session

    async def send_heartbeat(self, payload: HeartbeatPayload) -> None:
        session = await self.ensure_session()
        url = f"{self._config.orchestrator_endpoint}/runs/{self._config.run_id}/heartbeat"
        async with session.post(url, json=payload.__dict__, timeout=10):
            pass

    async def close(self) -> None:
        if self._session is not None:
            await self._session.close()
            self._session = None


__all__ = ["ControlClient", "HeartbeatPayload"]
