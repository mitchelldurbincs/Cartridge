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
