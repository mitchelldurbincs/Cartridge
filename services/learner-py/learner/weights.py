"""Weight publication helpers."""

from __future__ import annotations

import asyncio
import json
from collections.abc import Mapping
from dataclasses import dataclass
from datetime import datetime, timezone

import grpc
import structlog
from google.protobuf.timestamp_pb2 import Timestamp
from redis import asyncio as aioredis

from .config import WeightPublisherConfig
from .proto.weights.v1 import weights_pb2, weights_pb2_grpc


@dataclass(slots=True)
class WeightPayload:
    run_id: str
    step: int
    checksum: str
    uri: str
    metadata: Mapping[str, str] | None = None


class WeightPublisher:
    """Publishes weight updates to the configured distribution backend."""

    def __init__(
        self,
        config: WeightPublisherConfig,
        *,
        redis_client: aioredis.Redis | None = None,
        grpc_channel: grpc.aio.Channel | None = None,
        grpc_stub: weights_pb2_grpc.WeightsServiceStub | None = None,
    ) -> None:
        self._config = config
        self._redis: aioredis.Redis | None = redis_client
        self._grpc_channel: grpc.aio.Channel | None = grpc_channel
        self._grpc_stub: weights_pb2_grpc.WeightsServiceStub | None = grpc_stub
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
                elif self._config.backend == "grpc":
                    await self._publish_grpc(payload)
                else:  # pragma: no cover - defensive against misconfiguration
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

    async def _publish_grpc(self, payload: WeightPayload) -> None:
        await self._ensure_grpc_stub()

        metadata = {str(key): str(value) for key, value in (payload.metadata or {}).items()}
        published_at = Timestamp()
        published_at.FromDatetime(datetime.now(tz=timezone.utc))

        request = weights_pb2.PublishWeightsRequest(
            run_id=payload.run_id,
            step=payload.step,
            checksum=payload.checksum,
            artifact_uri=payload.uri,
            metadata=metadata,
            published_at=published_at,
        )

        try:
            response = await self._grpc_stub.PublishWeights(request)
            self._logger.debug(
                "gRPC publish acknowledged",
                version_step=response.version.step if response.version else None,
                version_checksum=response.version.checksum if response.version else None,
            )
        except grpc.RpcError as exc:  # pragma: no cover - network failure path
            self._logger.error(
                "gRPC publish failed",
                status_code=exc.code().name if exc.code() is not None else None,
                details=exc.details(),
            )
            raise RuntimeError("Failed to publish weights via gRPC") from exc

    async def _ensure_grpc_stub(self) -> None:
        if self._grpc_stub is not None:
            return

        if self._grpc_channel is None:
            self._logger.debug("Connecting to weights service", endpoint=self._config.endpoint)
            self._grpc_channel = grpc.aio.insecure_channel(self._config.endpoint)  # type: ignore[attr-defined]

        self._grpc_stub = weights_pb2_grpc.WeightsServiceStub(self._grpc_channel)
        self._logger.debug("Weights gRPC stub initialized")

    async def close(self) -> None:
        if self._redis is not None:
            self._logger.info(
                "Closing weight publisher",
                total_weights_published=self._publish_count
            )
            await self._redis.close()
            self._redis = None
            self._logger.debug("Weight publisher closed successfully")

        if self._grpc_channel is not None:
            self._logger.info(
                "Closing weights gRPC channel",
                total_weights_published=self._publish_count,
            )
            await self._grpc_channel.close()
            self._grpc_channel = None
            self._grpc_stub = None
            self._logger.debug("Weights gRPC channel closed successfully")

    @property
    def last_payload(self) -> WeightPayload | None:
        return self._last_payload


__all__ = ["WeightPayload", "WeightPublisher"]
