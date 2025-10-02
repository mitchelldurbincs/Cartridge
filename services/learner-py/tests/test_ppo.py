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
