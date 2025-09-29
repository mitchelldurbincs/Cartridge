//! TicTacToe game implementation for the Cartridge engine
//! 
//! This crate provides a complete reference implementation of TicTacToe
//! demonstrating how to implement the Game trait for the engine framework.

use engine_core::typed::{Game, EngineId, Capabilities, Encoding, ActionSpace, EncodeError, DecodeError};
use rand_chacha::ChaCha20Rng;

/// TicTacToe game state
/// 
/// Represents the complete state of a TicTacToe game including the board,
/// current player, and winner information.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct State {
    /// Board representation: 0=empty, 1=X, 2=O
    board: [u8; 9],
    /// Current player: 1=X, 2=O
    current_player: u8,
    /// Winner: 0=none/ongoing, 1=X, 2=O, 3=draw
    winner: u8,
}

impl State {
    /// Create a new initial game state
    pub fn new() -> Self {
        Self {
            board: [0; 9],
            current_player: 1, // X goes first
            winner: 0,
        }
    }
    
    /// Check if the game is over
    pub fn is_done(&self) -> bool {
        self.winner != 0
    }
    
    /// Get legal moves (empty positions)
    pub fn legal_moves(&self) -> Vec<u8> {
        if self.is_done() {
            return Vec::new();
        }
        
        (0..9u8).filter(|&pos| self.board[pos as usize] == 0).collect()
    }
    
    /// Make a move and return the new state
    pub fn make_move(&self, position: u8) -> State {
        if self.is_done() || position >= 9 || self.board[position as usize] != 0 {
            return *self; // Invalid move, return unchanged state
        }
        
        let mut new_state = *self;
        new_state.board[position as usize] = self.current_player;
        
        // Check for winner
        new_state.winner = Self::check_winner(&new_state.board);
        
        // Switch player if game not over
        if new_state.winner == 0 {
            new_state.current_player = if self.current_player == 1 { 2 } else { 1 };
        }
        
        new_state
    }
    
    /// Check for winner on the board
    fn check_winner(board: &[u8; 9]) -> u8 {
        // Winning positions (rows, columns, diagonals)
        const LINES: [[usize; 3]; 8] = [
            [0, 1, 2], [3, 4, 5], [6, 7, 8], // rows
            [0, 3, 6], [1, 4, 7], [2, 5, 8], // columns
            [0, 4, 8], [2, 4, 6],           // diagonals
        ];
        
        for line in &LINES {
            let [a, b, c] = *line;
            if board[a] != 0 && board[a] == board[b] && board[b] == board[c] {
                return board[a]; // Return the winning player
            }
        }
        
        // Check for draw (board full but no winner)
        if board.iter().all(|&cell| cell != 0) {
            return 3; // Draw
        }
        
        0 // Game ongoing
    }
}

impl Default for State {
    fn default() -> Self {
        Self::new()
    }
}

/// TicTacToe action
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Action {
    /// Place a piece at the given position (0-8)
    Place(u8),
}

impl Action {
    /// Get the position for this action
    pub fn position(&self) -> u8 {
        match self {
            Action::Place(pos) => *pos,
        }
    }
}

/// TicTacToe observation
/// 
/// Provides a neural network-friendly representation of the game state
/// including board state and legal move mask.
#[derive(Debug, Clone, PartialEq)]
pub struct Observation {
    /// One-hot encoding of board: [X_positions, O_positions] (18 values)
    pub board_view: [f32; 18],
    /// Legal moves mask (9 values: 1.0 = legal, 0.0 = illegal)
    pub legal_moves: [f32; 9],
    /// Current player indicator: [is_X, is_O] (2 values)
    pub current_player: [f32; 2],
}

impl Observation {
    /// Create observation from game state
    pub fn from_state(state: &State) -> Self {
        let mut board_view = [0.0; 18];
        let mut legal_moves = [0.0; 9];
        let mut current_player = [0.0; 2];
        
        // Encode board state (one-hot for X and O)
        for (i, &cell) in state.board.iter().enumerate() {
            if cell == 1 {
                board_view[i] = 1.0; // X positions
            } else if cell == 2 {
                board_view[i + 9] = 1.0; // O positions
            }
        }
        
        // Encode legal moves
        if !state.is_done() {
            for pos in state.legal_moves() {
                legal_moves[pos as usize] = 1.0;
            }
        }
        
        // Encode current player
        if state.current_player == 1 {
            current_player[0] = 1.0; // X
        } else {
            current_player[1] = 1.0; // O
        }
        
        Self {
            board_view,
            legal_moves,
            current_player,
        }
    }
}

/// TicTacToe game implementation
#[derive(Debug)]
pub struct TicTacToe;

impl TicTacToe {
    /// Create a new TicTacToe game
    pub fn new() -> Self {
        Self
    }
    
    /// Calculate reward for the current state
    fn calculate_reward(state: &State, previous_player: u8) -> f32 {
        match state.winner {
            0 => 0.0,    // Game ongoing
            1 => if previous_player == 1 { 1.0 } else { -1.0 }, // X wins
            2 => if previous_player == 2 { 1.0 } else { -1.0 }, // O wins  
            3 => 0.0,    // Draw
            _ => 0.0,    // Shouldn't happen
        }
    }
}

