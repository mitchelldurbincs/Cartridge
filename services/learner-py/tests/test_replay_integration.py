"""Integration tests for replay client gRPC functionality."""

from __future__ import annotations

import asyncio
import logging
import struct
from typing import AsyncIterator
from unittest.mock import AsyncMock, Mock, patch
import pytest
import torch

from learner.config import ReplayConfig
from learner.datamodel import TransitionBatch
from learner.metrics import MetricsRegistry
from learner.replay import sample_response_to_batch
from learner.replay_client import ReplayClient


class MockTransition:
    """Mock transition for testing."""

    def __init__(self, observation: bytes, action: bytes, reward: float, done: bool, metadata: dict[str, str]):
        self.observation = observation
        self.action = action
        self.reward = reward
        self.done = done
        self.metadata = metadata


class MockSampleResponse:
    """Mock gRPC sample response."""

    def __init__(self, transitions: list[MockTransition]):
        self.transitions = transitions


@pytest.fixture
def sample_response_missing_metadata() -> MockSampleResponse:
    """Sample response containing a transition without log-prob/value metadata."""

    obs_data = struct.pack("ff", 1.0, 2.0)
    action_data = struct.pack("f", 0.0)
    transition = MockTransition(
        obs_data,
        action_data,
        reward=1.0,
        done=False,
        metadata={},
    )

    return MockSampleResponse([transition])


class TestReplayConversion:
    """Test replay data conversion logic."""

    def test_single_transition_conversion(self):
        """Test converting a single transition to batch."""
        # Create test data
        obs_data = struct.pack('ff', 1.0, 2.0)  # 2 floats
        action_data = struct.pack('f', 0.0)     # 1 float

        transition = MockTransition(
            observation=obs_data,
            action=action_data,
            reward=1.5,
            done=False,
            metadata={'log_prob': '-0.5', 'value': '2.3'}
        )

        response = MockSampleResponse([transition])
        batch = sample_response_to_batch(response)

        assert isinstance(batch, TransitionBatch)
        assert batch.observations.shape == (1, 2)  # [batch_size, obs_dim]
        assert batch.actions.shape == (1, 1)       # [batch_size, action_dim]
        assert batch.log_probs.shape == (1,)
        assert batch.rewards.shape == (1,)
        assert batch.dones.shape == (1,)
        assert batch.values.shape == (1,)

        # Verify values
        assert torch.allclose(batch.observations[0], torch.tensor([1.0, 2.0]))
        assert torch.allclose(batch.actions[0], torch.tensor([0.0]))
        assert batch.rewards[0].item() == 1.5
        assert batch.log_probs[0].item() == -0.5
        assert batch.values[0].item() == 2.3
        assert batch.dones[0].item() is False

    def test_batch_conversion(self):
        """Test converting multiple transitions to batch."""
        obs_data1 = struct.pack('ff', 1.0, 2.0)
        obs_data2 = struct.pack('ff', 3.0, 4.0)
        action_data1 = struct.pack('f', 0.0)
        action_data2 = struct.pack('f', 1.0)

        transitions = [
            MockTransition(obs_data1, action_data1, 1.0, False, {'log_prob': '-0.1', 'value': '1.0'}),
            MockTransition(obs_data2, action_data2, 2.0, True, {'log_prob': '-0.2', 'value': '2.0'}),
        ]

        response = MockSampleResponse(transitions)
        batch = sample_response_to_batch(response)

        assert batch.observations.shape == (2, 2)
        assert batch.actions.shape == (2, 1)
        assert len(batch.log_probs) == 2
        assert len(batch.rewards) == 2

    def test_inconsistent_shapes_error(self):
        """Test that inconsistent tensor shapes raise appropriate errors."""
        obs_data1 = struct.pack('ff', 1.0, 2.0)    # 2 floats
        obs_data2 = struct.pack('fff', 3.0, 4.0, 5.0)  # 3 floats - different size!
        action_data = struct.pack('f', 0.0)

        transitions = [
            MockTransition(obs_data1, action_data, 1.0, False, {'log_prob': '-0.1', 'value': '1.0'}),
            MockTransition(obs_data2, action_data, 2.0, True, {'log_prob': '-0.2', 'value': '2.0'}),
        ]

        response = MockSampleResponse(transitions)

        with pytest.raises(ValueError, match="Incompatible tensor sizes"):
            sample_response_to_batch(response)

    def test_missing_metadata_defaults(self, sample_response_missing_metadata, caplog: pytest.LogCaptureFixture):
        """Missing metadata should default to zeros while logging a warning."""

        with caplog.at_level(logging.WARNING):
            batch = sample_response_to_batch(sample_response_missing_metadata)

        assert isinstance(batch, TransitionBatch)
        assert batch.log_probs.shape == (1,)
        assert batch.values.shape == (1,)
        assert batch.log_probs[0].item() == pytest.approx(0.0)
        assert batch.values[0].item() == pytest.approx(0.0)
        assert "missing log-probability/value" in caplog.text


