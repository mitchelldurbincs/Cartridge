//! Typed Game trait providing ergonomic interface for game developers
//! 
//! This trait allows game implementations to work with strongly-typed state,
//! action, and observation types while maintaining compile-time type safety.

use rand_chacha::ChaCha20Rng;

/// Engine identification information
#[derive(Debug, Clone, PartialEq)]
pub struct EngineId {
    pub env_id: String,
    pub build_id: String,
}

/// Encoding format specifications
#[derive(Debug, Clone, PartialEq)]
pub struct Encoding {
    pub state: String,
    pub action: String,
    pub obs: String,
    pub schema_version: u32,
}

/// Action space variants
#[derive(Debug, Clone, PartialEq)]
pub enum ActionSpace {
    Discrete(u32),
    MultiDiscrete(Vec<u32>),
    Continuous { low: Vec<f32>, high: Vec<f32>, shape: Vec<u32> },
}

/// Game capabilities and configuration
#[derive(Debug, Clone, PartialEq)]
pub struct Capabilities {
    pub id: EngineId,
    pub encoding: Encoding,
    pub max_horizon: u32,
    pub action_space: ActionSpace,
    pub preferred_batch: u32,
}

/// Main trait for game implementations
/// 
/// Games should implement this trait with their specific types for State, Action, and Obs.
/// The trait provides compile-time type safety while allowing conversion to the erased
/// interface for runtime polymorphism.
/// 
/// # Type Parameters
/// 
/// * `State` - Game state type, should be POD-like for efficient copying
/// * `Action` - Action type, should be small and Copy or compact
/// * `Obs` - Observation type, often contiguous arrays of f32
/// 
/// # Example
/// 
/// ```rust
/// # use engine_core::typed::*;
/// # use rand_chacha::ChaCha20Rng;
/// 
/// #[derive(Clone, Copy)]
/// struct TicTacToeState {
///     board: [u8; 9],
///     current_player: u8,
/// }
/// 
/// #[derive(Clone, Copy)]
/// enum TicTacToeAction {
///     Place(u8),
/// }
/// 
/// #[derive(Clone)]
/// struct TicTacToeObs {
///     board_view: [f32; 18],
///     legal_moves: [f32; 9],
/// }
/// 
/// struct TicTacToe;
/// 
/// impl Game for TicTacToe {
///     type State = TicTacToeState;
///     type Action = TicTacToeAction;
///     type Obs = TicTacToeObs;
///     
///     // Implementation methods...
/// #   fn engine_id(&self) -> EngineId { todo!() }
/// #   fn capabilities(&self) -> Capabilities { todo!() }
/// #   fn reset(&mut self, rng: &mut ChaCha20Rng, hint: &[u8]) -> (Self::State, Self::Obs) { todo!() }
/// #   fn step(&mut self, state: &mut Self::State, action: Self::Action, rng: &mut ChaCha20Rng) -> (Self::Obs, f32, bool) { todo!() }
/// #   fn encode_state(state: &Self::State, out: &mut Vec<u8>) -> Result<(), EncodeError> { todo!() }
/// #   fn decode_state(buf: &[u8]) -> Result<Self::State, DecodeError> { todo!() }
/// #   fn encode_action(action: &Self::Action, out: &mut Vec<u8>) -> Result<(), EncodeError> { todo!() }
/// #   fn decode_action(buf: &[u8]) -> Result<Self::Action, DecodeError> { todo!() }
/// #   fn encode_obs(obs: &Self::Obs, out: &mut Vec<u8>) -> Result<(), EncodeError> { todo!() }
/// }
/// ```
pub trait Game: Send + Sync + 'static {
    /// Game state type - should be efficiently copyable
    type State: Send + Sync + 'static;
    
    /// Action type - should be small and Copy or compact
    type Action: Send + Sync + 'static;
    
    /// Observation type - often contiguous arrays of f32
    type Obs: Send + Sync + 'static;

    /// Get engine identification information
    fn engine_id(&self) -> EngineId;
    
    /// Get game capabilities and configuration
    fn capabilities(&self) -> Capabilities;
    
    /// Reset the game to initial state
    /// 
    /// # Arguments
    /// 
    /// * `rng` - Deterministic random number generator for reproducible resets
    /// * `hint` - Optional hint data for environment setup
    /// 
    /// # Returns
    /// 
    /// A tuple of (initial_state, initial_observation)
    fn reset(&mut self, rng: &mut ChaCha20Rng, hint: &[u8]) -> (Self::State, Self::Obs);
    
    /// Perform one simulation step
    /// 
    /// # Arguments
    /// 
    /// * `state` - Current game state (mutable for in-place updates)
    /// * `action` - Action to take
    /// * `rng` - Random number generator for stochastic elements
    /// 
    /// # Returns
    /// 
    /// A tuple of (observation, reward, done)
    fn step(&mut self, state: &mut Self::State, action: Self::Action, rng: &mut ChaCha20Rng) -> (Self::Obs, f32, bool);

    // Encoding/Decoding hooks for serialization
    
    /// Encode state to bytes
    fn encode_state(state: &Self::State, out: &mut Vec<u8>) -> Result<(), EncodeError>;
    
    /// Decode state from bytes
    fn decode_state(buf: &[u8]) -> Result<Self::State, DecodeError>;
    
    /// Encode action to bytes
    fn encode_action(action: &Self::Action, out: &mut Vec<u8>) -> Result<(), EncodeError>;
    
    /// Decode action from bytes
    fn decode_action(buf: &[u8]) -> Result<Self::Action, DecodeError>;
    
    /// Encode observation to bytes
    fn encode_obs(obs: &Self::Obs, out: &mut Vec<u8>) -> Result<(), EncodeError>;
}

