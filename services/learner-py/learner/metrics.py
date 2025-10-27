"""Prometheus metrics and tracing utilities."""

from __future__ import annotations

import asyncio
import contextlib
import structlog
from typing import AsyncIterator

from prometheus_client import (
    CollectorRegistry,
    Counter,
    Gauge,
    Histogram,
    start_http_server,
)

_LOGGER = structlog.get_logger(__name__)


class MetricsRegistry:
    """Centralised Prometheus metrics for the learner process."""

    def __init__(self, *, port: int = 9001, addr: str = "0.0.0.0", registry: CollectorRegistry | None = None) -> None:
        self._port = port
        self._addr = addr
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
            buckets=[.01, .05, .1, .25, .5, 1, 2.5, 5, 10, 30, 60],
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
            buckets=[.01, .05, .1, .25, .5, 1, 2.5, 5, 10, 30, 60],
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
        try:
            _LOGGER.info("Starting Prometheus metrics HTTP server", port=self._port, addr=self._addr)
            loop = asyncio.get_running_loop()
            # start_http_server is blocking, so run it in a thread executor
            # We don't await completion because the server runs indefinitely
            await loop.run_in_executor(None, self._start_server_blocking)
        except Exception as exc:
            _LOGGER.error(
                "Failed to start Prometheus metrics HTTP server",
                error=str(exc),
                error_type=type(exc).__name__,
                port=self._port,
                addr=self._addr
            )
            raise

    def _start_server_blocking(self) -> None:
        """Blocking call to start the HTTP server - runs in executor thread."""
        try:
            _LOGGER.info("Calling prometheus_client.start_http_server", port=self._port, addr=self._addr)
            # This call blocks indefinitely - it only returns if the server stops
            start_http_server(self._port, self._addr, self._registry)
            # We should never reach here unless the server stops
            _LOGGER.warning("Prometheus metrics HTTP server stopped unexpectedly", port=self._port, addr=self._addr)
        except Exception as exc:
            _LOGGER.error(
                "Exception in Prometheus HTTP server thread",
                error=str(exc),
                error_type=type(exc).__name__,
                port=self._port,
                addr=self._addr
            )
            raise

    @contextlib.asynccontextmanager
    async def track_sample_latency(self) -> AsyncIterator[None]:
        with self.sample_latency_seconds.time():
            yield


__all__ = ["MetricsRegistry"]