impl Default for TicTacToe {
    fn default() -> Self {
        Self::new()
    }
}

impl Game for TicTacToe {
    type State = State;
    type Action = Action;
    type Obs = Observation;
    
    fn engine_id(&self) -> EngineId {
        EngineId {
            env_id: "tictactoe".to_string(),
            build_id: env!("CARGO_PKG_VERSION").to_string(),
        }
    }
    
    fn capabilities(&self) -> Capabilities {
        Capabilities {
            id: self.engine_id(),
            encoding: Encoding {
                state: "tictactoe_state:v1".to_string(),
                action: "discrete_position:v1".to_string(),
                obs: "f32x29:v1".to_string(), // 18 + 9 + 2 = 29 floats
                schema_version: 1,
            },
            max_horizon: 9, // Maximum 9 moves in TicTacToe
            action_space: ActionSpace::Discrete(9), // 9 possible positions
            preferred_batch: 64,
        }
    }
    
    fn reset(&mut self, _rng: &mut ChaCha20Rng, _hint: &[u8]) -> (Self::State, Self::Obs) {
        let state = State::new();
        let obs = Observation::from_state(&state);
        (state, obs)
    }
    
    fn step(&mut self, state: &mut Self::State, action: Self::Action, _rng: &mut ChaCha20Rng) -> (Self::Obs, f32, bool) {
        let previous_player = state.current_player;
        *state = state.make_move(action.position());
        
        let obs = Observation::from_state(state);
        let reward = Self::calculate_reward(state, previous_player);
        let done = state.is_done();
        
        (obs, reward, done)
    }
    
    fn encode_state(state: &Self::State, out: &mut Vec<u8>) -> Result<(), EncodeError> {
        // Simple binary encoding: board (9 bytes) + current_player (1 byte) + winner (1 byte)
        out.extend_from_slice(&state.board);
        out.push(state.current_player);
        out.push(state.winner);
        Ok(())
    }
    
    fn decode_state(buf: &[u8]) -> Result<Self::State, DecodeError> {
        if buf.len() != 11 {
            return Err(DecodeError::InvalidLength { 
                expected: 11, 
                actual: buf.len() 
            });
        }
        
        let mut board = [0u8; 9];
        board.copy_from_slice(&buf[0..9]);
        
        let current_player = buf[9];
        let winner = buf[10];
        
        // Validate the state
        if current_player != 1 && current_player != 2 {
            return Err(DecodeError::CorruptedData(
                format!("Invalid current_player: {}", current_player)
            ));
        }
        
        if winner > 3 {
            return Err(DecodeError::CorruptedData(
                format!("Invalid winner: {}", winner)
            ));
        }
        
        for &cell in &board {
            if cell > 2 {
                return Err(DecodeError::CorruptedData(
                    format!("Invalid board cell: {}", cell)
                ));
            }
        }
        
        Ok(State {
            board,
            current_player,
            winner,
        })
    }
    
    fn encode_action(action: &Self::Action, out: &mut Vec<u8>) -> Result<(), EncodeError> {
        let position = action.position();
        if position >= 9 {
            return Err(EncodeError::InvalidData(
                format!("Invalid action position: {}", position)
            ));
        }
        out.push(position);
        Ok(())
    }
    
    fn decode_action(buf: &[u8]) -> Result<Self::Action, DecodeError> {
        if buf.len() != 1 {
            return Err(DecodeError::InvalidLength { 
                expected: 1, 
                actual: buf.len() 
            });
        }
        
        let position = buf[0];
        if position >= 9 {
            return Err(DecodeError::CorruptedData(
                format!("Invalid action position: {}", position)
            ));
        }
        
        Ok(Action::Place(position))
    }
    