/// Error type for encoding operations
#[derive(Debug, thiserror::Error)]
pub enum EncodeError {
    #[error("Failed to encode data: {0}")]
    SerializationError(String),
    #[error("Buffer too small, needed {needed} bytes but got {available}")]
    BufferTooSmall { needed: usize, available: usize },
    #[error("Invalid data: {0}")]
    InvalidData(String),
}

/// Error type for decoding operations  
#[derive(Debug, thiserror::Error)]
pub enum DecodeError {
    #[error("Failed to decode data: {0}")]
    DeserializationError(String),
    #[error("Invalid buffer length: expected {expected} but got {actual}")]
    InvalidLength { expected: usize, actual: usize },
    #[error("Corrupted data: {0}")]
    CorruptedData(String),
    #[error("Unsupported version: {version}")]
    UnsupportedVersion { version: u32 },
}

#[cfg(test)]
mod tests {
    use super::*;
    use rand::SeedableRng;

    // Helper types for testing
    #[derive(Clone, Copy, Debug, PartialEq)]
    struct TestState(u32);
    
    #[derive(Clone, Copy, Debug, PartialEq)]
    struct TestAction(u8);
    
    #[derive(Clone, Debug, PartialEq)]
    struct TestObs(Vec<f32>);
    
    struct TestGame;
    
    impl Game for TestGame {
        type State = TestState;
        type Action = TestAction;
        type Obs = TestObs;
        
        fn engine_id(&self) -> EngineId {
            EngineId {
                env_id: "test".to_string(),
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
            (TestState(0), TestObs(vec![0.0, 1.0]))
        }
        
        fn step(&mut self, state: &mut Self::State, action: Self::Action, _rng: &mut ChaCha20Rng) -> (Self::Obs, f32, bool) {
            state.0 += action.0 as u32;
            (TestObs(vec![state.0 as f32]), 1.0, state.0 >= 10)
        }
        
        fn encode_state(state: &Self::State, out: &mut Vec<u8>) -> Result<(), EncodeError> {
            out.extend_from_slice(&state.0.to_le_bytes());
            Ok(())
        }
        
        fn decode_state(buf: &[u8]) -> Result<Self::State, DecodeError> {
            if buf.len() != 4 {
                return Err(DecodeError::InvalidLength { expected: 4, actual: buf.len() });
            }
            let value = u32::from_le_bytes(buf.try_into().unwrap());
            Ok(TestState(value))
        }
        
        fn encode_action(action: &Self::Action, out: &mut Vec<u8>) -> Result<(), EncodeError> {
            out.push(action.0);
            Ok(())
        }
        
        fn decode_action(buf: &[u8]) -> Result<Self::Action, DecodeError> {
            if buf.len() != 1 {
                return Err(DecodeError::InvalidLength { expected: 1, actual: buf.len() });
            }
            Ok(TestAction(buf[0]))
        }
        
        fn encode_obs(obs: &Self::Obs, out: &mut Vec<u8>) -> Result<(), EncodeError> {
            for &value in &obs.0 {
                out.extend_from_slice(&value.to_le_bytes());
            }
            Ok(())
        }
    }

    #[test]
    fn test_game_basic_functionality() {
        let mut game = TestGame;
        let mut rng = ChaCha20Rng::seed_from_u64(42);
        
        let (state, obs) = game.reset(&mut rng, &[]);
        assert_eq!(state, TestState(0));
        assert_eq!(obs, TestObs(vec![0.0, 1.0]));
        
        let caps = game.capabilities();
        assert_eq!(caps.id.env_id, "test");
        assert_eq!(caps.max_horizon, 100);
    }
    
    #[test]
    fn test_state_encoding_roundtrip() {
        let state = TestState(42);
        let mut buf = Vec::new();
        
        TestGame::encode_state(&state, &mut buf).unwrap();
        let decoded = TestGame::decode_state(&buf).unwrap();
        
        assert_eq!(state, decoded);
    }
    
    #[test]
    fn test_action_encoding_roundtrip() {
        let action = TestAction(3);
        let mut buf = Vec::new();
        
        TestGame::encode_action(&action, &mut buf).unwrap();
        let decoded = TestGame::decode_action(&buf).unwrap();
        
        assert_eq!(action, decoded);
    }
}