class TestReplayClientIntegration:
    """Test ReplayClient integration with mocked gRPC."""

    @pytest.fixture
    def config(self) -> ReplayConfig:
        return ReplayConfig(
            endpoint="localhost:8080",
            tls_enabled=False,
            prefetch_depth=2,
            batch_size=4
        )

    @pytest.fixture
    def metrics(self) -> MetricsRegistry:
        return MetricsRegistry()

    async def test_custom_sampler(self, config: ReplayConfig):
        """Test ReplayClient with a custom sampler function."""

        def mock_sampler() -> TransitionBatch:
            return TransitionBatch(
                observations=torch.randn(2, 4),
                actions=torch.randint(0, 2, (2,)),
                log_probs=torch.randn(2),
                rewards=torch.randn(2),
                dones=torch.zeros(2, dtype=torch.bool),
                values=torch.randn(2),
            )

        client = ReplayClient(config, sampler=mock_sampler)

        async with client:
            batch = await client.sample()
            assert isinstance(batch, TransitionBatch)
            assert batch.observations.shape == (2, 4)

    @patch('learner.replay_client.grpc.aio.insecure_channel')
    @patch('learner.replay_client.importlib.import_module')
    async def test_grpc_connection_reuse(self, mock_import, mock_channel, config: ReplayConfig, metrics: MetricsRegistry):
        """Test that gRPC connections are reused properly."""

        # Mock the proto modules
        mock_pb2 = Mock()
        mock_pb2_grpc = Mock()
        mock_import.side_effect = lambda name: mock_pb2 if 'replay_pb2' in name and 'grpc' not in name else mock_pb2_grpc

        # Mock the channel and stub
        mock_channel_instance = AsyncMock()
        mock_channel.return_value = mock_channel_instance

        mock_stub = AsyncMock()
        mock_pb2_grpc.ReplayStub.return_value = mock_stub

        # Mock the response
        mock_response = MockSampleResponse([
            MockTransition(
                struct.pack('ff', 1.0, 2.0),
                struct.pack('f', 0.0),
                1.0, False,
                {'log_prob': '-0.1', 'value': '1.0'}
            )
        ])
        mock_stub.Sample.return_value = mock_response

        client = ReplayClient(config, metrics=metrics)

        # Make two sequential gRPC calls
        await client._grpc_sampler()
        await client._grpc_sampler()

        # Channel should be created only once
        mock_channel.assert_called_once()
        # Stub should be called twice
        assert mock_stub.Sample.call_count == 2

        await client.stop()

    @patch('learner.replay_client.grpc.aio.insecure_channel')
    @patch('learner.replay_client.importlib.import_module')
    async def test_grpc_error_handling(self, mock_import, mock_channel, config: ReplayConfig, metrics: MetricsRegistry):
        """Test gRPC error handling and connection recovery."""

        # Mock the proto modules
        mock_pb2 = Mock()
        mock_pb2_grpc = Mock()
        mock_import.side_effect = lambda name: mock_pb2 if 'replay_pb2' in name and 'grpc' not in name else mock_pb2_grpc

        # Mock the channel and stub
        mock_channel_instance = AsyncMock()
        mock_channel.return_value = mock_channel_instance

        mock_stub = AsyncMock()
        mock_pb2_grpc.ReplayStub.return_value = mock_stub

        # First call fails, second succeeds
        import grpc
        mock_stub.Sample.side_effect = [
            grpc.RpcError("Connection failed"),
            MockSampleResponse([
                MockTransition(
                    struct.pack('ff', 1.0, 2.0),
                    struct.pack('f', 0.0),
                    1.0, False,
                    {'log_prob': '-0.1', 'value': '1.0'}
                )
            ])
        ]

        client = ReplayClient(config, metrics=metrics)

        # First call should fail
        with pytest.raises(grpc.RpcError):
            await client._grpc_sampler()

        # Channel should be closed and reopened for second call
        # Second call should succeed
        response = await client._grpc_sampler()
        assert len(response.transitions) == 1

        await client.stop()

    async def test_prefetch_loop_with_custom_sampler(self, config: ReplayConfig):
        """Test the prefetch loop with a custom sampler."""

        call_count = 0

        async def async_sampler() -> TransitionBatch:
            nonlocal call_count
            call_count += 1
            await asyncio.sleep(0.01)  # Simulate async work
            return TransitionBatch(
                observations=torch.randn(1, 2),
                actions=torch.randint(0, 2, (1,)),
                log_probs=torch.randn(1),
                rewards=torch.randn(1),
                dones=torch.zeros(1, dtype=torch.bool),
                values=torch.randn(1),
            )

        client = ReplayClient(config, sampler=async_sampler)

        async with client:
            # Get a few samples
            batch1 = await client.sample()
            batch2 = await client.sample()

            assert isinstance(batch1, TransitionBatch)
            assert isinstance(batch2, TransitionBatch)
            assert call_count >= 2  # Should have called sampler at least twice

    async def test_device_conversion(self, config: ReplayConfig):
        """Test that tensors are properly converted to specified device."""

        def cuda_sampler() -> TransitionBatch:
            # Create batch on CPU
            return TransitionBatch(
                observations=torch.randn(2, 4),
                actions=torch.randint(0, 2, (2,)),
                log_probs=torch.randn(2),
                rewards=torch.randn(2),
                dones=torch.zeros(2, dtype=torch.bool),
                values=torch.randn(2),
            )

        client = ReplayClient(config, sampler=cuda_sampler)

        async with client:
            batch = await client.sample()

            # All tensors should be on CPU (default device for sample_response_to_batch)
            assert batch.observations.device.type == 'cpu'
            assert batch.actions.device.type == 'cpu'
            assert batch.log_probs.device.type == 'cpu'


if __name__ == "__main__":
    # Run tests
    pytest.main([__file__, "-v"])