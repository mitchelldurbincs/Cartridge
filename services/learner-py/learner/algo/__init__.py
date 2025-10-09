"""Algorithm registry for the learner.

This module exposes the public registry API and ensures that all algorithm
implementations are imported so their registration side effects run at import
time. Without importing the implementation modules, the registry remains empty
and configuration values such as ``algorithm="ppo"`` fail with ``ValueError``.
"""

from .registry import AlgorithmFactory, AlgorithmProtocol, get_algorithm

# Import algorithm implementations so that they register themselves with the
# global registry as a side effect. The imported names are intentionally unused
# which is why we assign them to a dummy alias.
from . import ppo as _ppo  # noqa: F401


__all__ = ["AlgorithmFactory", "AlgorithmProtocol", "get_algorithm"]
