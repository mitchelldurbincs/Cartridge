"""Async replay buffer client with background prefetching."""

from __future__ import annotations

import asyncio
import contextlib
import logging
from collections.abc import Awaitable, Callable

import importlib
from typing import TYPE_CHECKING

import grpc
from tenacity import (
    AsyncRetrying,
    RetryError,
    retry_if_exception_type,
    stop_after_attempt,
    wait_random_exponential
)

from .config import ReplayConfig
from .datamodel import TransitionBatch
from .metrics import MetricsRegistry
from .replay import SampleResponseLike, sample_response_to_batch

if TYPE_CHECKING:
    from .proto.replay.v1 import replay_pb2 as _replay_pb2
    from .proto.replay.v1 import replay_pb2_grpc as _replay_pb2_grpc

SamplerResult = TransitionBatch | SampleResponseLike
SamplerFn = Callable[[], Awaitable[SamplerResult]] | Callable[[], SamplerResult]


class ReplayClient:
    """Client responsible for streaming batches from the replay buffer."""

    def __init__(
        self,
        config: ReplayConfig,
        *,
        sampler: SamplerFn | None = None,
        metrics: MetricsRegistry | None = None,
    ) -> None:
        self._config = config
        self._queue: asyncio.Queue[TransitionBatch] = asyncio.Queue(maxsize=config.prefetch_depth)
        self._sampler = sampler
        self._prefetch_task: asyncio.Task[None] | None = None
        self._stopping = asyncio.Event()
        self._metrics = metrics
        self._channel: grpc.aio.Channel | None = None
        self._stub = None
        self._logger = logging.getLogger(__name__)

    async def __aenter__(self) -> "ReplayClient":
        await self.start()
        return self

    async def __aexit__(self, *exc_info: object) -> None:
        await self.stop()

    async def start(self) -> None:
        if self._prefetch_task is None:
            self._prefetch_task = asyncio.create_task(self._prefetch_loop())

    async def stop(self) -> None:
        if self._prefetch_task is not None:
            self._stopping.set()
            self._prefetch_task.cancel()
            with contextlib.suppress(asyncio.CancelledError):
                await self._prefetch_task
            self._prefetch_task = None
            self._stopping.clear()

        await self._close_channel()

        while not self._queue.empty():
            try:
                self._queue.get_nowait()
            except asyncio.QueueEmpty:  # pragma: no cover - defensive
                break

    async def sample(self) -> TransitionBatch:
        """Return the next available batch, waiting for prefetch if necessary."""

        return await self._queue.get()

    async def _prefetch_loop(self) -> None:
        """Background prefetch loop with enhanced error handling."""
        consecutive_failures = 0
        max_consecutive_failures = 10

        while not self._stopping.is_set():
            try:
                # Use shorter retry for individual samples in the prefetch loop
                async for attempt in AsyncRetrying(
                    wait=wait_random_exponential(multiplier=0.5, max=5.0),
                    stop=stop_after_attempt(3),
                    retry=retry_if_exception_type((grpc.RpcError, ConnectionError, ValueError)),
                    reraise=True,
                ):
                    with attempt:
                        batch = await self._invoke_sampler()
                        await self._queue.put(batch)
                        consecutive_failures = 0  # Reset on success
                        break  # Break out of retry loop on success

            except (RetryError, RuntimeError) as exc:
                consecutive_failures += 1
                self._logger.error(
                    "Sample fetch failed (attempt %d/%d): %s",
                    consecutive_failures, max_consecutive_failures, exc
                )

                if consecutive_failures >= max_consecutive_failures:
                    self._logger.critical(
                        "Too many consecutive failures (%d), stopping prefetch loop",
                        consecutive_failures
                    )
                    raise RuntimeError("Prefetch loop failed after too many consecutive errors") from exc

                # Wait before retrying the entire sample operation
                await asyncio.sleep(min(consecutive_failures * 0.5, 5.0))

            except Exception as exc:
                # Unexpected errors should stop the prefetch loop
                self._logger.critical("Unexpected error in prefetch loop: %s", exc)
                raise

    async def _invoke_sampler(self) -> TransitionBatch:
        sampler = self._sampler or self._grpc_sampler
        result = sampler()
        if asyncio.iscoroutine(result):
            result = await result
        if isinstance(result, TransitionBatch):
            return result
        return sample_response_to_batch(result)

    def _load_replay_modules(self) -> tuple[object, object]:
        replay_pb2 = importlib.import_module("learner.proto.replay.v1.replay_pb2")
        replay_pb2_grpc = importlib.import_module("learner.proto.replay.v1.replay_pb2_grpc")
        return replay_pb2, replay_pb2_grpc

    async def _ensure_connection(self) -> None:
        """Ensure gRPC channel and stub are initialized."""
        if self._channel is None or self._stub is None:
            replay_pb2, replay_pb2_grpc = self._load_replay_modules()

            if self._config.tls_enabled:
                self._channel = grpc.aio.secure_channel(  # type: ignore[attr-defined]
                    self._config.endpoint, grpc.ssl_channel_credentials()
                )
            else:
                self._channel = grpc.aio.insecure_channel(self._config.endpoint)  # type: ignore[attr-defined]

            self._stub = replay_pb2_grpc.ReplayStub(self._channel)
            self._logger.debug("gRPC connection established to %s", self._config.endpoint)

    async def _close_channel(self) -> None:
        """Close the gRPC channel if it exists."""
        if self._channel is not None:
            try:
                await self._channel.close()
                self._logger.debug("gRPC channel closed")
            except Exception as e:
                self._logger.warning("Error closing gRPC channel: %s", e)
            finally:
                self._channel = None
                self._stub = None

    async def _grpc_sampler(self) -> SampleResponseLike:
        """Sample from replay buffer with retry logic."""
        replay_pb2, _ = self._load_replay_modules()

        # Retry logic for transient failures
        async for attempt in AsyncRetrying(
            wait=wait_random_exponential(multiplier=0.25, min=0.1, max=2.0),
            stop=stop_after_attempt(3),
            retry=retry_if_exception_type((grpc.RpcError, ConnectionError)),
            reraise=True,
        ):
            with attempt:
                await self._ensure_connection()

                request = replay_pb2.SampleRequest(
                    config=replay_pb2.SampleConfig(batch_size=self._config.batch_size)
                )

                try:
                    if self._metrics is not None:
                        self._metrics.samples_total.labels(status="attempt").inc()

                    response = await self._stub.Sample(request)

                    if self._metrics is not None:
                        self._metrics.samples_total.labels(status="success").inc()

                    self._logger.debug("Successfully sampled %d transitions", len(list(response.transitions)))
                    return response

                except grpc.RpcError as e:
                    if self._metrics is not None:
                        self._metrics.samples_total.labels(status="error").inc()

                    # Close connection on RPC errors to force reconnection on retry
                    await self._close_channel()

                    # Log different error types
                    if e.code() == grpc.StatusCode.UNAVAILABLE:
                        self._logger.warning("Replay service unavailable, will retry: %s", e)
                    elif e.code() == grpc.StatusCode.DEADLINE_EXCEEDED:
                        self._logger.warning("Replay request timeout, will retry: %s", e)
                    else:
                        self._logger.error("gRPC sampling failed: %s", e)

                    raise  # Re-raise for retry logic

        # This should never be reached due to reraise=True
        raise RuntimeError("Retry logic failed unexpectedly")


__all__ = ["ReplayClient", "SamplerFn"]
