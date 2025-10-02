"""Algorithm registry for the learner."""

from .registry import AlgorithmFactory, AlgorithmProtocol, get_algorithm

__all__ = ["AlgorithmFactory", "AlgorithmProtocol", "get_algorithm"]
