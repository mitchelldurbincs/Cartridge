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


def test_compute_gae_accepts_1d_inputs() -> None:
    rewards = torch.tensor([1.0, 0.5, 0.0])
    values = torch.tensor([0.3, 0.2, 0.1, 0.0])
    dones = torch.tensor([False, False, True])

    advantages, returns = compute_gae(rewards, values, dones, gamma=0.9, gae_lambda=0.95)

    assert advantages.shape == rewards.shape
    assert returns.shape == rewards.shape
    assert torch.all(torch.isfinite(advantages))


def test_compute_gae_appends_bootstrap_value_when_missing() -> None:
    rewards = torch.tensor([[1.0], [0.5]])
    values = torch.tensor([[0.3], [0.2]])  # Missing final bootstrap value
    dones = torch.tensor([[False], [True]])

    advantages, _ = compute_gae(rewards, values, dones, gamma=0.9, gae_lambda=0.95)

    assert advantages.shape == rewards.shape
