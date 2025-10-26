# Actor Throughput Benchmarks

This directory contains performance benchmarks for the actor service to measure throughput and latency characteristics.

## Overview

The benchmarks measure the actor's ability to run game episodes using mock engine and replay services. This isolates the actor's performance from external service dependencies.

## Benchmarks

### Episode Throughput (`benchmark_episode_throughput`)

Measures how many episodes the actor can process per second when running 10 episodes consecutively with different episode lengths:

- **10 steps/episode**: ~43 episodes/second
- **50 steps/episode**: ~30 episodes/second
- **100 steps/episode**: ~22 episodes/second

### Single Episode Latency (`benchmark_single_episode`)

Measures the time to complete a single episode with different step counts:

- **10 steps**: ~208 ms/episode
- **50 steps**: ~219 ms/episode
- **100 steps**: ~232 ms/episode

## Running the Benchmarks

```bash
# Run all benchmarks
cargo bench --bench throughput

# Run specific benchmark
cargo bench --bench throughput -- actor_episode_throughput

# Run with custom timing
cargo bench --bench throughput -- --warm-up-time 3 --measurement-time 10
```

## Benchmark Setup

The benchmarks use:

- **MockEngine**: Fast in-memory game engine that simulates Reset/Step operations
- **MockReplay**: No-op replay service that accepts transitions without storage
- **Isolated Testing**: gRPC servers run on localhost with random ports

This setup removes network and storage bottlenecks to measure pure actor performance.

## Results Location

Benchmark results are saved to:
- `target/criterion/` - Detailed HTML reports with graphs
- Console output - Summary statistics

## Interpreting Results

- **Throughput (elements/s)**: Number of episodes completed per second
- **Time**: Latency per benchmark iteration
- **Outliers**: Measurements that deviate significantly from the median

## Performance Observations

1. Episode length has moderate impact on throughput (43 → 22 eps/sec from 10 → 100 steps)
2. Most overhead comes from gRPC communication and actor initialization (~200ms baseline)
3. Per-step overhead is relatively low (~3-4ms per additional step)

## Future Improvements

- Add benchmarks for different batch sizes
- Measure transition buffer flush performance
- Test with realistic network latency
- Benchmark different action space types
- Add memory usage profiling
