"""Async replay buffer client with background prefetching."""

from __future__ import annotations

import asyncio
import contextlib
from collections.abc import Awaitable, Callable

import importlib
from typing import TYPE_CHECKING

import grpc
from tenacity import AsyncRetrying, RetryError, stop_after_attempt, wait_random_exponential

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
        while not self._queue.empty():
            try:
                self._queue.get_nowait()
            except asyncio.QueueEmpty:  # pragma: no cover - defensive
                break

    async def sample(self) -> TransitionBatch:
        """Return the next available batch, waiting for prefetch if necessary."""

        return await self._queue.get()

    async def _prefetch_loop(self) -> None:
        try:
            async for attempt in AsyncRetrying(
                wait=wait_random_exponential(multiplier=0.5, max=10.0),
                stop=stop_after_attempt(5),
                reraise=True,
            ):
                with attempt:
                    while not self._stopping.is_set():
                        batch = await self._invoke_sampler()
                        await self._queue.put(batch)
        except RetryError as exc:  # pragma: no cover - escalated to orchestrator later
            raise RuntimeError("Replay client failed after retries") from exc

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

    async def _grpc_sampler(self) -> SampleResponseLike:
        replay_pb2, replay_pb2_grpc = self._load_replay_modules()
        if self._config.tls_enabled:
            channel: grpc.aio.Channel = grpc.aio.secure_channel(  # type: ignore[attr-defined]
                self._config.endpoint, grpc.ssl_channel_credentials()
            )
        else:
            channel = grpc.aio.insecure_channel(self._config.endpoint)  # type: ignore[attr-defined]
        stub = replay_pb2_grpc.ReplayStub(channel)
        request = replay_pb2.SampleRequest(
            config=replay_pb2.SampleConfig(batch_size=self._config.batch_size)
        )
        try:
            return await stub.Sample(request)
        except grpc.RpcError:
            if self._metrics is not None:
                self._metrics.samples_total.labels(status="error").inc()
            raise
        finally:
            await channel.close()


__all__ = ["ReplayClient", "SamplerFn"]
