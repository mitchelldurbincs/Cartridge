"""Model definitions used by learner algorithms."""

from __future__ import annotations

from collections.abc import Sequence

import torch
from torch import nn
from torch.distributions import Categorical, Distribution


class ActorCriticNetwork(nn.Module):
    """Simple shared-body actor-critic network."""

    def __init__(
        self,
        observation_dim: int,
        action_dim: int,
        hidden_sizes: Sequence[int] = (256, 256),
        activation: type[nn.Module] = nn.Tanh,
    ) -> None:
        super().__init__()
        layers: list[nn.Module] = []
        input_dim = observation_dim
        for size in hidden_sizes:
            layers.append(nn.Linear(input_dim, size))
            layers.append(activation())
            input_dim = size
        self.body = nn.Sequential(*layers)
        self.policy_head = nn.Linear(input_dim, action_dim)
        self.value_head = nn.Linear(input_dim, 1)

        self._init_parameters()

    def forward(self, observations: torch.Tensor) -> tuple[Distribution, torch.Tensor]:
        features = self.body(observations)
        logits = self.policy_head(features)
        value = self.value_head(features).squeeze(-1)
        return Categorical(logits=logits), value

    def _init_parameters(self) -> None:
        for module in self.modules():
            if isinstance(module, nn.Linear):
                nn.init.orthogonal_(module.weight, gain=nn.init.calculate_gain("tanh"))
                nn.init.zeros_(module.bias)


__all__ = ["ActorCriticNetwork"]
