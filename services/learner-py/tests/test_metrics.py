from __future__ import annotations

import asyncio

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
