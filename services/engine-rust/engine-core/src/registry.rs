//! Static game registry for compile-time game registration
//! 
//! This module provides a thread-safe registry system that allows games to be
//! registered at compile-time and looked up at runtime by their env_id.

use std::collections::HashMap;
use once_cell::sync::Lazy;
use std::sync::Mutex;

use crate::erased::ErasedGame;

/// Factory function type for creating game instances
pub type GameFactory = fn() -> Box<dyn ErasedGame>;

/// Thread-safe registry mapping env_id to game factory functions
static REGISTRY: Lazy<Mutex<HashMap<String, GameFactory>>> = 
    Lazy::new(|| Mutex::new(HashMap::new()));

/// Register a game with the global registry
/// 
/// This function should typically be called from game crate initialization
/// or using the `register_game!` macro.
/// 
/// # Arguments
/// 
/// * `env_id` - Unique environment identifier (e.g., "tictactoe")
/// * `factory` - Function that creates new instances of the game
/// 
/// # Example
/// 
/// ```rust
/// # use engine_core::registry::*;
/// # use engine_core::erased::ErasedGame;
/// # use engine_core::adapter::GameAdapter;
/// # use engine_core::typed::*;
/// 
/// # struct MyGame;
/// # impl Game for MyGame {
/// #     type State = ();
/// #     type Action = ();
/// #     type Obs = ();
/// #     fn engine_id(&self) -> EngineId { todo!() }
/// #     fn capabilities(&self) -> Capabilities { todo!() }
/// #     fn reset(&mut self, rng: &mut rand_chacha::ChaCha20Rng, hint: &[u8]) -> (Self::State, Self::Obs) { todo!() }
/// #     fn step(&mut self, state: &mut Self::State, action: Self::Action, rng: &mut rand_chacha::ChaCha20Rng) -> (Self::Obs, f32, bool) { todo!() }
/// #     fn encode_state(state: &Self::State, out: &mut Vec<u8>) -> Result<(), crate::typed::EncodeError> { todo!() }
/// #     fn decode_state(buf: &[u8]) -> Result<Self::State, crate::typed::DecodeError> { todo!() }
/// #     fn encode_action(action: &Self::Action, out: &mut Vec<u8>) -> Result<(), crate::typed::EncodeError> { todo!() }
/// #     fn decode_action(buf: &[u8]) -> Result<Self::Action, crate::typed::DecodeError> { todo!() }
/// #     fn encode_obs(obs: &Self::Obs, out: &mut Vec<u8>) -> Result<(), crate::typed::EncodeError> { todo!() }
/// # }
/// 
/// fn my_game_factory() -> Box<dyn ErasedGame> {
///     Box::new(GameAdapter::new(MyGame))
/// }
/// 
/// register_game("my_game".to_string(), my_game_factory);
/// ```
pub fn register_game(env_id: String, factory: GameFactory) {
    let mut registry = REGISTRY.lock().unwrap();
    if registry.contains_key(&env_id) {
        eprintln!("Warning: Overriding existing game registration for '{}'", env_id);
    }
    registry.insert(env_id, factory);
}

/// Create a new game instance by env_id
/// 
/// # Arguments
/// 
/// * `env_id` - Environment identifier to look up
/// 
/// # Returns
/// 
/// Returns `Some(game)` if the env_id is registered, `None` otherwise.
/// 
/// # Example
/// 
/// ```rust
/// # use engine_core::registry::*;
/// 
/// match create_game("tictactoe") {
///     Some(game) => {
///         println!("Created game: {}", game.engine_id().env_id);
///     }
///     None => {
///         println!("Game 'tictactoe' not found");
///     }
/// }
/// ```
pub fn create_game(env_id: &str) -> Option<Box<dyn ErasedGame>> {
    let registry = REGISTRY.lock().unwrap();
    registry.get(env_id).map(|factory| factory())
}

/// Get list of all registered environment IDs
/// 
/// This is useful for debugging and listing available games.
/// 
/// # Returns
/// 
/// A vector of all registered env_id strings.
pub fn list_registered_games() -> Vec<String> {
    let registry = REGISTRY.lock().unwrap();
    registry.keys().cloned().collect()
}

/// Check if a game is registered
/// 
/// # Arguments
/// 
/// * `env_id` - Environment identifier to check
/// 
/// # Returns
/// 
/// `true` if the game is registered, `false` otherwise.
pub fn is_registered(env_id: &str) -> bool {
    let registry = REGISTRY.lock().unwrap();
    registry.contains_key(env_id)
}

/// Clear all registered games (mainly for testing)
/// 
/// This function removes all registered games from the registry.
/// It should primarily be used in test scenarios.
pub fn clear_registry() {
    let mut registry = REGISTRY.lock().unwrap();
    registry.clear();
}

