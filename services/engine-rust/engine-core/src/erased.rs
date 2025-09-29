//! Erased Game interface for runtime polymorphism
//! 
//! This trait provides a bytes-only interface that works across gRPC boundaries
//! without generics. All typed games are converted to this interface via the
//! adapter layer.

use crate::typed::{EngineId, Capabilities};

/// Runtime error for erased game operations
#[derive(Debug, thiserror::Error)]
pub enum ErasedGameError {
    #[error("Encoding error: {0}")]
    Encoding(String),
    #[error("Decoding error: {0}")]
    Decoding(String),
    #[error("Invalid state: {0}")]
    InvalidState(String),
    #[error("Invalid action: {0}")]
    InvalidAction(String),
    #[error("Game logic error: {0}")]
    GameLogic(String),
}

/// Erased game trait that works only with bytes
/// 
/// This trait provides a runtime interface for games without generics,
/// making it suitable for use across gRPC boundaries and in dynamic
/// dispatch scenarios.
/// 
/// All data is passed as byte slices and results are written to provided
/// output buffers to enable allocation-free hot paths.
/// 
/// # Example Usage
/// 
/// ```rust
/// # use engine_core::erased::*;
/// # use engine_core::typed::*;
/// 
/// fn simulate_game(game: &mut dyn ErasedGame) -> Result<(), ErasedGameError> {
///     let caps = game.capabilities();
///     println!("Simulating {}", caps.id.env_id);
///     
///     let mut state_buf = Vec::new();
///     let mut obs_buf = Vec::new();
///     
///     // Reset the game
///     game.reset(42, &[], &mut state_buf, &mut obs_buf)?;
///     
///     // Take a step (would need valid action bytes)
///     let action_bytes = vec![0]; // Placeholder
///     let (reward, done) = game.step(&state_buf, &action_bytes, &mut state_buf, &mut obs_buf)?;
///     
///     println!("Reward: {}, Done: {}", reward, done);
///     Ok(())
/// }
/// ```
pub trait ErasedGame: Send + Sync + 'static {
    /// Get engine identification information
    fn engine_id(&self) -> EngineId;
    
    /// Get game capabilities and configuration
    fn capabilities(&self) -> Capabilities;
    
    /// Reset the game to initial state
    /// 
    /// # Arguments
    /// 
    /// * `seed` - Random seed for deterministic reset
    /// * `hint` - Optional hint data for environment setup
    /// * `out_state` - Buffer to write encoded initial state
    /// * `out_obs` - Buffer to write encoded initial observation
    /// 
    /// # Errors
    /// 
    /// Returns `ErasedGameError` if reset fails or encoding fails
    fn reset(
        &mut self, 
        seed: u64, 
        hint: &[u8], 
        out_state: &mut Vec<u8>, 
        out_obs: &mut Vec<u8>
    ) -> Result<(), ErasedGameError>;
    
    /// Perform one simulation step
    /// 
    /// # Arguments
    /// 
    /// * `state` - Current state encoded as bytes
    /// * `action` - Action to take encoded as bytes
    /// * `out_state` - Buffer to write encoded new state
    /// * `out_obs` - Buffer to write encoded new observation
    /// 
    /// # Returns
    /// 
    /// Returns `Ok((reward, done))` on success, where:
    /// - `reward` - Reward received from this step
    /// - `done` - Whether the episode has terminated
    /// 
    /// # Errors
    /// 
    /// Returns `ErasedGameError` if step fails or encoding/decoding fails
    fn step(
        &mut self, 
        state: &[u8], 
        action: &[u8], 
        out_state: &mut Vec<u8>, 
        out_obs: &mut Vec<u8>
    ) -> Result<(f32, bool), ErasedGameError>;
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::typed::{Encoding, ActionSpace};

    // Mock implementation for testing
    struct MockErasedGame {
        step_count: u32,
    }
    
    impl MockErasedGame {
        fn new() -> Self {
            Self { step_count: 0 }
        }
    }
    
    impl ErasedGame for MockErasedGame {
        fn engine_id(&self) -> EngineId {
            EngineId {
                env_id: "mock".to_string(),
                build_id: "0.1.0".to_string(),
            }
        }
        
        fn capabilities(&self) -> Capabilities {
            Capabilities {
                id: self.engine_id(),
                encoding: Encoding {
                    state: "u32:v1".to_string(),
                    action: "u8:v1".to_string(),
                    obs: "f32:v1".to_string(),
                    schema_version: 1,
                },
                max_horizon: 10,
                action_space: ActionSpace::Discrete(2),
                preferred_batch: 16,
            }
        }
        
        fn reset(
            &mut self, 
            _seed: u64, 
            _hint: &[u8], 
            out_state: &mut Vec<u8>, 
            out_obs: &mut Vec<u8>
        ) -> Result<(), ErasedGameError> {
            self.step_count = 0;
            
            // Encode state as u32 (step count)
            out_state.extend_from_slice(&self.step_count.to_le_bytes());
            
            // Encode observation as f32
            out_obs.extend_from_slice(&(self.step_count as f32).to_le_bytes());
            
            Ok(())
        }
        
        fn step(
            &mut self, 
            state: &[u8], 
            _action: &[u8], 
            out_state: &mut Vec<u8>, 
            out_obs: &mut Vec<u8>
        ) -> Result<(f32, bool), ErasedGameError> {
            // Decode current state
            if state.len() != 4 {
                return Err(ErasedGameError::InvalidState(
                    format!("Expected 4 bytes, got {}", state.len())
                ));
            }
            
            let current_step = u32::from_le_bytes(state.try_into().unwrap());
            let new_step = current_step + 1;
            
            // Encode new state
            out_state.extend_from_slice(&new_step.to_le_bytes());
            
            // Encode new observation
            out_obs.extend_from_slice(&(new_step as f32).to_le_bytes());
            
            let reward = 1.0;
            let done = new_step >= 5;
            
            Ok((reward, done))
        }
    }

    #[test]
    fn test_erased_game_reset() {
        let mut game = MockErasedGame::new();
        let mut state_buf = Vec::new();
        let mut obs_buf = Vec::new();
        
        game.reset(42, &[], &mut state_buf, &mut obs_buf).unwrap();
        
        assert_eq!(state_buf.len(), 4);
        assert_eq!(obs_buf.len(), 4);
        
        let state = u32::from_le_bytes(state_buf.try_into().unwrap());
        assert_eq!(state, 0);
    }
    
    #[test]
    fn test_erased_game_step() {
        let mut game = MockErasedGame::new();
        let mut state_buf = Vec::new();
        let mut obs_buf = Vec::new();
        
        // Reset first
        game.reset(42, &[], &mut state_buf, &mut obs_buf).unwrap();
        
        // Take a step
        let action_bytes = vec![0]; // Mock action
        let mut new_state_buf = Vec::new();
        let mut new_obs_buf = Vec::new();
        
        let (reward, done) = game.step(&state_buf, &action_bytes, &mut new_state_buf, &mut new_obs_buf).unwrap();
        
        assert_eq!(reward, 1.0);
        assert!(!done);
        assert_eq!(new_state_buf.len(), 4);
        assert_eq!(new_obs_buf.len(), 4);
        
        let new_state = u32::from_le_bytes(new_state_buf.try_into().unwrap());
        assert_eq!(new_state, 1);
    }
    
    #[test]
    fn test_erased_game_capabilities() {
        let game = MockErasedGame::new();
        let caps = game.capabilities();
        
        assert_eq!(caps.id.env_id, "mock");
        assert_eq!(caps.max_horizon, 10);
        assert_eq!(caps.encoding.state, "u32:v1");
        
        match caps.action_space {
            ActionSpace::Discrete(n) => assert_eq!(n, 2),
            _ => panic!("Expected discrete action space"),
        }
    }
    
    #[test]
    fn test_invalid_state_error() {
        let mut game = MockErasedGame::new();
        let invalid_state = vec![1, 2, 3]; // Wrong length
        let action_bytes = vec![0];
        let mut state_buf = Vec::new();
        let mut obs_buf = Vec::new();
        
        let result = game.step(&invalid_state, &action_bytes, &mut state_buf, &mut obs_buf);
        
        assert!(result.is_err());
        match result.unwrap_err() {
            ErasedGameError::InvalidState(msg) => {
                assert!(msg.contains("Expected 4 bytes, got 3"));
            }
            _ => panic!("Expected InvalidState error"),
        }
    }
}