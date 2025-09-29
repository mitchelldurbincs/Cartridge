//! Game registry initialization
//! 
//! This module initializes the global game registry by registering all available games.

use engine_core::{GameAdapter, register_game};
use games_tictactoe::TicTacToe;

/// Initialize the global game registry with all available games
/// 
/// This function should be called once at startup to register all game implementations
/// with the global registry.
pub fn initialize_registry() {
    // Register TicTacToe game
    register_game(
        "tictactoe".to_string(), 
        || Box::new(GameAdapter::new(TicTacToe::new()))
    );
    
    println!("Initialized game registry with {} games", 
             engine_core::registry::list_registered_games().len());
    
    // Log registered games
    for game_id in engine_core::registry::list_registered_games() {
        println!("  - {}", game_id);
    }
}