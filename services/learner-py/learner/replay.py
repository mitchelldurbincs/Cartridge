"""Helpers for working with replay service payloads."""

from __future__ import annotations

import logging
from typing import Iterable, Mapping, Protocol

import torch

from .datamodel import TransitionBatch

_LOG_PROB_KEY = "log_prob"
_VALUE_KEY = "value"
_LOGGER = logging.getLogger(__name__)


class TransitionLike(Protocol):
    observation: bytes
    action: bytes
    reward: float
    done: bool
    metadata: Mapping[str, str]


class SampleResponseLike(Protocol):
    transitions: Iterable[TransitionLike]


def _tensor_from_bytes(blob: bytes, *, dtype: torch.dtype, field: str) -> torch.Tensor:
    if not blob:
        raise ValueError(f"Transition field '{field}' is empty")
    # ``torch.frombuffer`` avoids intermediate copies; clone to own the memory afterwards.
    tensor = torch.frombuffer(memoryview(blob), dtype=dtype)  # type: ignore[arg-type]
    if tensor.numel() == 0:
        raise ValueError(f"Transition field '{field}' decoded to an empty tensor")
    return tensor.clone()


def _validate_tensor_compatibility(tensors: list[torch.Tensor], field: str) -> tuple[torch.Size, int]:
    """Validate that tensors can be stacked and return expected shape info."""
    if not tensors:
        raise ValueError(f"No tensors provided for field '{field}'")

    # Check original shapes before reshaping
    first_original_shape = tensors[0].shape
    first_numel = tensors[0].numel()

    for i, tensor in enumerate(tensors[1:], 1):
        if tensor.numel() != first_numel:
            raise ValueError(
                f"Incompatible tensor sizes for '{field}': transition {i} has {tensor.numel()} elements, "
                f"but transition 0 has {first_numel} elements (shapes: {tensor.shape} vs {first_original_shape})"
            )

    return first_original_shape, first_numel


def _stack_tensors(tensors: list[torch.Tensor], *, field: str, target_shape: torch.Size | None = None) -> torch.Tensor:
    """Stack tensors with improved shape validation and optional reshaping."""
    if not tensors:
        raise ValueError(f"No tensors to stack for field '{field}'")

    # Validate compatibility
    original_shape, numel = _validate_tensor_compatibility(tensors, field)

    if target_shape is None:
        # Default behavior: flatten each tensor
        reshaped = [t.reshape(-1) for t in tensors]
        _LOGGER.debug(f"Stacking {len(tensors)} tensors for '{field}', flattened shape: {reshaped[0].shape}")
    else:
        # Reshape to specific target shape
        if torch.Size(target_shape).numel() != numel:
            raise ValueError(
                f"Target shape {target_shape} is incompatible with tensor size {numel} for field '{field}'"
            )
        reshaped = [t.reshape(target_shape) for t in tensors]
        _LOGGER.debug(f"Stacking {len(tensors)} tensors for '{field}', target shape: {target_shape}")

    return torch.stack(reshaped).contiguous()


def sample_response_to_batch(response: SampleResponseLike) -> TransitionBatch:
    """Convert a :class:`SampleResponse` into a :class:`TransitionBatch` on CPU."""

    transitions = list(response.transitions)
    if not transitions:
        raise ValueError("SampleResponse contained no transitions")

    observations = []
    actions = []
    log_probs: list[float] = []
    rewards: list[float] = []
    dones: list[bool] = []
    values: list[float] = []

    for transition in transitions:
        observations.append(
            _tensor_from_bytes(transition.observation, dtype=torch.float32, field="observation")
        )
        actions.append(_tensor_from_bytes(transition.action, dtype=torch.float32, field="action"))
        rewards.append(float(transition.reward))
        dones.append(bool(transition.done))
        metadata = transition.metadata
        if _LOG_PROB_KEY not in metadata or _VALUE_KEY not in metadata:
            raise ValueError("Transition metadata missing log-probability or value estimate")
        log_probs.append(float(metadata[_LOG_PROB_KEY]))
        values.append(float(metadata[_VALUE_KEY]))

    # Validate and stack tensor fields with improved error handling
    try:
        obs_tensor = _stack_tensors(observations, field="observation").to(device="cpu")
        action_tensor = _stack_tensors(actions, field="action").to(device="cpu")
    except ValueError as e:
        _LOGGER.error("Failed to convert replay response to batch: %s", e)
        raise ValueError(f"Replay data conversion failed: {e}") from e

    # Create scalar tensors
    log_probs_tensor = torch.tensor(log_probs, dtype=torch.float32, device="cpu")
    rewards_tensor = torch.tensor(rewards, dtype=torch.float32, device="cpu")
    dones_tensor = torch.tensor(dones, dtype=torch.bool, device="cpu")
    values_tensor = torch.tensor(values, dtype=torch.float32, device="cpu")

    _LOGGER.debug(
        "Created TransitionBatch: obs=%s, actions=%s, batch_size=%d",
        obs_tensor.shape, action_tensor.shape, len(transitions)
    )

    return TransitionBatch(
        observations=obs_tensor,
        actions=action_tensor,
        log_probs=log_probs_tensor,
        rewards=rewards_tensor,
        dones=dones_tensor,
        values=values_tensor,
    )


__all__ = ["SampleResponseLike", "sample_response_to_batch"]
