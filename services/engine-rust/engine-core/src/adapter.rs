//! Adapter layer converting typed games to erased interface
//! 
//! This module provides the `GameAdapter` struct that automatically converts
//! any typed `Game` implementation to the `ErasedGame` interface, handling
//! all encoding/decoding and random number generation management.

use rand_chacha::ChaCha20Rng;
use rand::SeedableRng;

use crate::typed::{Game, EngineId, Capabilities};
use crate::erased::{ErasedGame, ErasedGameError};

/// Adapter that converts typed games to erased interface
/// 
/// This struct wraps any typed `Game` implementation and provides the `ErasedGame`
/// interface by handling all encoding/decoding operations and managing the
/// random number generator state.
/// 
/// The adapter maintains its own RNG instance that gets re-seeded on each reset,
/// ensuring deterministic behavior while providing the stateless interface
/// expected by the gRPC layer.
/// 
/// # Example
/// 
/// ```rust
/// # use engine_core::typed::*;
/// # use engine_core::adapter::GameAdapter;
/// # use engine_core::erased::ErasedGame;
/// # use rand_chacha::ChaCha20Rng;
/// 
/// # #[derive(Default)]
/// # struct MyGame;
/// # impl Game for MyGame {
/// #     type State = u32;
/// #     type Action = u8;
/// #     type Obs = Vec<f32>;
/// #     fn engine_id(&self) -> EngineId { todo!() }
/// #     fn capabilities(&self) -> Capabilities { todo!() }
/// #     fn reset(&mut self, rng: &mut ChaCha20Rng, hint: &[u8]) -> (Self::State, Self::Obs) { todo!() }
/// #     fn step(&mut self, state: &mut Self::State, action: Self::Action, rng: &mut ChaCha20Rng) -> (Self::Obs, f32, bool) { todo!() }
/// #     fn encode_state(state: &Self::State, out: &mut Vec<u8>) -> Result<(), EncodeError> { todo!() }
/// #     fn decode_state(buf: &[u8]) -> Result<Self::State, DecodeError> { todo!() }
/// #     fn encode_action(action: &Self::Action, out: &mut Vec<u8>) -> Result<(), EncodeError> { todo!() }
/// #     fn decode_action(buf: &[u8]) -> Result<Self::Action, DecodeError> { todo!() }
/// #     fn encode_obs(obs: &Self::Obs, out: &mut Vec<u8>) -> Result<(), EncodeError> { todo!() }
/// # }
/// 
/// let typed_game = MyGame::default();
/// let mut erased_game: Box<dyn ErasedGame> = Box::new(GameAdapter::new(typed_game));
/// 
/// // Now you can use the erased interface
/// let engine_id = erased_game.engine_id();
/// println!("Game: {}", engine_id.env_id);
/// ```
pub struct GameAdapter<T: Game> {
    game: T,
    rng: ChaCha20Rng,
}

impl<T: Game> GameAdapter<T> {
    /// Create a new adapter wrapping the given game
    /// 
    /// The adapter starts with a default-seeded RNG that will be re-seeded
    /// on the first reset call.
    pub fn new(game: T) -> Self {
        Self {
            game,
            rng: ChaCha20Rng::seed_from_u64(0), // Will be re-seeded on reset
        }
    }
    
    /// Get a reference to the underlying game
    pub fn game(&self) -> &T {
        &self.game
    }
    
    /// Get a mutable reference to the underlying game
    pub fn game_mut(&mut self) -> &mut T {
        &mut self.game
    }
    
    /// Consume the adapter and return the underlying game
    pub fn into_inner(self) -> T {
        self.game
    }
}

impl<T: Game> ErasedGame for GameAdapter<T> {
    fn engine_id(&self) -> EngineId {
        self.game.engine_id()
    }
    
    fn capabilities(&self) -> Capabilities {
        self.game.capabilities()
    }
    
    fn reset(
        &mut self, 
        seed: u64, 
        hint: &[u8], 
        out_state: &mut Vec<u8>, 
        out_obs: &mut Vec<u8>
    ) -> Result<(), ErasedGameError> {
        // Re-seed the RNG for deterministic behavior
        self.rng = ChaCha20Rng::seed_from_u64(seed);
        
        // Clear output buffers
        out_state.clear();
        out_obs.clear();
        
        // Call the typed reset method
        let (state, obs) = self.game.reset(&mut self.rng, hint);
        
        // Encode the results
        T::encode_state(&state, out_state)
            .map_err(|e| ErasedGameError::Encoding(e.to_string()))?;
            
        T::encode_obs(&obs, out_obs)
            .map_err(|e| ErasedGameError::Encoding(e.to_string()))?;
        
        Ok(())
    }
    