/// Convenience macro for registering games
/// 
/// This macro simplifies the registration process by automatically creating
/// the factory function and calling register_game.
/// 
/// # Example
/// 
/// ```ignore
/// register_game!(TicTacToe, "tictactoe");
/// ```
#[macro_export]
macro_rules! register_game {
    ($game_type:ty, $env_id:expr) => {
        {
            fn factory() -> Box<dyn $crate::erased::ErasedGame> {
                Box::new($crate::adapter::GameAdapter::new(<$game_type>::default()))
            }
            $crate::registry::register_game($env_id.to_string(), factory);
        }
    };
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::typed::{Game, EngineId, Capabilities, Encoding, ActionSpace};
    use crate::adapter::GameAdapter;
    use rand_chacha::ChaCha20Rng;

    // Test game implementation
    #[derive(Default)]
    struct TestGame {
        name: String,
    }
    
    impl TestGame {
        fn new(name: String) -> Self {
            Self { name }
        }
    }
    
    impl Game for TestGame {
        type State = u32;
        type Action = u8;
        type Obs = Vec<f32>;
        
        fn engine_id(&self) -> EngineId {
            EngineId {
                env_id: self.name.clone(),
                build_id: "0.1.0".to_string(),
            }
        }
        
        fn capabilities(&self) -> Capabilities {
            Capabilities {
                id: self.engine_id(),
                encoding: Encoding {
                    state: "u32:v1".to_string(),
                    action: "u8:v1".to_string(),
                    obs: "f32_vec:v1".to_string(),
                    schema_version: 1,
                },
                max_horizon: 100,
                action_space: ActionSpace::Discrete(4),
                preferred_batch: 32,
            }
        }
        
        fn reset(&mut self, _rng: &mut ChaCha20Rng, _hint: &[u8]) -> (Self::State, Self::Obs) {
            (0, vec![0.0])
        }
        
        fn step(&mut self, state: &mut Self::State, action: Self::Action, _rng: &mut ChaCha20Rng) -> (Self::Obs, f32, bool) {
            *state += action as u32;
            (vec![*state as f32], 1.0, *state >= 10)
        }
        
        fn encode_state(state: &Self::State, out: &mut Vec<u8>) -> Result<(), crate::typed::EncodeError> {
            out.extend_from_slice(&state.to_le_bytes());
            Ok(())
        }
        
        fn decode_state(buf: &[u8]) -> Result<Self::State, crate::typed::DecodeError> {
            if buf.len() != 4 {
                return Err(crate::typed::DecodeError::InvalidLength { expected: 4, actual: buf.len() });
            }
            Ok(u32::from_le_bytes(buf.try_into().unwrap()))
        }
        
        fn encode_action(action: &Self::Action, out: &mut Vec<u8>) -> Result<(), crate::typed::EncodeError> {
            out.push(*action);
            Ok(())
        }
        
        fn decode_action(buf: &[u8]) -> Result<Self::Action, crate::typed::DecodeError> {
            if buf.len() != 1 {
                return Err(crate::typed::DecodeError::InvalidLength { expected: 1, actual: buf.len() });
            }
            Ok(buf[0])
        }
        
        fn encode_obs(obs: &Self::Obs, out: &mut Vec<u8>) -> Result<(), crate::typed::EncodeError> {
            for &value in obs {
                out.extend_from_slice(&value.to_le_bytes());
            }
            Ok(())
        }
    }

    #[test]
    fn test_register_and_create_game() {
        // Clear registry for clean test
        clear_registry();
        
        // Register a test game
        fn test_factory() -> Box<dyn ErasedGame> {
            Box::new(GameAdapter::new(TestGame::new("test_game".to_string())))
        }
        
        register_game("test_game".to_string(), test_factory);
        
        // Create game instance
        let game = create_game("test_game");
        assert!(game.is_some());
        
        let game = game.unwrap();
        assert_eq!(game.engine_id().env_id, "test_game");
    }
    
    #[test]
    fn test_create_nonexistent_game() {
        clear_registry();
        
        let game = create_game("nonexistent");
        assert!(game.is_none());
    }
    
    #[test]
    fn test_list_registered_games() {
        clear_registry();
        
        fn factory1() -> Box<dyn ErasedGame> {
            Box::new(GameAdapter::new(TestGame::new("game1".to_string())))
        }
        fn factory2() -> Box<dyn ErasedGame> {
            Box::new(GameAdapter::new(TestGame::new("game2".to_string())))
        }
        
        register_game("game1".to_string(), factory1);
        register_game("game2".to_string(), factory2);
        
        let mut games = list_registered_games();
        games.sort(); // HashMap order is not guaranteed
        
        assert_eq!(games, vec!["game1".to_string(), "game2".to_string()]);
    }
    
    #[test]
    fn test_is_registered() {
        clear_registry();
        
        fn factory() -> Box<dyn ErasedGame> {
            Box::new(GameAdapter::new(TestGame::new("registered_game".to_string())))
        }
        
        assert!(!is_registered("registered_game"));
        
        register_game("registered_game".to_string(), factory);
        assert!(is_registered("registered_game"));
        assert!(!is_registered("unregistered_game"));
    }
    
    #[test]
    fn test_clear_registry() {
        fn factory() -> Box<dyn ErasedGame> {
            Box::new(GameAdapter::new(TestGame::new("temp_game".to_string())))
        }
        
        register_game("temp_game".to_string(), factory);
        assert!(is_registered("temp_game"));
        
        clear_registry();
        assert!(!is_registered("temp_game"));
        assert!(list_registered_games().is_empty());
    }
}