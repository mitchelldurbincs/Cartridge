"""Domain models used by the learner service."""

from __future__ import annotations

from dataclasses import dataclass
from typing import Mapping

import torch


@dataclass(slots=True)
class TransitionBatch:
    """A batch of transitions sampled from replay."""

    observations: torch.Tensor
    actions: torch.Tensor
    log_probs: torch.Tensor
    rewards: torch.Tensor
    dones: torch.Tensor
    values: torch.Tensor
    advantages: torch.Tensor | None = None
    returns: torch.Tensor | None = None
    metadata: Mapping[str, str] | None = None

    def to_device(self, device: torch.device | str) -> "TransitionBatch":
        """Move all tensor fields to ``device`` returning ``self`` for chaining."""

        self.observations = self.observations.to(device)
        self.actions = self.actions.to(device)
        self.log_probs = self.log_probs.to(device)
        self.rewards = self.rewards.to(device)
        self.dones = self.dones.to(device)
        self.values = self.values.to(device)
        if self.advantages is not None:
            self.advantages = self.advantages.to(device)
        if self.returns is not None:
            self.returns = self.returns.to(device)
        return self


@dataclass(slots=True)
class AlgorithmUpdate:
    """Result of executing one optimisation step."""

    step: int
    loss: float
    policy_loss: float
    value_loss: float
    entropy: float


__all__ = ["AlgorithmUpdate", "TransitionBatch"]