    fn step(
        &mut self, 
        state: &[u8], 
        action: &[u8], 
        out_state: &mut Vec<u8>, 
        out_obs: &mut Vec<u8>
    ) -> Result<(f32, bool), ErasedGameError> {
        // Clear output buffers
        out_state.clear();
        out_obs.clear();
        
        // Decode the inputs
        let mut state = T::decode_state(state)
            .map_err(|e| ErasedGameError::Decoding(e.to_string()))?;
            
        let action = T::decode_action(action)
            .map_err(|e| ErasedGameError::Decoding(e.to_string()))?;
        
        // Call the typed step method
        let (obs, reward, done) = self.game.step(&mut state, action, &mut self.rng);
        
        // Encode the results
        T::encode_state(&state, out_state)
            .map_err(|e| ErasedGameError::Encoding(e.to_string()))?;
            
        T::encode_obs(&obs, out_obs)
            .map_err(|e| ErasedGameError::Encoding(e.to_string()))?;
        
        Ok((reward, done))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::typed::{Encoding, ActionSpace, EncodeError, DecodeError};

    // Test game implementation
    #[derive(Debug, PartialEq)]
    struct TestGame {
        id: String,
        reset_count: u32,
        step_count: u32,
    }
    
    impl TestGame {
        fn new(id: String) -> Self {
            Self {
                id,
                reset_count: 0,
                step_count: 0,
            }
        }
    }
    
    impl Game for TestGame {
        type State = u32;
        type Action = u8;
        type Obs = Vec<f32>;
        
        fn engine_id(&self) -> EngineId {
            EngineId {
                env_id: self.id.clone(),
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
        
        fn reset(&mut self, rng: &mut ChaCha20Rng, _hint: &[u8]) -> (Self::State, Self::Obs) {
            self.reset_count += 1;
            self.step_count = 0;
            
            // Use RNG to ensure it's properly seeded
            use rand::Rng;
            let random_val = rng.gen::<u32>() % 100;
            
            (random_val, vec![random_val as f32])
        }
        
        fn step(&mut self, state: &mut Self::State, action: Self::Action, _rng: &mut ChaCha20Rng) -> (Self::Obs, f32, bool) {
            self.step_count += 1;
            *state += action as u32;
            
            let obs = vec![*state as f32, self.step_count as f32];
            let reward = action as f32;
            let done = *state >= 20 || self.step_count >= 10;
            
            (obs, reward, done)
        }
        
        fn encode_state(state: &Self::State, out: &mut Vec<u8>) -> Result<(), EncodeError> {
            out.extend_from_slice(&state.to_le_bytes());
            Ok(())
        }
        
        fn decode_state(buf: &[u8]) -> Result<Self::State, DecodeError> {
            if buf.len() != 4 {
                return Err(DecodeError::InvalidLength { expected: 4, actual: buf.len() });
            }
            Ok(u32::from_le_bytes(buf.try_into().unwrap()))
        }
        
        fn encode_action(action: &Self::Action, out: &mut Vec<u8>) -> Result<(), EncodeError> {
            out.push(*action);
            Ok(())
        }
        
        fn decode_action(buf: &[u8]) -> Result<Self::Action, DecodeError> {
            if buf.len() != 1 {
                return Err(DecodeError::InvalidLength { expected: 1, actual: buf.len() });
            }
            Ok(buf[0])
        }
        
        fn encode_obs(obs: &Self::Obs, out: &mut Vec<u8>) -> Result<(), EncodeError> {
            // Encode length first, then values
            let len = obs.len() as u32;
            out.extend_from_slice(&len.to_le_bytes());
            for &value in obs {
                out.extend_from_slice(&value.to_le_bytes());
            }
            Ok(())
        }
    }

    #[test]
    fn test_adapter_basic_functionality() {
        let game = TestGame::new("test".to_string());
        let adapter = GameAdapter::new(game);
        
        // Test engine_id passthrough
        let id = adapter.engine_id();
        assert_eq!(id.env_id, "test");
        
        // Test capabilities passthrough
        let caps = adapter.capabilities();
        assert_eq!(caps.id.env_id, "test");
        assert_eq!(caps.max_horizon, 100);
    }
    
    #[test]
    fn test_adapter_reset() {
        let game = TestGame::new("test".to_string());
        let mut adapter = GameAdapter::new(game);
        
        let mut state_buf = Vec::new();
        let mut obs_buf = Vec::new();
        
        adapter.reset(42, &[], &mut state_buf, &mut obs_buf).unwrap();
        
        // State should be encoded as 4 bytes (u32)
        assert_eq!(state_buf.len(), 4);
        let state_value = u32::from_le_bytes(state_buf.try_into().unwrap());
        
        // Obs should be encoded as length + values
        assert!(obs_buf.len() >= 4); // At least length header
        let obs_len = u32::from_le_bytes(obs_buf[0..4].try_into().unwrap());
        assert_eq!(obs_len, 1); // One f32 value
        assert_eq!(obs_buf.len(), 4 + 4); // Length + one f32
        
        // Verify the observation value matches the state
        let obs_value = f32::from_le_bytes(obs_buf[4..8].try_into().unwrap());
        assert_eq!(obs_value, state_value as f32);
    }
    
    #[test]
    fn test_adapter_step() {
        let game = TestGame::new("test".to_string());
        let mut adapter = GameAdapter::new(game);
        
        // Reset first
        let mut state_buf = Vec::new();
        let mut obs_buf = Vec::new();
        adapter.reset(42, &[], &mut state_buf, &mut obs_buf).unwrap();
        
        // Prepare action
        let action_bytes = vec![3u8];
        
        // Take a step
        let mut new_state_buf = Vec::new();
        let mut new_obs_buf = Vec::new();
        let (reward, _done) = adapter.step(&state_buf, &action_bytes, &mut new_state_buf, &mut new_obs_buf).unwrap();
        
        // Verify reward
        assert_eq!(reward, 3.0);
        
        // Decode new state
        let new_state = u32::from_le_bytes(new_state_buf.try_into().unwrap());
        let old_state = u32::from_le_bytes(state_buf.try_into().unwrap());
        assert_eq!(new_state, old_state + 3);
        
        // Verify obs structure
        assert!(new_obs_buf.len() >= 4);
        let obs_len = u32::from_le_bytes(new_obs_buf[0..4].try_into().unwrap());
        assert_eq!(obs_len, 2); // Two f32 values (state and step_count)
    }
    
    #[test]
    fn test_adapter_deterministic_reset() {
        let game1 = TestGame::new("test".to_string());
        let mut adapter1 = GameAdapter::new(game1);
        
        let game2 = TestGame::new("test".to_string());
        let mut adapter2 = GameAdapter::new(game2);
        
        // Reset with same seed
        let mut state1 = Vec::new();
        let mut obs1 = Vec::new();
        adapter1.reset(12345, &[], &mut state1, &mut obs1).unwrap();
        
        let mut state2 = Vec::new();
        let mut obs2 = Vec::new();
        adapter2.reset(12345, &[], &mut state2, &mut obs2).unwrap();
        
        // Results should be identical
        assert_eq!(state1, state2);
        assert_eq!(obs1, obs2);
    }
    
    #[test]
    fn test_adapter_different_seeds() {
        let game1 = TestGame::new("test".to_string());
        let mut adapter1 = GameAdapter::new(game1);
        
        let game2 = TestGame::new("test".to_string());
        let mut adapter2 = GameAdapter::new(game2);
        
        // Reset with different seeds
        let mut state1 = Vec::new();
        let mut obs1 = Vec::new();
        adapter1.reset(12345, &[], &mut state1, &mut obs1).unwrap();
        
        let mut state2 = Vec::new();
        let mut obs2 = Vec::new();
        adapter2.reset(54321, &[], &mut state2, &mut obs2).unwrap();
        
        // Results should be different (with very high probability)
        // Note: There's a tiny chance they could be the same due to randomness
        assert!(state1 != state2 || obs1 != obs2);
    }
    
    #[test]
    fn test_adapter_inner_access() {
        let game = TestGame::new("test".to_string());
        let mut adapter = GameAdapter::new(game);
        
        // Test mutable access
        adapter.game_mut().id = "modified".to_string();
        assert_eq!(adapter.game().id, "modified");
        
        // Test into_inner
        let inner_game = adapter.into_inner();
        assert_eq!(inner_game.id, "modified");
    }
    
    #[test]
    fn test_adapter_invalid_action_decoding() {
        let game = TestGame::new("test".to_string());
        let mut adapter = GameAdapter::new(game);
        
        // Reset first
        let mut state_buf = Vec::new();
        let mut obs_buf = Vec::new();
        adapter.reset(42, &[], &mut state_buf, &mut obs_buf).unwrap();
        
        // Try step with invalid action (wrong length)
        let invalid_action = vec![1, 2, 3]; // Should be 1 byte
        let mut new_state_buf = Vec::new();
        let mut new_obs_buf = Vec::new();
        
        let result = adapter.step(&state_buf, &invalid_action, &mut new_state_buf, &mut new_obs_buf);
        
        assert!(result.is_err());
        match result.unwrap_err() {
            ErasedGameError::Decoding(_) => {
                // Test passes - we got the expected error type
            }
            _ => panic!("Expected Decoding error"),
        }
    }
    
    #[test]
    fn test_adapter_invalid_state_decoding() {
        let game = TestGame::new("test".to_string());
        let mut adapter = GameAdapter::new(game);
        
        // Try step with invalid state (wrong length)
        let invalid_state = vec![1, 2, 3]; // Should be 4 bytes for u32
        let action = vec![1u8];
        let mut new_state_buf = Vec::new();
        let mut new_obs_buf = Vec::new();
        
        let result = adapter.step(&invalid_state, &action, &mut new_state_buf, &mut new_obs_buf);
        
        assert!(result.is_err());
        match result.unwrap_err() {
            ErasedGameError::Decoding(_) => {
                // Test passes - we got the expected error type
            }
            _ => panic!("Expected Decoding error"),
        }
    }
}