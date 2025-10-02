"""Registry for learner algorithms."""

from __future__ import annotations

from collections.abc import Callable
from typing import Protocol

from torch import nn, optim

from ..config import AlgorithmConfig, TrainingConfig
from ..datamodel import AlgorithmUpdate, TransitionBatch


class AlgorithmProtocol(Protocol):
    """Interface implemented by learner algorithms."""

    def update(self, batch: TransitionBatch) -> AlgorithmUpdate:
        ...

    @property
    def model(self) -> nn.Module:
        ...

    @property
    def optimizer(self) -> optim.Optimizer:
        ...


AlgorithmFactory = Callable[[AlgorithmConfig, TrainingConfig], AlgorithmProtocol]

_REGISTRY: dict[str, AlgorithmFactory] = {}


def register(name: str, factory: AlgorithmFactory) -> None:
    _REGISTRY[name] = factory


def get_algorithm(config: AlgorithmConfig, training: TrainingConfig) -> AlgorithmProtocol:
    try:
        factory = _REGISTRY[config.name]
    except KeyError as exc:  # pragma: no cover - caller validated names already
        raise ValueError(f"Unknown algorithm '{config.name}'") from exc
    return factory(config, training)


__all__ = ["AlgorithmFactory", "AlgorithmProtocol", "get_algorithm", "register"]
