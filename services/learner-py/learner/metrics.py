"""Prometheus metrics and tracing utilities."""

from __future__ import annotations

import asyncio
import contextlib
from typing import AsyncIterator

from prometheus_client import Counter, Gauge, Histogram, start_http_server


class MetricsRegistry:
    """Centralised Prometheus metrics for the learner process."""

    def __init__(self, *, port: int = 9001) -> None:
        self._port = port
        self.samples_total = Counter("learner_samples_total", "Number of samples processed", ["status"])
        self.sample_latency_seconds = Histogram(
            "learner_sample_latency_seconds", "Latency of replay sampling requests"
        )
        self.sgd_steps_total = Counter("learner_sgd_steps_total", "Number of SGD steps executed")
        self.policy_loss = Gauge("learner_policy_loss", "Latest policy loss")
        self.value_loss = Gauge("learner_value_loss", "Latest value loss")
        self.entropy = Gauge("learner_entropy", "Latest policy entropy")
        self.checkpoint_duration = Histogram(
            "learner_checkpoint_duration_seconds", "Duration of checkpoint operations"
        )
        self.weights_published_total = Counter(
            "learner_weights_publish_total", "Number of weight updates published"
        )
        self._server_task: asyncio.Task[None] | None = None

    def start_exporter(self) -> None:
        if self._server_task is None:
            loop = asyncio.get_running_loop()
            self._server_task = loop.create_task(self._run_exporter())

    async def _run_exporter(self) -> None:
        loop = asyncio.get_running_loop()
        await loop.run_in_executor(None, start_http_server, self._port)

    @contextlib.asynccontextmanager
    async def track_sample_latency(self) -> AsyncIterator[None]:
        with self.sample_latency_seconds.time():
            yield


__all__ = ["MetricsRegistry"]
