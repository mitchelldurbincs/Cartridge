import pytest
import torch
from torch import nn, optim

from learner.checkpoints import CheckpointConfig, CheckpointManager


def _linear_model() -> nn.Linear:
    model = nn.Linear(1, 1, bias=False)
    with torch.no_grad():
        model.weight.fill_(0.0)
    return model


async def _save_checkpoint(manager: CheckpointManager, step: int, value: float) -> dict:
    model = _linear_model()
    with torch.no_grad():
        model.weight.fill_(value)
    optimizer = optim.Adam(model.parameters(), lr=0.01)
    await manager.save(step=step, model=model, optimizer=optimizer, metadata={"loss": value})
    return optimizer.state_dict()


@pytest.mark.asyncio
async def test_checkpoint_manager_discovers_existing(tmp_path) -> None:
    bucket = tmp_path / "ckpts"
    config = CheckpointConfig(bucket=str(bucket), interval_steps=5, keep_last=5)
    creator = CheckpointManager(config)

    await _save_checkpoint(creator, step=10, value=1.0)
    await _save_checkpoint(creator, step=20, value=2.0)

    reloaded = CheckpointManager(config)
    latest = reloaded.latest

    assert latest is not None
    assert latest.step == 20
    assert latest.metadata.get("loss") == "2.0"


@pytest.mark.asyncio
async def test_restore_latest_loads_model_and_optimizer(tmp_path) -> None:
    bucket = tmp_path / "ckpts"
    config = CheckpointConfig(bucket=str(bucket), interval_steps=5, keep_last=2)
    manager = CheckpointManager(config)

    expected_state = await _save_checkpoint(manager, step=7, value=3.14)

    restored_model = _linear_model()
    restored_optimizer = optim.Adam(restored_model.parameters(), lr=0.01)

    manifest = await manager.restore_latest(restored_model, restored_optimizer)

    assert manifest is not None
    assert manifest.step == 7

    with torch.no_grad():
        assert torch.isclose(restored_model.weight.squeeze(), torch.tensor(3.14))

    assert restored_optimizer.state_dict() == expected_state
