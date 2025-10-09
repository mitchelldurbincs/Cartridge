"""Numerical helpers for reinforcement learning algorithms."""

from __future__ import annotations

import torch


def compute_gae(
    rewards: torch.Tensor,
    values: torch.Tensor,
    dones: torch.Tensor,
    *,
    gamma: float,
    gae_lambda: float,
) -> tuple[torch.Tensor, torch.Tensor]:
    """Compute Generalised Advantage Estimation.

    Args:
        rewards: Tensor of rewards with shape ``[T, B]``.
        values: Value function predictions with shape ``[T + 1, B]``.
        dones: Done flags with shape ``[T, B]``.
        gamma: Discount factor.
        gae_lambda: Smoothing parameter.

    Returns:
        advantages, returns tensors.
    """

    rewards_was_1d = rewards.ndim == 1
    dones_was_1d = dones.ndim == 1
    values_was_1d = values.ndim == 1

    if rewards.ndim not in (1, 2):
        raise ValueError("Rewards must be 1-D or 2-D tensor")
    if dones.ndim not in (1, 2):
        raise ValueError("Dones must be 1-D or 2-D tensor")
    if values.ndim not in (1, 2):
        raise ValueError("Values must be 1-D or 2-D tensor")

    if rewards_was_1d:
        rewards = rewards.unsqueeze(-1)
    if dones_was_1d:
        dones = dones.unsqueeze(-1)
    if values_was_1d:
        values = values.unsqueeze(-1)

    if rewards.ndim != 2 or values.ndim != 2 or dones.ndim != 2:
        raise ValueError("GAE expects 2-D tensors (time, batch)")

    if rewards.shape != dones.shape:
        raise ValueError("Rewards and dones must have matching shapes")

    dones = dones.to(dtype=rewards.dtype)

    if values.shape[0] == rewards.shape[0]:
        # Replay data may omit the bootstrap value for the final timestep. Use the
        # last value prediction as a best-effort bootstrap so computation can proceed.
        values = torch.cat([values, values[-1:].clone()], dim=0)

    if values.shape[0] != rewards.shape[0] + 1:
        raise ValueError("Values must have one more timestep than rewards for bootstrapping")

    device = rewards.device
    advantages = torch.zeros_like(rewards, device=device)
    gae = torch.zeros(rewards.shape[1], device=device)

    for t in reversed(range(rewards.shape[0])):
        mask = 1.0 - dones[t]
        delta = rewards[t] + gamma * values[t + 1] * mask - values[t]
        gae = delta + gamma * gae_lambda * mask * gae
        advantages[t] = gae

    returns = advantages + values[:-1]

    if rewards_was_1d:
        advantages = advantages.squeeze(-1)
        returns = returns.squeeze(-1)

    return advantages, returns


__all__ = ["compute_gae"]
