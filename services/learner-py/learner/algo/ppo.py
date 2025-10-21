"""Proximal Policy Optimisation implementation."""

from __future__ import annotations

import logging
import torch
from torch import nn
from torch.optim import Adam
import structlog

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
        self._logger = structlog.get_logger(__name__)

        model_params = sum(p.numel() for p in self._model.parameters())
        self._logger.info(
            "PPO algorithm initialized",
            device=str(self._device),
            observation_dim=training.observation_dim,
            action_dim=training.action_dim,
            learning_rate=training.learning_rate,
            model_parameters=model_params,
            gamma=config.gamma,
            gae_lambda=config.gae_lambda,
            clip_ratio=config.clip_ratio,
            entropy_coef=config.entropy_coef,
            value_loss_coef=config.value_loss_coef
        )

    def update(self, batch: TransitionBatch) -> AlgorithmUpdate:
        self._model.train()
        batch = batch.to_device(self._device)
        advantages, returns = self._ensure_advantages(batch)

        observations = batch.observations
        actions = batch.actions
        old_log_probs = batch.log_probs

        batch_size = observations.size(0)
        self._logger.debug(
            "Starting PPO update",
            step=self._step + 1,
            batch_size=batch_size,
            observation_shape=list(observations.shape),
            action_shape=list(actions.shape)
        )

        flat_obs = observations.view(-1, observations.shape[-1])
        flat_actions = actions.view(-1)
        flat_old_log_probs = old_log_probs.view(-1)
        flat_advantages = advantages.view(-1)
        flat_returns = returns.view(-1)

        # Normalize advantages
        adv_mean = flat_advantages.mean()
        adv_std = flat_advantages.std(unbiased=False)
        flat_advantages = (flat_advantages - adv_mean) / (adv_std + 1e-8)

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

        # Check for numerical issues
        if not torch.isfinite(loss):
            self._logger.error(
                "Non-finite loss detected",
                step=self._step + 1,
                loss=float(loss),
                policy_loss=float(policy_loss),
                value_loss=float(value_loss),
                entropy=float(entropy)
            )
            raise ValueError(f"Non-finite loss: {loss}")

        self._optimizer.zero_grad()
        loss.backward()

        # Log gradient norms before clipping
        total_grad_norm = nn.utils.clip_grad_norm_(self._model.parameters(), self._config.max_grad_norm)

        self._optimizer.step()
        self._step += 1

        # Log detailed metrics every 100 steps
        if self._step % 100 == 1:
            self._logger.info(
                "PPO training metrics",
                step=self._step,
                advantage_mean=float(adv_mean),
                advantage_std=float(adv_std),
                gradient_norm=float(total_grad_norm),
                ratio_mean=float(ratio.mean()),
                ratio_std=float(ratio.std()),
                clipping_fraction=float((ratio != clipped_ratio).float().mean())
            )

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

        rewards = batch.rewards
        dones = batch.dones
        values = batch.values

        rewards_was_1d = rewards.ndim == 1
        dones_was_1d = dones.ndim == 1
        values_was_1d = values.ndim == 1

        if rewards_was_1d:
            rewards = rewards.unsqueeze(-1)
        if dones_was_1d:
            dones = dones.unsqueeze(-1)
        if values_was_1d:
            values = values.unsqueeze(-1)

        advantages, returns = compute_gae(
            rewards=rewards,
            values=values,
            dones=dones,
            gamma=self._config.gamma,
            gae_lambda=self._config.gae_lambda,
        )

        if rewards_was_1d:
            advantages = advantages.squeeze(-1)
            returns = returns.squeeze(-1)

        batch.advantages = advantages
        batch.returns = returns
        return advantages, returns


def _register() -> None:
    register("ppo", lambda cfg, training: PPOLearner(cfg, training))


_register()


__all__ = ["PPOLearner"]
