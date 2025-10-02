"""Configuration parsing and validation for the learner service."""

from __future__ import annotations

import argparse
import json
from pathlib import Path
from typing import Any, Iterable, Mapping, MutableMapping

import yaml
from pydantic import BaseModel, Field, ValidationError, field_validator, model_validator


class ReplayConfig(BaseModel):
    """Configuration for talking to the replay buffer service."""

    endpoint: str = Field(..., description="Target gRPC endpoint for the replay service")
    tls_enabled: bool = Field(False, description="Whether to use TLS when connecting to replay")
    prefetch_depth: int = Field(4, ge=1, description="Number of batches to prefetch asynchronously")
    batch_size: int = Field(..., gt=0, description="Total transitions per sample request")


class AlgorithmConfig(BaseModel):
    """Algorithm specific hyper-parameters. Defaults mirror PPO."""

    name: str = Field("ppo", description="Learner algorithm identifier")
    gamma: float = Field(0.99, ge=0.0, le=1.0)
    gae_lambda: float = Field(0.95, ge=0.0, le=1.0)
    clip_ratio: float = Field(0.2, ge=0.0, le=1.0)
    entropy_coef: float = Field(0.01, ge=0.0)
    value_loss_coef: float = Field(0.5, ge=0.0)
    max_grad_norm: float = Field(0.5, ge=0.0)
    num_epochs: int = Field(4, ge=1)
    minibatch_size: int = Field(128, ge=1)

class TrainingConfig(BaseModel):
    """SGD control parameters."""

    rollout_size: int = Field(1024, ge=1)
    learning_rate: float = Field(3e-4, gt=0.0)
    seed: int = Field(0, ge=0)
    device: str = Field("cuda", description="PyTorch device string to execute on")
    observation_dim: int = Field(..., gt=0)
    action_dim: int = Field(..., gt=0)


class CheckpointConfig(BaseModel):
    """Durable artifact settings."""

    bucket: str = Field(..., description="Object store bucket for checkpoints")
    interval_steps: int = Field(10_000, ge=1)
    keep_last: int = Field(3, ge=1)


class WeightPublisherConfig(BaseModel):
    """Settings for distributing policy weights to actors."""

    backend: str = Field("redis", description="Distribution mechanism identifier")
    endpoint: str = Field(..., description="Endpoint for the weight sink")
    channel: str = Field(..., description="Channel/key used when publishing new weights")


class ControlConfig(BaseModel):
    orchestrator_endpoint: str = Field(..., description="HTTP endpoint for the orchestrator")
    run_id: str = Field(..., description="Unique identifier for the active run")
    heartbeat_interval_seconds: int = Field(30, ge=5)


class LearnerConfig(BaseModel):
    """Top level configuration model for the learner service."""

    replay: ReplayConfig
    training: TrainingConfig
    algorithm: AlgorithmConfig = Field(default_factory=AlgorithmConfig)
    checkpoints: CheckpointConfig
    weights: WeightPublisherConfig
    control: ControlConfig

    @field_validator("algorithm")
    @classmethod
    def _validate_algo(cls, algo: AlgorithmConfig) -> AlgorithmConfig:
        if algo.name.lower() != "ppo":
            raise ValueError(
                "Only the PPO algorithm is supported in the initial implementation; "
                f"received '{algo.name}'."
            )
        return algo

    @model_validator(mode="after")
    def _validate_training_constraints(self) -> "LearnerConfig":
        if self.algorithm.minibatch_size > self.training.rollout_size:
            raise ValueError("minibatch_size cannot exceed rollout_size")
        return self


def parse_args(argv: Iterable[str] | None = None) -> argparse.Namespace:
    """Parse CLI arguments for the learner entrypoint."""

    parser = argparse.ArgumentParser(description="Run the Cartridge learner service")
    parser.add_argument("--config", type=Path, help="Path to the learner configuration file", required=True)
    parser.add_argument(
        "--override",
        type=str,
        nargs="*",
        default=[],
        help="Override configuration values (dot.separated=value)",
    )
    return parser.parse_args(list(argv) if argv is not None else None)


def load_config(path: Path, *, overrides: Iterable[str] | None = None) -> LearnerConfig:
    """Load a :class:`LearnerConfig` from ``path`` applying optional overrides."""

    raw = _read_config_file(path)
    mutable = _ensure_mutable(raw)
    for override in overrides or []:
        _apply_override(mutable, override)
    try:
        return LearnerConfig.model_validate(mutable)
    except ValidationError as exc:  # pragma: no cover - surfaced directly to caller
        raise ValueError(f"Invalid learner configuration: {exc}") from exc


def _read_config_file(path: Path) -> Mapping[str, Any]:
    if not path.exists():
        raise FileNotFoundError(f"Configuration file '{path}' does not exist")
    content = path.read_text()
    if path.suffix.lower() in {".yaml", ".yml"}:
        data = yaml.safe_load(content)
    else:
        data = json.loads(content)
    if not isinstance(data, Mapping):
        raise ValueError("Configuration root must be a mapping")
    return data


def _ensure_mutable(data: Mapping[str, Any]) -> MutableMapping[str, Any]:
    return json.loads(json.dumps(data))


def _apply_override(target: MutableMapping[str, Any], assignment: str) -> None:
    if "=" not in assignment:
        raise ValueError(f"Invalid override '{assignment}', expected key=value format")
    key, raw_value = assignment.split("=", 1)
    keys = key.split(".")
    cursor: MutableMapping[str, Any] = target
    for part in keys[:-1]:
        node = cursor.setdefault(part, {})
        if not isinstance(node, MutableMapping):
            raise ValueError(f"Cannot override '{key}' because '{part}' is not a mapping")
        cursor = node
    cursor[keys[-1]] = _coerce_override_value(raw_value)


def _coerce_override_value(raw: str) -> Any:
    lowered = raw.lower()
    if lowered in {"true", "false"}:
        return lowered == "true"
    try:
        return int(raw)
    except ValueError:
        try:
            return float(raw)
        except ValueError:
            return raw


__all__ = [
    "AlgorithmConfig",
    "CheckpointConfig",
    "ControlConfig",
    "LearnerConfig",
    "ReplayConfig",
    "TrainingConfig",
    "WeightPublisherConfig",
    "load_config",
    "parse_args",
]
