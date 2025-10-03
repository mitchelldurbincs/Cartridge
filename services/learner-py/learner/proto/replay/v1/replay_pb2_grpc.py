"""gRPC client and server classes for replay.v1.Replay service."""
from __future__ import annotations

import grpc

from . import replay_pb2 as replay__pb2


class ReplayStub:
    """Minimal stub for the Replay service supporting unary calls."""

    def __init__(self, channel: grpc.Channel | grpc.aio.Channel) -> None:  # type: ignore[name-defined]
        """Initialise the stub bound to ``channel``."""

        self.Sample = channel.unary_unary(  # type: ignore[no-untyped-call]
            "/replay.v1.Replay/Sample",
            request_serializer=replay__pb2.SampleRequest.SerializeToString,
            response_deserializer=replay__pb2.SampleResponse.FromString,
        )


__all__ = ["ReplayStub"]
