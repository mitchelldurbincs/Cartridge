"""Main training loop for the learner."""

from __future__ import annotations

import asyncio
import logging
import time
from typing import Awaitable, Callable

import structlog

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
        self._logger = structlog.get_logger(__name__)
        self._step_count = 0
        self._last_log_time = time.time()

    async def run(self) -> None:
        self._logger.info("Starting training loop", algorithm=self._config.algorithm.name,
                         device=self._config.training.device, rollout_size=self._config.training.rollout_size)
        self._metrics.start_exporter()

        async with self._replay_client:
            while not self._stopping.is_set():
                try:
                    loop_start = time.time()

                    batch = await self._fetch_batch()
                    update = self._algorithm.update(batch)
                    self._record_update(update)

                    await self._maybe_checkpoint(update)
                    await self._maybe_publish_weights(update)

                    if self._heartbeat_callback is not None:
                        await self._heartbeat_callback(update)

                    self._step_count += 1
                    loop_duration = time.time() - loop_start

                    # Log progress every 10 steps or every 30 seconds
                    current_time = time.time()
                    if self._step_count % 10 == 0 or (current_time - self._last_log_time) >= 30:
                        self._logger.info(
                            "Training progress",
                            step=update.step,
                            policy_loss=update.policy_loss,
                            value_loss=update.value_loss,
                            entropy=update.entropy,
                            total_loss=update.loss,
                            loop_duration_ms=round(loop_duration * 1000, 2),
                            steps_processed=self._step_count
                        )
                        self._last_log_time = current_time

                except Exception as exc:
                    self._logger.error("Error in training loop", error=str(exc), step=getattr(update, 'step', 'unknown') if 'update' in locals() else 'unknown')
                    raise

    async def stop(self) -> None:
        self._logger.info("Stopping training loop", total_steps_processed=self._step_count)
        self._stopping.set()
        await self._replay_client.stop()
        await self._weights.close()
        self._logger.info("Training loop stopped successfully")

    async def _fetch_batch(self) -> TransitionBatch:
        start_time = time.time()
        async with self._metrics.track_sample_latency():
            batch = await self._replay_client.sample()
        self._metrics.samples_total.labels(status="ok").inc()

        fetch_duration = time.time() - start_time
        if fetch_duration > 1.0:  # Log if fetching takes more than 1 second
            self._logger.warning(
                "Slow batch fetch detected",
                fetch_duration_ms=round(fetch_duration * 1000, 2),
                batch_size=len(batch.observations) if hasattr(batch, 'observations') else 'unknown'
            )

        return batch

    def _record_update(self, update: AlgorithmUpdate) -> None:
        self._metrics.sgd_steps_total.inc()
        self._metrics.policy_loss.set(update.policy_loss)
        self._metrics.value_loss.set(update.value_loss)
        self._metrics.entropy.set(update.entropy)

    async def _maybe_checkpoint(self, update: AlgorithmUpdate) -> None:
        if update.step < self._next_checkpoint_step:
            return

        self._logger.info("Starting checkpoint save", step=update.step, next_checkpoint=self._next_checkpoint_step)
        start = time.perf_counter()

        try:
            manifest = await self._checkpoints.save(
                step=update.step,
                model=self._algorithm.model,
                optimizer=self._algorithm.optimizer,
                metadata={"loss": update.loss},
            )
            duration = time.perf_counter() - start
            self._metrics.checkpoint_duration.observe(duration)
            self._next_checkpoint_step = update.step + self._config.checkpoints.interval_steps

            self._logger.info(
                "Checkpoint saved successfully",
                step=update.step,
                path=str(manifest.path),
                duration_ms=round(duration * 1000, 2),
                next_checkpoint=self._next_checkpoint_step
            )

            await self._weights.publish(
                WeightPayload(step=update.step, checksum=manifest.checksum, uri=str(manifest.path))
            )
            self._metrics.weights_published_total.inc()

            self._logger.info("Weights published to distribution", step=update.step, checksum=manifest.checksum)

        except Exception as exc:
            self._logger.error(
                "Failed to save checkpoint",
                step=update.step,
                error=str(exc),
                duration_ms=round((time.perf_counter() - start) * 1000, 2)
            )
            raise

    async def _maybe_publish_weights(self, update: AlgorithmUpdate) -> None:
        # Already handled inside checkpoint logic for the MVP cadence.
        return


__all__ = ["LearnerCore"]
