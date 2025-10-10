"""Orchestrator control channel."""

from __future__ import annotations

import asyncio
import logging
from dataclasses import asdict, dataclass

import aiohttp
import structlog

from .config import ControlConfig


@dataclass(slots=True)
class HeartbeatPayload:
    run_id: str
    status: str
    step: int
    samples_per_sec: float
    loss: float
    checkpoint_version: int
    queued_commands: list[str] | None = None
    notes: str | None = None


class ControlClient:
    """Thin wrapper around the orchestrator HTTP API."""

    def __init__(self, config: ControlConfig) -> None:
        self._config = config
        self._session: aiohttp.ClientSession | None = None
        self._lock = asyncio.Lock()
        self._logger = structlog.get_logger(__name__)
        self._heartbeat_count = 0
        self._last_heartbeat_error = None

        self._logger.info(
            "ControlClient initialized",
            orchestrator_endpoint=config.orchestrator_endpoint,
            run_id=config.run_id,
            heartbeat_interval=config.heartbeat_interval_seconds
        )

    async def ensure_session(self) -> aiohttp.ClientSession:
        async with self._lock:
            if self._session is None:
                self._session = aiohttp.ClientSession()
                self._logger.debug("Created new HTTP session for orchestrator communication")
            return self._session

    async def send_heartbeat(self, payload: HeartbeatPayload) -> None:
        session = await self.ensure_session()
        url = f"{self._config.orchestrator_endpoint}/runs/{self._config.run_id}/heartbeat"

        try:
            async with session.post(url, json=asdict(payload), timeout=10) as response:
                response.raise_for_status()
                self._heartbeat_count += 1

                # Log every 10th heartbeat or if recovering from an error
                if self._heartbeat_count % 10 == 1 or self._last_heartbeat_error is not None:
                    self._logger.info(
                        "Heartbeat sent successfully",
                        run_id=payload.run_id,
                        step=payload.step,
                        status=payload.status,
                        loss=payload.loss,
                        samples_per_sec=payload.samples_per_sec,
                        checkpoint_version=payload.checkpoint_version,
                        heartbeat_count=self._heartbeat_count,
                        status_code=response.status
                    )
                    if self._last_heartbeat_error is not None:
                        self._logger.info("Recovered from heartbeat error")
                        self._last_heartbeat_error = None

        except asyncio.TimeoutError as exc:
            self._last_heartbeat_error = str(exc)
            self._logger.error(
                "Heartbeat timeout",
                run_id=self._config.run_id,
                step=payload.step,
                url=url,
                timeout_seconds=10
            )
            raise

        except aiohttp.ClientResponseError as exc:
            self._last_heartbeat_error = str(exc)
            self._logger.error(
                "Heartbeat HTTP error",
                run_id=self._config.run_id,
                step=payload.step,
                url=url,
                status_code=exc.status,
                message=exc.message
            )
            raise

        except Exception as exc:
            self._last_heartbeat_error = str(exc)
            self._logger.error(
                "Heartbeat failed with unexpected error",
                run_id=self._config.run_id,
                step=payload.step,
                url=url,
                error=str(exc)
            )
            raise

    async def close(self) -> None:
        if self._session is not None:
            self._logger.info(
                "Closing orchestrator session",
                total_heartbeats_sent=self._heartbeat_count
            )
            await self._session.close()
            self._session = None
            self._logger.debug("Orchestrator session closed")


__all__ = ["ControlClient", "HeartbeatPayload"]
