"""Entrypoint wiring for the learner service."""

from __future__ import annotations

import asyncio
import logging
import random
from pathlib import Path

import numpy as np
import torch
import structlog

from .checkpoints import CheckpointManager
from .datamodel import AlgorithmUpdate
from .config import load_config, parse_args
from .control import ControlClient, HeartbeatPayload
from .core import LearnerCore
from .metrics import MetricsRegistry
from .replay_client import ReplayClient
from .utils.logging import configure_logging
from .weights import WeightPublisher

_LOGGER = structlog.get_logger(__name__)


def _seed_everything(seed: int) -> None:
    random.seed(seed)
    np.random.seed(seed)
    torch.manual_seed(seed)
    if torch.cuda.is_available():  # pragma: no cover - device specific
        torch.cuda.manual_seed_all(seed)


async def _run_async(config_path: Path, overrides: list[str]) -> None:
    config = load_config(config_path, overrides=overrides)
    configure_logging()

    _LOGGER.info(
        "Learner service starting",
        run_id=config.control.run_id,
        config_path=str(config_path),
        overrides=overrides,
        algorithm=config.algorithm.name,
        device=config.training.device,
        learning_rate=config.training.learning_rate,
        seed=config.training.seed
    )

    _seed_everything(config.training.seed)
    _LOGGER.info("Random seeds initialized", seed=config.training.seed)

    try:
        # Initialize components
        _LOGGER.info("Initializing service components")
        metrics = MetricsRegistry()
        weights = WeightPublisher(config.weights)
        checkpoints = CheckpointManager(config.checkpoints)
        control = ControlClient(config.control)
        replay = ReplayClient(config.replay, metrics=metrics)
        _LOGGER.info("All service components initialized successfully")

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

        _LOGGER.info("Starting main training loop")
        await learner.run()

    except asyncio.CancelledError:
        _LOGGER.info("Learner service cancelled")
        raise
    except Exception as exc:
        _LOGGER.error(
            "Learner service failed with unexpected error",
            error=str(exc),
            error_type=type(exc).__name__
        )
        raise
    finally:
        _LOGGER.info("Shutting down learner service")
        try:
            await learner.stop()
            await control.close()
            _LOGGER.info("Learner service shutdown completed successfully")
        except Exception as exc:
            _LOGGER.error(
                "Error during learner service shutdown",
                error=str(exc),
                error_type=type(exc).__name__
            )


def run(argv: list[str] | None = None) -> None:
    args = parse_args(argv)
    _LOGGER.info(
        "Learner service entry point",
        config_file=str(args.config),
        overrides=args.override
    )
    try:
        asyncio.run(_run_async(args.config, args.override))
    except KeyboardInterrupt:
        _LOGGER.info("Learner service interrupted by user")
    except Exception as exc:
        _LOGGER.error(
            "Learner service exited with error",
            error=str(exc),
            error_type=type(exc).__name__
        )
        raise


if __name__ == "__main__":  # pragma: no cover
    run()