    fn encode_obs(obs: &Self::Obs, out: &mut Vec<u8>) -> Result<(), EncodeError> {
        // Encode as 29 f32 values in little-endian format
        for &value in &obs.board_view {
            out.extend_from_slice(&value.to_le_bytes());
        }
        for &value in &obs.legal_moves {
            out.extend_from_slice(&value.to_le_bytes());
        }
        for &value in &obs.current_player {
            out.extend_from_slice(&value.to_le_bytes());
        }
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use rand::SeedableRng;
    
    #[test]
    fn test_initial_state() {
        let state = State::new();
        assert_eq!(state.board, [0; 9]);
        assert_eq!(state.current_player, 1);
        assert_eq!(state.winner, 0);
        assert!(!state.is_done());
    }
    
    #[test]
    fn test_legal_moves() {
        let state = State::new();
        let legal = state.legal_moves();
        assert_eq!(legal, (0..9).collect::<Vec<_>>());
        
        // After one move
        let state = state.make_move(4); // Center
        let legal = state.legal_moves();
        assert_eq!(legal.len(), 8);
        assert!(!legal.contains(&4));
    }
    
    #[test]
    fn test_make_move() {
        let state = State::new();
        let new_state = state.make_move(4); // X places in center
        
        assert_eq!(new_state.board[4], 1);
        assert_eq!(new_state.current_player, 2); // Now O's turn
        assert!(!new_state.is_done());
    }
    
    #[test]
    fn test_invalid_move() {
        let state = State::new();
        let state_with_move = state.make_move(4);
        
        // Try to place in same position
        let invalid_state = state_with_move.make_move(4);
        assert_eq!(invalid_state, state_with_move); // Should be unchanged
    }
    
    #[test]
    fn test_winning_game() {
        let mut state = State::new();
        
        // X wins with top row
        state = state.make_move(0); // X
        state = state.make_move(3); // O
        state = state.make_move(1); // X
        state = state.make_move(4); // O
        state = state.make_move(2); // X wins
        
        assert_eq!(state.winner, 1);
        assert!(state.is_done());
        assert!(state.legal_moves().is_empty());
    }
    
    #[test]
    fn test_draw_game() {
        // Create a draw state manually since getting the exact move sequence is tricky
        // Board: X O X / O X O / O X O
        let state = State {
            board: [1, 2, 1, 2, 1, 2, 2, 1, 2], // X=1, O=2
            current_player: 1, // Doesn't matter since game is over
            winner: 3, // This should be detected as a draw
        };

        // Verify this is actually a draw by checking the game logic
        let detected_winner = State::check_winner(&state.board);
        assert_eq!(detected_winner, 3); // Should be draw
        assert!(state.is_done());
    }
    
    #[test]
    fn test_observation_encoding() {
        let state = State::new();
        let obs = Observation::from_state(&state);
        
        // All board positions should be 0 initially
        assert_eq!(obs.board_view, [0.0; 18]);
        // All moves should be legal
        assert_eq!(obs.legal_moves, [1.0; 9]);
        // X should be current player
        assert_eq!(obs.current_player, [1.0, 0.0]);
    }
    
    #[test]
    fn test_game_trait_implementation() {
        let mut game = TicTacToe::new();
        let mut rng = ChaCha20Rng::seed_from_u64(42);
        
        let (state, _obs) = game.reset(&mut rng, &[]);
        assert_eq!(state, State::new());
        
        let action = Action::Place(4);
        let (_new_obs, reward, done) = game.step(&mut state.clone(), action, &mut rng);
        
        // Should not be done after one move
        assert!(!done);
        // Reward should be 0 for ongoing game
        assert_eq!(reward, 0.0);
    }
    
    #[test]
    fn test_state_encoding_roundtrip() {
        let original_state = State {
            board: [1, 0, 2, 0, 1, 0, 2, 0, 0],
            current_player: 2,
            winner: 0,
        };
        
        let mut buf = Vec::new();
        TicTacToe::encode_state(&original_state, &mut buf).unwrap();
        let decoded_state = TicTacToe::decode_state(&buf).unwrap();
        
        assert_eq!(original_state, decoded_state);
    }
    
    #[test]
    fn test_action_encoding_roundtrip() {
        let action = Action::Place(5);
        
        let mut buf = Vec::new();
        TicTacToe::encode_action(&action, &mut buf).unwrap();
        let decoded_action = TicTacToe::decode_action(&buf).unwrap();
        
        assert_eq!(action, decoded_action);
    }
    
    #[test]
    fn test_observation_byte_encoding() {
        let state = State {
            board: [1, 0, 2, 0, 0, 0, 0, 0, 0],
            current_player: 2,
            winner: 0,
        };
        let obs = Observation::from_state(&state);

        let mut buf = Vec::new();
        TicTacToe::encode_obs(&obs, &mut buf).unwrap();

        // Should be 29 * 4 = 116 bytes (29 f32 values)
        assert_eq!(buf.len(), 116);
    }
    
    #[test]
    fn test_engine_capabilities() {
        let game = TicTacToe::new();
        let caps = game.capabilities();
        
        assert_eq!(caps.id.env_id, "tictactoe");
        assert_eq!(caps.max_horizon, 9);
        
        match caps.action_space {
            ActionSpace::Discrete(n) => assert_eq!(n, 9),
            _ => panic!("Expected discrete action space"),
        }
    }
    
    #[test]
    fn test_invalid_state_decoding() {
        // Test wrong length
        let buf = vec![1, 2, 3]; // Too short
        let result = TicTacToe::decode_state(&buf);
        assert!(result.is_err());
        
        // Test invalid current_player
        let mut buf = vec![0; 11];
        buf[9] = 5; // Invalid player
        let result = TicTacToe::decode_state(&buf);
        assert!(result.is_err());
        
        // Test invalid winner
        let mut buf = vec![0; 11];
        buf[9] = 1; // Valid player
        buf[10] = 5; // Invalid winner
        let result = TicTacToe::decode_state(&buf);
        assert!(result.is_err());
    }
    
    #[test]
    fn test_invalid_action_decoding() {
        // Test wrong length
        let buf = vec![1, 2]; // Too long
        let result = TicTacToe::decode_action(&buf);
        assert!(result.is_err());
        
        // Test invalid position
        let buf = vec![9]; // Position out of bounds
        let result = TicTacToe::decode_action(&buf);
        assert!(result.is_err());
    }
}