from __future__ import annotations

import torch

from learner.algo.ppo import PPOLearner
from learner.config import AlgorithmConfig, TrainingConfig
from learner.datamodel import TransitionBatch


def _make_training_config() -> TrainingConfig:
    return TrainingConfig(
        rollout_size=4,
        learning_rate=1e-3,
        seed=0,
        device="cpu",
        observation_dim=2,
        action_dim=2,
    )


def test_ppo_update_runs() -> None:
    algo = PPOLearner(AlgorithmConfig(), _make_training_config())
    observations = torch.zeros(4, 2)
    actions = torch.zeros(4, dtype=torch.long)
    log_probs = torch.zeros(4)
    rewards = torch.ones(4, 1)
    dones = torch.zeros(4, 1)
    values = torch.zeros(5, 1)

    batch = TransitionBatch(
        observations=observations,
        actions=actions,
        log_probs=log_probs,
        rewards=rewards,
        dones=dones,
        values=values,
    )

    update = algo.update(batch)

    assert update.step == 1
    assert isinstance(update.loss, float)


def test_ppo_ensure_advantages_handles_two_dimensional_inputs() -> None:
    algo = PPOLearner(AlgorithmConfig(), _make_training_config())
    batch = TransitionBatch(
        observations=torch.zeros(4, 2),
        actions=torch.zeros(4, dtype=torch.long),
        log_probs=torch.zeros(4),
        rewards=torch.ones(4, 1),
        dones=torch.zeros(4, 1),
        values=torch.zeros(5, 1),
    )

    advantages, returns = algo._ensure_advantages(batch)

    assert advantages.shape == (4, 1)
    assert returns.shape == (4, 1)


def test_ppo_reuses_provided_advantages_and_returns() -> None:
    algo = PPOLearner(AlgorithmConfig(), _make_training_config())
    advantages = torch.full((4,), 42.0)
    returns = torch.full((4,), 24.0)
    batch = TransitionBatch(
        observations=torch.zeros(4, 2),
        actions=torch.zeros(4, dtype=torch.long),
        log_probs=torch.zeros(4),
        rewards=torch.ones(4),
        dones=torch.zeros(4),
        values=torch.zeros(5),
        advantages=advantages,
        returns=returns,
    )

    resolved_advantages, resolved_returns = algo._ensure_advantages(batch)

    assert torch.equal(resolved_advantages, advantages)
    assert torch.equal(resolved_returns, returns)


def test_ppo_ensure_advantages_treats_1d_batch_independently() -> None:
    algo = PPOLearner(AlgorithmConfig(), _make_training_config())
    rewards = torch.tensor([1.0, 0.5, 0.25])
    dones = torch.tensor([False, True, False])
    values = torch.tensor([0.3, 0.7, 0.1])

    batch = TransitionBatch(
        observations=torch.zeros(3, 2),
        actions=torch.zeros(3, dtype=torch.long),
        log_probs=torch.zeros(3),
        rewards=rewards,
        dones=dones,
        values=values,
    )

    advantages, returns = algo._ensure_advantages(batch)

    mask = (~dones).float()
    expected_advantages = rewards + algo._config.gamma * values * mask - values
    expected_returns = expected_advantages + values

    assert advantages.shape == rewards.shape
    assert returns.shape == rewards.shape
    assert torch.allclose(advantages, expected_advantages)
    assert torch.allclose(returns, expected_returns)
