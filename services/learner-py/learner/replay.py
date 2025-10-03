"""Helpers for working with replay service payloads."""

from __future__ import annotations

from typing import Iterable, Mapping, Protocol

import torch

from .datamodel import TransitionBatch

_LOG_PROB_KEY = "log_prob"
_VALUE_KEY = "value"


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


def _stack_1d(tensors: Iterable[torch.Tensor], *, field: str) -> torch.Tensor:
    collected = [t.reshape(-1) for t in tensors]
    if not collected:
        raise ValueError("SampleResponse did not include any transitions")
    first_shape = collected[0].shape
    for tensor in collected[1:]:
        if tensor.shape != first_shape:
            raise ValueError(f"Inconsistent shapes for '{field}' tensors: {tensor.shape} vs {first_shape}")
    return torch.stack(collected).contiguous()


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

    return TransitionBatch(
        observations=_stack_1d(observations, field="observation").to(device="cpu"),
        actions=_stack_1d(actions, field="action").to(device="cpu"),
        log_probs=torch.tensor(log_probs, dtype=torch.float32, device="cpu"),
        rewards=torch.tensor(rewards, dtype=torch.float32, device="cpu"),
        dones=torch.tensor(dones, dtype=torch.bool, device="cpu"),
        values=torch.tensor(values, dtype=torch.float32, device="cpu"),
    )


__all__ = ["SampleResponseLike", "sample_response_to_batch"]
