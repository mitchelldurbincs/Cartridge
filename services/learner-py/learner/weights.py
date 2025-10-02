"""Weight publication helpers."""

"""Weight publication helpers."""

from __future__ import annotations

import asyncio
import json
from dataclasses import dataclass

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

    async def publish(self, payload: WeightPayload) -> None:
        async with self._lock:
            if self._config.backend == "redis":
                await self._publish_redis(payload)
            else:  # pragma: no cover - not yet implemented
                raise NotImplementedError(f"Unknown weight backend '{self._config.backend}'")
            self._last_payload = payload

    async def _publish_redis(self, payload: WeightPayload) -> None:
        if self._redis is None:
            self._redis = aioredis.from_url(self._config.endpoint)
        message = json.dumps({"step": payload.step, "checksum": payload.checksum, "uri": payload.uri})
        try:
            await self._redis.publish(self._config.channel, message)
        except Exception as exc:  # pragma: no cover - network failure path
            raise RuntimeError("Failed to publish weights to redis") from exc

    async def close(self) -> None:
        if self._redis is not None:
            await self._redis.close()
            self._redis = None

    @property
    def last_payload(self) -> WeightPayload | None:
        return self._last_payload


__all__ = ["WeightPayload", "WeightPublisher"]
