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

    if rewards.ndim != 2 or values.ndim != 2 or dones.ndim != 2:
        raise ValueError("GAE expects 2-D tensors (time, batch)")
    if values.shape[0] != rewards.shape[0] + 1:
        raise ValueError("Values must have one more timestep than rewards for bootstrapping")
    if rewards.shape != dones.shape:
        raise ValueError("Rewards and dones must have matching shapes")

    device = rewards.device
    advantages = torch.zeros_like(rewards, device=device)
    gae = torch.zeros(rewards.shape[1], device=device)

    for t in reversed(range(rewards.shape[0])):
        mask = 1.0 - dones[t]
        delta = rewards[t] + gamma * values[t + 1] * mask - values[t]
        gae = delta + gamma * gae_lambda * mask * gae
        advantages[t] = gae

    returns = advantages + values[:-1]
    return advantages, returns


__all__ = ["compute_gae"]
