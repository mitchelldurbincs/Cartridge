//! Game registry initialization
//!
//! This module initializes the global game registry by registering all available games.

use engine_core::{register_game, GameAdapter};
use games_tictactoe::TicTacToe;
use metrics::gauge;
use tracing::{debug, info};

/// Initialize the global game registry with all available games
///
/// This function should be called once at startup to register all game implementations
/// with the global registry.
pub fn initialize_registry() {
    // Register TicTacToe game
    register_game("tictactoe".to_string(), || {
        Box::new(GameAdapter::new(TicTacToe::new()))
    });

    let games = engine_core::registry::list_registered_games();
    gauge!("engine_registry_games", games.len() as f64);
    info!(count = games.len(), "Initialized game registry");

    // Log registered games
    for game_id in games {
        debug!(%game_id, "Registered game");
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use engine_core::registry::{clear_registry, create_game, is_registered, list_registered_games};
    use once_cell::sync::Lazy;
    use std::sync::Mutex;

    static REGISTRY_LOCK: Lazy<Mutex<()>> = Lazy::new(|| Mutex::new(()));

    #[test]
    fn test_initialize_registry_registers_tictactoe() {
        let _guard = REGISTRY_LOCK.lock().unwrap();
        // Clear registry for clean test
        clear_registry();

        // Initialize registry
        initialize_registry();

        // Verify tictactoe is registered
        assert!(is_registered("tictactoe"));
    }

    #[test]
    fn test_initialize_registry_creates_working_game() {
        let _guard = REGISTRY_LOCK.lock().unwrap();
        clear_registry();
        initialize_registry();

        // Verify we can create a game instance
        let game = create_game("tictactoe");
        assert!(game.is_some());

        // Verify the game has the correct env_id
        let game = game.unwrap();
        assert_eq!(game.engine_id().env_id, "tictactoe");
    }

    #[test]
    fn test_initialize_registry_lists_correct_games() {
        let _guard = REGISTRY_LOCK.lock().unwrap();
        clear_registry();
        initialize_registry();

        let games = list_registered_games();
        assert_eq!(games.len(), 1);
        assert!(games.contains(&"tictactoe".to_string()));
    }

    #[test]
    fn test_initialize_registry_idempotent() {
        let _guard = REGISTRY_LOCK.lock().unwrap();
        clear_registry();

        // Initialize twice
        initialize_registry();
        initialize_registry();

        // Should only have one game registered (overridden)
        let games = list_registered_games();
        assert_eq!(games.len(), 1);
        assert!(is_registered("tictactoe"));
    }

    #[test]
    fn test_initialize_registry_creates_functional_game() {
        let _guard = REGISTRY_LOCK.lock().unwrap();
        use rand::SeedableRng;
        use rand_chacha::ChaCha20Rng;

        clear_registry();
        initialize_registry();

        let mut game = create_game("tictactoe").expect("Game should be created");
        let mut rng = ChaCha20Rng::seed_from_u64(42);

        // Test that we can reset the game
        let mut state_buf = Vec::new();
        let mut obs_buf = Vec::new();
        let result = game.reset(42, &[], &mut state_buf, &mut obs_buf);

        assert!(result.is_ok());
        assert!(!state_buf.is_empty());
        assert!(!obs_buf.is_empty());
    }
}
