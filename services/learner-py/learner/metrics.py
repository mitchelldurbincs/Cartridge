"""Prometheus metrics and tracing utilities."""

from __future__ import annotations

import contextlib
from socketserver import BaseServer
from typing import AsyncIterator

import structlog
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
        self._server: BaseServer | None = None

    def start_exporter(self) -> None:
        if self._server is not None:
            _LOGGER.debug(
                "Prometheus metrics HTTP server already running",
                port=self._port,
                addr=self._addr,
            )
            return

        try:
            _LOGGER.info("Starting Prometheus metrics HTTP server", port=self._port, addr=self._addr)
            self._server = start_http_server(self._port, self._addr, self._registry)
            _LOGGER.info("Prometheus metrics HTTP server started", port=self._port, addr=self._addr)
        except Exception as exc:
            _LOGGER.error(
                "Failed to start Prometheus metrics HTTP server",
                error=str(exc),
                error_type=type(exc).__name__,
                port=self._port,
                addr=self._addr,
            )
            raise

    def stop_exporter(self) -> None:
        if self._server is None:
            return

        _LOGGER.info("Stopping Prometheus metrics HTTP server", port=self._port, addr=self._addr)
        self._server.shutdown()
        self._server.server_close()
        self._server = None

    @contextlib.asynccontextmanager
    async def track_sample_latency(self) -> AsyncIterator[None]:
        with self.sample_latency_seconds.time():
            yield


__all__ = ["MetricsRegistry"]
