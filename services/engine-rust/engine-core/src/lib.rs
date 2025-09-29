//! Core traits and types for the Cartridge game engine
//! 
//! This crate provides the fundamental abstractions used by the engine server:
//! - `Game`: Typed trait for ergonomic game development  
//! - `ErasedGame`: Runtime interface that works only with bytes
//! - `GameAdapter`: Automatic conversion from typed to erased interface
//! - `Registry`: Static registration system for games

pub mod typed;
pub mod erased;
pub mod adapter;
pub mod registry;

// Re-export main types for convenience
pub use typed::Game;
pub use erased::ErasedGame;
pub use adapter::GameAdapter;
pub use registry::{register_game, create_game, GameFactory};