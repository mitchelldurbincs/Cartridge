"""Entrypoint wiring for the learner service."""

from __future__ import annotations

import asyncio
import logging
import random
from pathlib import Path

import numpy as np
import torch

from .checkpoints import CheckpointManager
from .datamodel import AlgorithmUpdate
from .config import load_config, parse_args
from .control import ControlClient, HeartbeatPayload
from .core import LearnerCore
from .metrics import MetricsRegistry
from .replay_client import ReplayClient
from .utils.logging import configure_logging
from .weights import WeightPublisher

_LOGGER = logging.getLogger(__name__)


def _seed_everything(seed: int) -> None:
    random.seed(seed)
    np.random.seed(seed)
    torch.manual_seed(seed)
    if torch.cuda.is_available():  # pragma: no cover - device specific
        torch.cuda.manual_seed_all(seed)


async def _run_async(config_path: Path, overrides: list[str]) -> None:
    config = load_config(config_path, overrides=overrides)
    configure_logging()
    _LOGGER.info("learner.start", run_id=config.control.run_id)
    _seed_everything(config.training.seed)

    metrics = MetricsRegistry()
    weights = WeightPublisher(config.weights)
    checkpoints = CheckpointManager(config.checkpoints)
    control = ControlClient(config.control)
    replay = ReplayClient(config.replay)

    async def heartbeat(update: AlgorithmUpdate) -> None:
        checkpoint_step = checkpoints.latest.step if checkpoints.latest else None
        payload = HeartbeatPayload(
            step=update.step,
            policy_loss=update.policy_loss,
            value_loss=update.value_loss,
            checkpoint_step=checkpoint_step,
        )
        await control.send_heartbeat(payload)

    learner = LearnerCore(
        config,
        replay,
        checkpoints,
        weights,
        metrics,
        heartbeat_callback=heartbeat,
    )
    try:
        await learner.run()
    except asyncio.CancelledError:  # pragma: no cover - cancellation path
        raise
    finally:
        await learner.stop()
        await control.close()


def run(argv: list[str] | None = None) -> None:
    args = parse_args(argv)
    asyncio.run(_run_async(args.config, args.override))


if __name__ == "__main__":  # pragma: no cover
    run()
