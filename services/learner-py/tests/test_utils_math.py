from __future__ import annotations

import torch

from learner.utils.math import compute_gae


def test_compute_gae_shapes() -> None:
    rewards = torch.tensor([[1.0, 0.0], [0.5, 0.2]])
    values = torch.tensor([[0.1, 0.0], [0.2, 0.1], [0.0, 0.0]])
    dones = torch.zeros_like(rewards)

    advantages, returns = compute_gae(rewards, values, dones, gamma=0.99, gae_lambda=0.95)

    assert advantages.shape == rewards.shape
    assert returns.shape == rewards.shape
    assert torch.all(torch.isfinite(advantages))
