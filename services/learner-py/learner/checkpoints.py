"""Checkpoint lifecycle management."""

from __future__ import annotations

import asyncio
import hashlib
import json
import re
import shutil
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Mapping

import structlog
import torch
from safetensors.torch import load_file, save_file
from torch import nn, optim

from .config import CheckpointConfig


@dataclass(slots=True)
class CheckpointManifest:
    step: int
    path: Path
    checksum: str
    metadata: Mapping[str, Any]


def _compute_file_sha256(path: Path) -> str:
    """Compute the SHA256 checksum of a file."""

    hash_obj = hashlib.sha256()
    with path.open("rb") as file_obj:
        for chunk in iter(lambda: file_obj.read(1024 * 1024), b""):
            hash_obj.update(chunk)
    return hash_obj.hexdigest()


class CheckpointManager:
    """Persists and manages learner checkpoints."""

    def __init__(self, config: CheckpointConfig, *, base_path: Path | None = None) -> None:
        self._config = config
        self._base_path = base_path or Path(config.bucket)
        self._base_path.mkdir(parents=True, exist_ok=True)
        self._manifests: list[CheckpointManifest] = []
        self._lock = asyncio.Lock()
        self._logger = structlog.get_logger(__name__)

        self._logger.info(
            "CheckpointManager initialized",
            base_path=str(self._base_path),
            interval_steps=config.interval_steps,
            keep_last=config.keep_last
        )

        self._load_existing_manifests()

    async def save(
        self,
        *,
        step: int,
        model: nn.Module,
        optimizer: optim.Optimizer,
        metadata: Mapping[str, Any] | None = None,
    ) -> CheckpointManifest:
        self._logger.info("Starting checkpoint save", step=step, metadata=metadata)

        metadata = {str(key): str(value) for key, value in (metadata or {}).items()}
        checkpoint_dir = self._base_path / f"step_{step}"
        checkpoint_dir.mkdir(parents=True, exist_ok=True)

        self._logger.debug("Created checkpoint directory", path=str(checkpoint_dir))

        model_state = {name: tensor.detach().cpu() for name, tensor in model.state_dict().items()}

        model_params = sum(p.numel() for p in model.parameters())
        self._logger.debug(
            "Prepared model state for saving",
            model_parameters=model_params,
            tensors_count=len(model_state)
        )

        tensor_path = checkpoint_dir / "weights.safetensors"

        try:
            await asyncio.get_running_loop().run_in_executor(
                None, save_file, model_state, str(tensor_path), metadata
            )

            file_size = tensor_path.stat().st_size
            self._logger.info(
                "Saved model weights",
                step=step,
                path=str(tensor_path),
                file_size_mb=round(file_size / (1024 * 1024), 2)
            )

        except Exception as exc:
            self._logger.error(
                "Failed to save model weights",
                step=step,
                path=str(tensor_path),
                error=str(exc)
            )
            raise

        optimizer_path = checkpoint_dir / "optimizer.pt"

        try:
            await asyncio.get_running_loop().run_in_executor(
                None, torch.save, optimizer.state_dict(), str(optimizer_path)
            )
            opt_file_size = optimizer_path.stat().st_size
            self._logger.info(
                "Saved optimizer state",
                step=step,
                path=str(optimizer_path),
                file_size_mb=round(opt_file_size / (1024 * 1024), 2)
            )
        except Exception as exc:
            self._logger.error(
                "Failed to save optimizer state",
                step=step,
                path=str(optimizer_path),
                error=str(exc)
            )
            raise

        loop = asyncio.get_running_loop()
        weights_checksum = await loop.run_in_executor(
            None, _compute_file_sha256, tensor_path
        )

        self._logger.debug(
            "Computed weights checksum",
            path=str(tensor_path),
            checksum=weights_checksum,
        )

        manifest_metadata = {
            **metadata,
            "optimizer": optimizer.__class__.__name__,
            "weights_artifact": tensor_path.name,
            "optimizer_artifact": optimizer_path.name,
            "weights_checksum_sha256": weights_checksum,
        }
        manifest = CheckpointManifest(
            step=step,
            path=tensor_path,
            checksum=weights_checksum,
            metadata=manifest_metadata,
        )
        manifest_path = checkpoint_dir / "MANIFEST.json"
        manifest_path.write_text(json.dumps({"step": step, **manifest_metadata}, indent=2))

        self._logger.debug("Created manifest file", path=str(manifest_path))

        async with self._lock:
            self._manifests.append(manifest)
            self._manifests.sort(key=lambda item: item.step, reverse=True)
            await self._trim_old_checkpoints()

        self._logger.info(
            "Checkpoint saved successfully",
            step=step,
            total_checkpoints=len(self._manifests),
            latest_step=self._manifests[0].step if self._manifests else None
        )

        return manifest

    async def _trim_old_checkpoints(self) -> None:
        trimmed_count = 0
        while len(self._manifests) > self._config.keep_last:
            manifest = self._manifests.pop()
            try:
                shutil.rmtree(manifest.path.parent, ignore_errors=True)
                trimmed_count += 1
                self._logger.debug(
                    "Removed old checkpoint",
                    step=manifest.step,
                    path=str(manifest.path.parent)
                )
            except Exception as exc:
                self._logger.warning(
                    "Failed to remove old checkpoint",
                    step=manifest.step,
                    path=str(manifest.path.parent),
                    error=str(exc)
                )

        if trimmed_count > 0:
            self._logger.info(
                "Trimmed old checkpoints",
                removed_count=trimmed_count,
                remaining_count=len(self._manifests)
            )

    @property
    def latest(self) -> CheckpointManifest | None:
        return self._manifests[0] if self._manifests else None

    def _load_existing_manifests(self) -> None:
        """Discover checkpoint manifests that were created in previous runs."""

        if not self._base_path.exists():
            return

        discovered: list[CheckpointManifest] = []
        pattern = re.compile(r"step_(?P<step>\d+)$")

        for entry in sorted(self._base_path.iterdir(), reverse=True):
            if not entry.is_dir():
                continue

            match = pattern.match(entry.name)
            if match is None:
                continue

            try:
                step = int(match.group("step"))
            except ValueError:
                self._logger.warning(
                    "Failed to parse checkpoint directory step",
                    directory=str(entry),
                )
                continue

            manifest_path = entry / "MANIFEST.json"
            metadata: Mapping[str, Any] = {}
            checksum: str | None = None

            if manifest_path.exists():
                try:
                    data = json.loads(manifest_path.read_text())
                    metadata = {k: v for k, v in data.items() if k != "step"}
                    checksum = data.get("weights_checksum_sha256")
                except (OSError, json.JSONDecodeError) as exc:
                    self._logger.warning(
                        "Failed to read checkpoint manifest",
                        path=str(manifest_path),
                        error=str(exc),
                    )

            weights_path = entry / "weights.safetensors"
            if not weights_path.exists():
                self._logger.warning(
                    "Checkpoint weights file missing",
                    path=str(weights_path),
                )
                continue

            if not checksum:
                try:
                    checksum = _compute_file_sha256(weights_path)
                except OSError as exc:
                    self._logger.warning(
                        "Failed to compute checksum for checkpoint",
                        path=str(weights_path),
                        error=str(exc),
                    )
                    continue

            discovered.append(
                CheckpointManifest(
                    step=step,
                    path=weights_path,
                    checksum=checksum,
                    metadata=metadata,
                )
            )

        if not discovered:
            return

        discovered.sort(key=lambda manifest: manifest.step, reverse=True)
        self._manifests.extend(discovered)

        self._logger.info(
            "Discovered existing checkpoints",
            total_checkpoints=len(self._manifests),
            latest_step=self._manifests[0].step,
        )

    async def restore_latest(self, model: nn.Module, optimizer: optim.Optimizer) -> CheckpointManifest | None:
        """Load the most recent checkpoint into the provided algorithm."""

        async with self._lock:
            manifest = self.latest

        if manifest is None:
            self._logger.info("No checkpoint available to restore")
            return None

        weights_path = manifest.path
        optimizer_path = weights_path.parent / "optimizer.pt"

        loop = asyncio.get_running_loop()

        try:
            state_dict = await loop.run_in_executor(None, lambda: load_file(str(weights_path)))
        except Exception as exc:
            self._logger.error(
                "Failed to load checkpoint weights",
                path=str(weights_path),
                error=str(exc),
            )
            raise

        try:
            optimizer_state = await loop.run_in_executor(
                None, lambda: torch.load(str(optimizer_path), map_location="cpu")
            )
        except Exception as exc:
            self._logger.error(
                "Failed to load optimizer state",
                path=str(optimizer_path),
                error=str(exc),
            )
            raise

        model.load_state_dict(state_dict)
        optimizer.load_state_dict(optimizer_state)

        self._logger.info(
            "Restored checkpoint",
            step=manifest.step,
            weights_path=str(weights_path),
            optimizer_path=str(optimizer_path),
        )

        return manifest


__all__ = ["CheckpointManager", "CheckpointManifest"]
