"""Main training loop for the learner."""

from __future__ import annotations

import asyncio
import time
from typing import Awaitable, Callable

from .algo import get_algorithm
from .config import LearnerConfig
from .datamodel import AlgorithmUpdate, TransitionBatch
from .metrics import MetricsRegistry
from .replay_client import ReplayClient
from .weights import WeightPayload, WeightPublisher
from .checkpoints import CheckpointManager


class LearnerCore:
    """Coordinates the end-to-end training workflow."""

    def __init__(
        self,
        config: LearnerConfig,
        replay_client: ReplayClient,
        checkpoints: CheckpointManager,
        weights: WeightPublisher,
        metrics: MetricsRegistry,
        *,
        heartbeat_callback: Callable[[AlgorithmUpdate], Awaitable[None]] | None = None,
    ) -> None:
        self._config = config
        self._replay_client = replay_client
        self._checkpoints = checkpoints
        self._weights = weights
        self._metrics = metrics
        self._heartbeat_callback = heartbeat_callback
        self._algorithm = get_algorithm(config.algorithm, config.training)
        self._next_checkpoint_step = config.checkpoints.interval_steps
        self._stopping = asyncio.Event()

    async def run(self) -> None:
        self._metrics.start_exporter()
        async with self._replay_client:
            while not self._stopping.is_set():
                batch = await self._fetch_batch()
                update = self._algorithm.update(batch)
                self._record_update(update)
                await self._maybe_checkpoint(update)
                await self._maybe_publish_weights(update)
                if self._heartbeat_callback is not None:
                    await self._heartbeat_callback(update)

    async def stop(self) -> None:
        self._stopping.set()
        await self._replay_client.stop()
        await self._weights.close()

    async def _fetch_batch(self) -> TransitionBatch:
        async with self._metrics.track_sample_latency():
            batch = await self._replay_client.sample()
        self._metrics.samples_total.labels(status="ok").inc()
        return batch

    def _record_update(self, update: AlgorithmUpdate) -> None:
        self._metrics.sgd_steps_total.inc()
        self._metrics.policy_loss.set(update.policy_loss)
        self._metrics.value_loss.set(update.value_loss)
        self._metrics.entropy.set(update.entropy)

    async def _maybe_checkpoint(self, update: AlgorithmUpdate) -> None:
        if update.step < self._next_checkpoint_step:
            return
        start = time.perf_counter()
        manifest = await self._checkpoints.save(
            step=update.step,
            model=self._algorithm.model,
            optimizer=self._algorithm.optimizer,
            metadata={"loss": update.loss},
        )
        duration = time.perf_counter() - start
        self._metrics.checkpoint_duration.observe(duration)
        self._next_checkpoint_step = update.step + self._config.checkpoints.interval_steps
        await self._weights.publish(
            WeightPayload(step=update.step, checksum=manifest.checksum, uri=str(manifest.path))
        )
        self._metrics.weights_published_total.inc()

    async def _maybe_publish_weights(self, update: AlgorithmUpdate) -> None:
        # Already handled inside checkpoint logic for the MVP cadence.
        return


__all__ = ["LearnerCore"]
