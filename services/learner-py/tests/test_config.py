from __future__ import annotations

import json
from pathlib import Path

from learner.config import load_config


def test_load_config(tmp_path: Path) -> None:
    config_path = tmp_path / "config.json"
    config_payload = {
        "replay": {"endpoint": "localhost:50051", "batch_size": 32},
        "training": {
            "rollout_size": 128,
            "learning_rate": 0.0003,
            "seed": 42,
            "device": "cpu",
            "observation_dim": 4,
            "action_dim": 2,
        },
        "algorithm": {"name": "ppo"},
        "checkpoints": {"bucket": str(tmp_path / "ckpts"), "interval_steps": 10, "keep_last": 2},
        "weights": {"backend": "redis", "endpoint": "redis://localhost:6379", "channel": "weights"},
        "control": {
            "orchestrator_endpoint": "http://localhost:8000",
            "run_id": "test-run",
            "heartbeat_interval_seconds": 30,
        },
    }
    config_path.write_text(json.dumps(config_payload))

    config = load_config(config_path, overrides=["training.learning_rate=0.001"])

    assert config.training.learning_rate == 0.001
    assert config.training.observation_dim == 4
    assert config.replay.prefetch_depth == 4
