"""Checkpoint lifecycle management."""

from __future__ import annotations

import asyncio
import json
import shutil
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Mapping

from safetensors.torch import save_file
from torch import nn, optim

from .config import CheckpointConfig


@dataclass(slots=True)
class CheckpointManifest:
    step: int
    path: Path
    checksum: str
    metadata: Mapping[str, Any]


class CheckpointManager:
    """Persists and manages learner checkpoints."""

    def __init__(self, config: CheckpointConfig, *, base_path: Path | None = None) -> None:
        self._config = config
        self._base_path = base_path or Path(config.bucket)
        self._base_path.mkdir(parents=True, exist_ok=True)
        self._manifests: list[CheckpointManifest] = []
        self._lock = asyncio.Lock()

    async def save(
        self,
        *,
        step: int,
        model: nn.Module,
        optimizer: optim.Optimizer,
        metadata: Mapping[str, Any] | None = None,
    ) -> CheckpointManifest:
        metadata = {str(key): str(value) for key, value in (metadata or {}).items()}
        checkpoint_dir = self._base_path / f"step_{step}"
        checkpoint_dir.mkdir(parents=True, exist_ok=True)
        tensors = {
            "model": model.state_dict(),
            "optimizer": optimizer.state_dict(),
        }
        tensor_path = checkpoint_dir / "weights.safetensors"
        await asyncio.get_running_loop().run_in_executor(
            None, save_file, tensors, str(tensor_path), metadata
        )
        manifest_metadata = {**metadata, "optimizer": "adam", "artifact": tensor_path.name}
        manifest = CheckpointManifest(
            step=step,
            path=tensor_path,
            checksum="",  # TODO: implement checksums once wiring with object store is added
            metadata=manifest_metadata,
        )
        manifest_path = checkpoint_dir / "MANIFEST.json"
        manifest_path.write_text(json.dumps({"step": step, **manifest_metadata}, indent=2))

        async with self._lock:
            self._manifests.append(manifest)
            self._manifests.sort(key=lambda item: item.step, reverse=True)
            await self._trim_old_checkpoints()
        return manifest

    async def _trim_old_checkpoints(self) -> None:
        while len(self._manifests) > self._config.keep_last:
            manifest = self._manifests.pop()
            shutil.rmtree(manifest.path.parent, ignore_errors=True)

    @property
    def latest(self) -> CheckpointManifest | None:
        return self._manifests[0] if self._manifests else None


__all__ = ["CheckpointManager", "CheckpointManifest"]
