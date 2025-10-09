"""Weight publication helpers."""

from __future__ import annotations

import asyncio
import json
import logging
from dataclasses import dataclass

import structlog
from redis import asyncio as aioredis

from .config import WeightPublisherConfig


@dataclass(slots=True)
class WeightPayload:
    step: int
    checksum: str
    uri: str


class WeightPublisher:
    """Publishes weight updates to the configured distribution backend."""

    def __init__(
        self, config: WeightPublisherConfig, *, redis_client: aioredis.Redis | None = None
    ) -> None:
        self._config = config
        self._redis: aioredis.Redis | None = redis_client
        self._lock = asyncio.Lock()
        self._last_payload: WeightPayload | None = None
        self._logger = structlog.get_logger(__name__)
        self._publish_count = 0

        self._logger.info(
            "WeightPublisher initialized",
            backend=config.backend,
            endpoint=config.endpoint,
            channel=config.channel
        )

    async def publish(self, payload: WeightPayload) -> None:
        self._logger.debug(
            "Publishing weights",
            step=payload.step,
            checksum=payload.checksum,
            uri=payload.uri
        )

        async with self._lock:
            try:
                if self._config.backend == "redis":
                    await self._publish_redis(payload)
                else:  # pragma: no cover - not yet implemented
                    raise NotImplementedError(f"Unknown weight backend '{self._config.backend}'")

                self._last_payload = payload
                self._publish_count += 1

                self._logger.info(
                    "Weights published successfully",
                    step=payload.step,
                    checksum=payload.checksum,
                    total_published=self._publish_count
                )

            except Exception as exc:
                self._logger.error(
                    "Failed to publish weights",
                    step=payload.step,
                    backend=self._config.backend,
                    error=str(exc)
                )
                raise

    async def _publish_redis(self, payload: WeightPayload) -> None:
        if self._redis is None:
            self._logger.debug("Connecting to Redis", endpoint=self._config.endpoint)
            self._redis = aioredis.from_url(self._config.endpoint)

        message = json.dumps({"step": payload.step, "checksum": payload.checksum, "uri": payload.uri})
        try:
            result = await self._redis.publish(self._config.channel, message)
            self._logger.debug(
                "Redis publish successful",
                channel=self._config.channel,
                message_size=len(message),
                subscribers=result
            )
        except Exception as exc:  # pragma: no cover - network failure path
            self._logger.error(
                "Redis publish failed",
                channel=self._config.channel,
                error=str(exc)
            )
            raise RuntimeError("Failed to publish weights to redis") from exc

    async def close(self) -> None:
        if self._redis is not None:
            self._logger.info(
                "Closing weight publisher",
                total_weights_published=self._publish_count
            )
            await self._redis.close()
            self._redis = None
            self._logger.debug("Weight publisher closed successfully")

    @property
    def last_payload(self) -> WeightPayload | None:
        return self._last_payload


__all__ = ["WeightPayload", "WeightPublisher"]
