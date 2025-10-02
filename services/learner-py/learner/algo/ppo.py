"""Proximal Policy Optimisation implementation."""

from __future__ import annotations

import torch
from torch import nn
from torch.optim import Adam

from ..config import AlgorithmConfig, TrainingConfig
from ..datamodel import AlgorithmUpdate, TransitionBatch
from ..utils.math import compute_gae
from .networks import ActorCriticNetwork
from .registry import AlgorithmProtocol, register


class PPOLearner(AlgorithmProtocol):
    """Minimal PPO implementation matching the design document."""

    def __init__(self, config: AlgorithmConfig, training: TrainingConfig) -> None:
        self._config = config
        self._training = training
        self._device = torch.device(training.device)
        self._model = ActorCriticNetwork(
            observation_dim=training.observation_dim,
            action_dim=training.action_dim,
        ).to(self._device)
        self._optimizer = Adam(self._model.parameters(), lr=training.learning_rate)
        self._step = 0

    def update(self, batch: TransitionBatch) -> AlgorithmUpdate:
        self._model.train()
        batch = batch.to_device(self._device)
        advantages, returns = self._ensure_advantages(batch)

        observations = batch.observations
        actions = batch.actions
        old_log_probs = batch.log_probs

        flat_obs = observations.view(-1, observations.shape[-1])
        flat_actions = actions.view(-1)
        flat_old_log_probs = old_log_probs.view(-1)
        flat_advantages = advantages.view(-1)
        flat_returns = returns.view(-1)

        flat_advantages = (flat_advantages - flat_advantages.mean()) / (
            flat_advantages.std(unbiased=False) + 1e-8
        )

        dist, values = self._model(flat_obs)
        log_probs = dist.log_prob(flat_actions)
        ratio = torch.exp(log_probs - flat_old_log_probs)
        clipped_ratio = torch.clamp(
            ratio, 1.0 - self._config.clip_ratio, 1.0 + self._config.clip_ratio
        )
        policy_loss = -torch.min(ratio * flat_advantages, clipped_ratio * flat_advantages).mean()

        values = values.view_as(flat_returns)
        value_loss = 0.5 * (flat_returns - values).pow(2).mean()
        entropy = dist.entropy().mean()

        loss = (
            policy_loss
            + self._config.value_loss_coef * value_loss
            - self._config.entropy_coef * entropy
        )

        self._optimizer.zero_grad()
        loss.backward()
        nn.utils.clip_grad_norm_(self._model.parameters(), self._config.max_grad_norm)
        self._optimizer.step()

        self._step += 1
        return AlgorithmUpdate(
            step=self._step,
            loss=float(loss.detach().cpu().item()),
            policy_loss=float(policy_loss.detach().cpu().item()),
            value_loss=float(value_loss.detach().cpu().item()),
            entropy=float(entropy.detach().cpu().item()),
        )

    @property
    def model(self) -> ActorCriticNetwork:
        return self._model

    @property
    def optimizer(self) -> Adam:
        return self._optimizer

    def _ensure_advantages(self, batch: TransitionBatch) -> tuple[torch.Tensor, torch.Tensor]:
        if batch.advantages is not None and batch.returns is not None:
            return batch.advantages, batch.returns
        advantages, returns = compute_gae(
            rewards=batch.rewards,
            values=batch.values,
            dones=batch.dones,
            gamma=self._config.gamma,
            gae_lambda=self._config.gae_lambda,
        )
        batch.advantages = advantages
        batch.returns = returns
        return advantages, returns


def _register() -> None:
    register("ppo", lambda cfg, training: PPOLearner(cfg, training))


_register()


__all__ = ["PPOLearner"]
