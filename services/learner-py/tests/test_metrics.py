from __future__ import annotations

import asyncio
from unittest.mock import MagicMock, patch

from learner.metrics import MetricsRegistry


def test_metrics_registry_uses_isolated_prometheus_registry() -> None:
    first = MetricsRegistry(port=0)
    second = MetricsRegistry(port=0)

    first.sample_attempts_total.inc()
    second.sample_attempts_total.inc()

    # The counters should track values independently to ensure registries do not clash.
    assert first.sample_attempts_total._value.get() == 1  # type: ignore[attr-defined]
    assert second.sample_attempts_total._value.get() == 1  # type: ignore[attr-defined]


def test_metrics_registry_track_sample_latency_context_manager() -> None:
    metrics = MetricsRegistry(port=0)

    async def run() -> None:
        async with metrics.track_sample_latency():
            await asyncio.sleep(0)

    asyncio.run(run())

    histogram = metrics.sample_latency_seconds.collect()[0]
    count_samples = [sample for sample in histogram.samples if sample.name.endswith("_count")]

    assert count_samples and count_samples[0].value == 1


def test_metrics_registry_start_exporter_only_starts_once() -> None:
    metrics = MetricsRegistry(port=0)
    mock_server = MagicMock()

    with patch("learner.metrics.start_http_server", return_value=mock_server) as start_mock:
        metrics.start_exporter()
        metrics.start_exporter()

    assert start_mock.call_count == 1
    assert metrics._server is mock_server


def test_metrics_registry_stop_exporter_shuts_down_server() -> None:
    metrics = MetricsRegistry(port=0)
    mock_server = MagicMock()

    with patch("learner.metrics.start_http_server", return_value=mock_server):
        metrics.start_exporter()

    metrics.stop_exporter()

    mock_server.shutdown.assert_called_once()
    mock_server.server_close.assert_called_once()
    assert metrics._server is None
