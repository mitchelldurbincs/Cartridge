"""Prometheus metrics and tracing utilities."""

from __future__ import annotations

import asyncio
import contextlib
from typing import AsyncIterator

from prometheus_client import (
    CollectorRegistry,
    Counter,
    Gauge,
    Histogram,
    start_http_server,
)


class MetricsRegistry:
    """Centralised Prometheus metrics for the learner process."""

    def __init__(self, *, port: int = 9001, registry: CollectorRegistry | None = None) -> None:
        self._port = port
        self._registry = registry or CollectorRegistry(auto_describe=True)
        self.sample_attempts_total = Counter(
            "learner_sample_attempts_total",
            "Number of replay sample attempts",
            registry=self._registry,
        )
        self.sample_results_total = Counter(
            "learner_sample_results_total",
            "Replay sampling outcomes by result",
            ["result"],
            registry=self._registry,
        )
        self.sample_latency_seconds = Histogram(
            "learner_sample_latency_seconds",
            "Latency of replay sampling requests",
            registry=self._registry,
        )
        self.sgd_steps_total = Counter(
            "learner_sgd_steps_total",
            "Number of SGD steps executed",
            registry=self._registry,
        )
        self.policy_loss = Gauge(
            "learner_policy_loss",
            "Latest policy loss",
            registry=self._registry,
        )
        self.value_loss = Gauge(
            "learner_value_loss",
            "Latest value loss",
            registry=self._registry,
        )
        self.entropy = Gauge(
            "learner_entropy",
            "Latest policy entropy",
            registry=self._registry,
        )
        self.replay_queue_depth = Gauge(
            "learner_replay_queue_depth",
            "Depth of the learner replay prefetch queue",
            registry=self._registry,
        )
        self.heartbeat_success = Gauge(
            "learner_heartbeat_success",
            "Whether the most recent heartbeat emission succeeded",
            registry=self._registry,
        )
        self.checkpoint_duration = Histogram(
            "learner_checkpoint_duration_seconds",
            "Duration of checkpoint operations",
            registry=self._registry,
        )
        self.weights_published_total = Counter(
            "learner_weights_publish_total",
            "Number of weight updates published",
            registry=self._registry,
        )
        self._server_task: asyncio.Task[None] | None = None

    def start_exporter(self) -> None:
        if self._server_task is None:
            loop = asyncio.get_running_loop()
            self._server_task = loop.create_task(self._run_exporter())

    async def _run_exporter(self) -> None:
        loop = asyncio.get_running_loop()
        await loop.run_in_executor(None, start_http_server, self._port, "", self._registry)

    @contextlib.asynccontextmanager
    async def track_sample_latency(self) -> AsyncIterator[None]:
        with self.sample_latency_seconds.time():
            yield


__all__ = ["MetricsRegistry"]
