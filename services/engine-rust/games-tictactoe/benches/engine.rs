use criterion::{criterion_group, criterion_main, BatchSize, Criterion};
use engine_core::Game;
use games_tictactoe::{Action, Observation, State, TicTacToe};
use rand::SeedableRng;
use rand_chacha::ChaCha20Rng;

fn bench_reset(c: &mut Criterion) {
    let mut group = c.benchmark_group("tictactoe_reset");
    group.bench_function("reset", |b| {
        let mut game = TicTacToe::new();
        b.iter_batched(
            || ChaCha20Rng::seed_from_u64(42),
            |mut rng| {
                let _ = game.reset(&mut rng, &[]);
            },
            BatchSize::SmallInput,
        );
    });
    group.finish();
}

fn bench_step(c: &mut Criterion) {
    let mut group = c.benchmark_group("tictactoe_step");
    group.bench_function("step_center", |b| {
        let mut game = TicTacToe::new();
        let mut rng = ChaCha20Rng::seed_from_u64(7);
        let (mut base_state, _) = game.reset(&mut rng, &[]);
        b.iter_batched(
            || base_state,
            |mut state| {
                let action = Action::Place(4);
                let _ = game.step(&mut state, action, &mut rng);
            },
            BatchSize::SmallInput,
        );
    });
    group.finish();
}

fn bench_encode_decode(c: &mut Criterion) {
    let mut group = c.benchmark_group("tictactoe_encoding");

    group.bench_function("state_roundtrip", |b| {
        let state = State::new();
        b.iter_batched(
            || Vec::with_capacity(16),
            |mut buffer| {
                TicTacToe::encode_state(&state, &mut buffer).unwrap();
                let _ = TicTacToe::decode_state(&buffer).unwrap();
                buffer
            },
            BatchSize::SmallInput,
        );
    });

    group.bench_function("observation_encode", |b| {
        let obs = Observation::from_state(&State::new());
        b.iter_batched(
            || Vec::with_capacity(128),
            |mut buffer| {
                TicTacToe::encode_obs(&obs, &mut buffer).unwrap();
                buffer
            },
            BatchSize::SmallInput,
        );
    });

    group.finish();
}

criterion_group!(benches, bench_reset, bench_step, bench_encode_decode);
criterion_main!(benches);
