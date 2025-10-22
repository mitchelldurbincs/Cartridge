"""Async replay buffer client with background prefetching."""

from __future__ import annotations

import asyncio
import contextlib
import logging
from collections.abc import Awaitable, Callable

import importlib
from typing import TYPE_CHECKING

import grpc
import structlog
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


class NoTransitionsAvailableError(RuntimeError):
    """Raised when the replay service has no transitions to return yet."""


_EMPTY_REPLAY_MESSAGE = "no transitions available for sampling"


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
        self._logger = structlog.get_logger(__name__)
        if self._metrics is not None:
            self._metrics.replay_queue_depth.set(0)

    async def __aenter__(self) -> "ReplayClient":
        await self.start()
        return self

    async def __aexit__(self, *exc_info: object) -> None:
        await self.stop()

    async def start(self) -> None:
        if self._prefetch_task is None:
            self._logger.info(
                "Starting replay client",
                endpoint=self._config.endpoint,
                batch_size=self._config.batch_size,
                prefetch_depth=self._config.prefetch_depth,
                tls_enabled=self._config.tls_enabled
            )
            self._prefetch_task = asyncio.create_task(self._prefetch_loop())

    async def stop(self) -> None:
        if self._prefetch_task is not None:
            self._logger.info("Stopping replay client prefetch loop")
            self._stopping.set()
            self._prefetch_task.cancel()
            with contextlib.suppress(asyncio.CancelledError):
                await self._prefetch_task
            self._prefetch_task = None
            self._stopping.clear()

        await self._close_channel()

        # Clear any remaining items in the queue
        queued_items = 0
        while not self._queue.empty():
            try:
                self._queue.get_nowait()
                queued_items += 1
            except asyncio.QueueEmpty:  # pragma: no cover - defensive
                break

        if queued_items > 0:
            self._logger.debug("Cleared queued batches during shutdown", cleared_count=queued_items)

        self._logger.info("Replay client stopped successfully")
        if self._metrics is not None:
            self._metrics.replay_queue_depth.set(0)

    async def sample(self) -> TransitionBatch:
        """Return the next available batch, waiting for prefetch if necessary."""

        batch = await self._queue.get()
        self._record_queue_depth_metric()
        return batch

    def queue_metrics(self) -> tuple[int | None, int | None]:
        """Return current depth and capacity for the prefetch queue."""

        try:
            depth = self._queue.qsize()
        except NotImplementedError:  # pragma: no cover - implementation detail on some loops
            depth = None

        capacity = self._queue.maxsize if self._queue.maxsize > 0 else None
        return depth, capacity

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
                        self._record_queue_depth_metric()

                        # Log successful prefetch occasionally
                        if hasattr(batch, 'observations') and len(batch.observations) > 0:
                            # Log every 50th successful prefetch
                            if getattr(self, '_prefetch_success_count', 0) % 50 == 0:
                                self._logger.debug(
                                    "Prefetch successful",
                                    batch_size=len(batch.observations),
                                    queue_size=self._queue.qsize(),
                                    prefetch_count=getattr(self, '_prefetch_success_count', 0) + 1
                                )
                            self._prefetch_success_count = getattr(self, '_prefetch_success_count', 0) + 1

                        break  # Break out of retry loop on success

            except NoTransitionsAvailableError:
                consecutive_failures = 0
                if self._metrics is not None:
                    self._metrics.sample_results_total.labels(result="empty").inc()
                self._logger.debug(
                    "Replay buffer empty, waiting for transitions",
                    queue_size=self._queue.qsize(),
                )
                await asyncio.sleep(1.0)
                continue

            except (RetryError, RuntimeError) as exc:
                consecutive_failures += 1
                self._logger.error(
                    "Sample fetch failed",
                    attempt=consecutive_failures,
                    max_attempts=max_consecutive_failures,
                    error=str(exc),
                    error_type=type(exc).__name__,
                )

                if consecutive_failures >= max_consecutive_failures:
                    self._logger.critical(
                        "Prefetch loop exceeded consecutive failure threshold",
                        consecutive_failures=consecutive_failures,
                        max_consecutive_failures=max_consecutive_failures,
                    )
                    raise RuntimeError("Prefetch loop failed after too many consecutive errors") from exc

                # Wait before retrying the entire sample operation
                await asyncio.sleep(min(consecutive_failures * 0.5, 5.0))

            except Exception as exc:
                # Unexpected errors should stop the prefetch loop
                self._logger.critical(
                    "Unexpected error in prefetch loop",
                    error=str(exc),
                    error_type=type(exc).__name__,
                )
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
            self._logger.debug(
                "gRPC connection established",
                endpoint=self._config.endpoint,
                tls_enabled=self._config.tls_enabled,
            )

    async def _close_channel(self) -> None:
        """Close the gRPC channel if it exists."""
        if self._channel is not None:
            try:
                await self._channel.close()
                self._logger.debug("gRPC channel closed")
            except Exception as exc:
                self._logger.warning(
                    "Error closing gRPC channel",
                    error=str(exc),
                    error_type=type(exc).__name__,
                )
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
                        self._metrics.sample_attempts_total.inc()

                    response = await self._stub.Sample(request)

                    if self._metrics is not None:
                        self._metrics.sample_results_total.labels(result="success").inc()

                    self._logger.debug(
                        "Sampled transitions successfully",
                        transition_count=len(list(response.transitions)),
                    )
                    return response

                except grpc.RpcError as e:
                    details = (e.details() or "").lower()
                    code = e.code()

                    if (
                        code == grpc.StatusCode.INTERNAL
                        and _EMPTY_REPLAY_MESSAGE in details
                    ):
                        self._logger.debug(
                            "Replay service returned no transitions",
                            status_code=code.name if code is not None else None,
                        )
                        raise NoTransitionsAvailableError(_EMPTY_REPLAY_MESSAGE) from e

                    if self._metrics is not None:
                        self._metrics.sample_results_total.labels(result="error").inc()

                    # Close connection on RPC errors to force reconnection on retry
                    await self._close_channel()

                    # Log different error types
                    if code == grpc.StatusCode.UNAVAILABLE:
                        self._logger.warning(
                            "Replay service unavailable, will retry",
                            status_code=code.name,
                            details=e.details(),
                        )
                    elif code == grpc.StatusCode.DEADLINE_EXCEEDED:
                        self._logger.warning(
                            "Replay request timeout, will retry",
                            status_code=code.name,
                            details=e.details(),
                        )
                    else:
                        self._logger.error(
                            "gRPC sampling failed",
                            status_code=code.name if code is not None else None,
                            details=e.details(),
                        )

                    raise  # Re-raise for retry logic

        # This should never be reached due to reraise=True
        raise RuntimeError("Retry logic failed unexpectedly")

    def _record_queue_depth_metric(self) -> None:
        if self._metrics is None:
            return

        try:
            depth = self._queue.qsize()
        except NotImplementedError:  # pragma: no cover - implementation detail
            return

        self._metrics.replay_queue_depth.set(depth)


__all__ = ["ReplayClient", "SamplerFn", "NoTransitionsAvailableError"]
