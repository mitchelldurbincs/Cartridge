//! Buffer pool management for allocation-free hot paths
//! 
//! This module provides a thread-safe buffer pool that enables allocation-free operation
//! in the hot paths of the gRPC service by reusing byte vectors.

use std::sync::{Arc, Mutex};

/// Thread-safe buffer pool for reusing byte vectors
/// 
/// The buffer pool maintains separate pools for different types of buffers
/// to optimize allocation patterns and reduce fragmentation.
#[derive(Debug, Clone)]
pub struct BufferPool {
    state_buffers: Arc<Mutex<Vec<Vec<u8>>>>,
    obs_buffers: Arc<Mutex<Vec<Vec<u8>>>>,
    action_buffers: Arc<Mutex<Vec<Vec<u8>>>>,
}

impl BufferPool {
    /// Create a new buffer pool
    pub fn new() -> Self {
        Self {
            state_buffers: Arc::new(Mutex::new(Vec::new())),
            obs_buffers: Arc::new(Mutex::new(Vec::new())),
            action_buffers: Arc::new(Mutex::new(Vec::new())),
        }
    }
    
    /// Create a new buffer pool with pre-allocated buffers
    /// 
    /// This method pre-allocates buffers to reduce allocation overhead during startup.
    /// 
    /// # Arguments
    /// 
    /// * `state_count` - Number of state buffers to pre-allocate
    /// * `obs_count` - Number of observation buffers to pre-allocate  
    /// * `action_count` - Number of action buffers to pre-allocate
    /// * `initial_capacity` - Initial capacity for each buffer
    pub fn with_capacity(
        state_count: usize, 
        obs_count: usize, 
        action_count: usize, 
        initial_capacity: usize
    ) -> Self {
        let mut state_buffers = Vec::with_capacity(state_count);
        let mut obs_buffers = Vec::with_capacity(obs_count);
        let mut action_buffers = Vec::with_capacity(action_count);
        
        for _ in 0..state_count {
            state_buffers.push(Vec::with_capacity(initial_capacity));
        }
        
        for _ in 0..obs_count {
            obs_buffers.push(Vec::with_capacity(initial_capacity));
        }
        
        for _ in 0..action_count {
            action_buffers.push(Vec::with_capacity(initial_capacity));
        }
        
        Self {
            state_buffers: Arc::new(Mutex::new(state_buffers)),
            obs_buffers: Arc::new(Mutex::new(obs_buffers)),
            action_buffers: Arc::new(Mutex::new(action_buffers)),
        }
    }
    
    /// Get a state buffer from the pool
    /// 
    /// If no buffer is available in the pool, returns a new empty vector.
    pub fn get_state_buffer(&self) -> Vec<u8> {
        self.state_buffers
            .lock()
            .unwrap()
            .pop()
            .unwrap_or_else(Vec::new)
    }
    
    /// Return a state buffer to the pool
    /// 
    /// The buffer is cleared before being returned to the pool.
    pub fn return_state_buffer(&self, mut buf: Vec<u8>) {
        buf.clear();
        self.state_buffers.lock().unwrap().push(buf);
    }
    
    /// Get an observation buffer from the pool
    pub fn get_obs_buffer(&self) -> Vec<u8> {
        self.obs_buffers
            .lock()
            .unwrap()
            .pop()
            .unwrap_or_else(Vec::new)
    }
    
    /// Return an observation buffer to the pool
    pub fn return_obs_buffer(&self, mut buf: Vec<u8>) {
        buf.clear();
        self.obs_buffers.lock().unwrap().push(buf);
    }
    
    /// Get an action buffer from the pool
    pub fn get_action_buffer(&self) -> Vec<u8> {
        self.action_buffers
            .lock()
            .unwrap()
            .pop()
            .unwrap_or_else(Vec::new)
    }
    
    /// Return an action buffer to the pool
    pub fn return_action_buffer(&self, mut buf: Vec<u8>) {
        buf.clear();
        self.action_buffers.lock().unwrap().push(buf);
    }
    
    /// Get statistics about the buffer pool
    pub fn stats(&self) -> BufferPoolStats {
        let state_count = self.state_buffers.lock().unwrap().len();
        let obs_count = self.obs_buffers.lock().unwrap().len();
        let action_count = self.action_buffers.lock().unwrap().len();
        
        BufferPoolStats {
            available_state_buffers: state_count,
            available_obs_buffers: obs_count,
            available_action_buffers: action_count,
        }
    }
    
    /// Clear all buffers from the pool
    /// 
    /// This is primarily useful for testing or memory pressure situations.
    pub fn clear(&self) {
        self.state_buffers.lock().unwrap().clear();
        self.obs_buffers.lock().unwrap().clear();
        self.action_buffers.lock().unwrap().clear();
    }
}

impl Default for BufferPool {
    fn default() -> Self {
        Self::new()
    }
}

/// Statistics about buffer pool usage
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct BufferPoolStats {
    pub available_state_buffers: usize,
    pub available_obs_buffers: usize,
    pub available_action_buffers: usize,
}

/// RAII wrapper for automatic buffer return
/// 
/// This wrapper ensures buffers are automatically returned to the pool
/// when they go out of scope, preventing buffer leaks.
pub struct PooledBuffer {
    buffer: Option<Vec<u8>>,
    return_fn: Option<Box<dyn FnOnce(Vec<u8>) + Send>>,
}

impl PooledBuffer {
    /// Create a new pooled buffer
    pub fn new<F>(buffer: Vec<u8>, return_fn: F) -> Self 
    where 
        F: FnOnce(Vec<u8>) + Send + 'static 
    {
        Self {
            buffer: Some(buffer),
            return_fn: Some(Box::new(return_fn)),
        }
    }
    
    /// Get a mutable reference to the buffer
    pub fn as_mut(&mut self) -> &mut Vec<u8> {
        self.buffer.as_mut().expect("Buffer already consumed")
    }
    
    /// Get an immutable reference to the buffer
    pub fn as_ref(&self) -> &Vec<u8> {
        self.buffer.as_ref().expect("Buffer already consumed")
    }
    
    /// Consume the wrapper and return the buffer without returning it to the pool
    pub fn into_inner(mut self) -> Vec<u8> {
        self.buffer.take().expect("Buffer already consumed")
    }
}

impl Drop for PooledBuffer {
    fn drop(&mut self) {
        if let (Some(buffer), Some(return_fn)) = (self.buffer.take(), self.return_fn.take()) {
            return_fn(buffer);
        }
    }
}

impl std::ops::Deref for PooledBuffer {
    type Target = Vec<u8>;
    
    fn deref(&self) -> &Self::Target {
        self.buffer.as_ref().expect("Buffer already consumed")
    }
}

impl std::ops::DerefMut for PooledBuffer {
    fn deref_mut(&mut self) -> &mut Self::Target {
        self.buffer.as_mut().expect("Buffer already consumed")
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    
    #[test]
    fn test_buffer_pool_basic_usage() {
        let pool = BufferPool::new();
        
        // Get and return a state buffer
        let mut buf = pool.get_state_buffer();
        buf.extend_from_slice(b"test data");
        assert_eq!(buf.len(), 9);
        
        pool.return_state_buffer(buf);
        
        // Get the buffer again - should be empty
        let buf2 = pool.get_state_buffer();
        assert_eq!(buf2.len(), 0);
        assert!(buf2.capacity() >= 9); // Should retain capacity
    }
    
    #[test]
    fn test_buffer_pool_with_capacity() {
        let pool = BufferPool::with_capacity(5, 3, 2, 128);
        let stats = pool.stats();
        
        assert_eq!(stats.available_state_buffers, 5);
        assert_eq!(stats.available_obs_buffers, 3);
        assert_eq!(stats.available_action_buffers, 2);
        
        // Test that buffers have the expected capacity
        let buf = pool.get_state_buffer();
        assert!(buf.capacity() >= 128);
    }
    
    #[test]
    fn test_multiple_buffer_types() {
        let pool = BufferPool::new();
        
        let state_buf = pool.get_state_buffer();
        let obs_buf = pool.get_obs_buffer();
        let action_buf = pool.get_action_buffer();
        
        pool.return_state_buffer(state_buf);
        pool.return_obs_buffer(obs_buf);
        pool.return_action_buffer(action_buf);
        
        let stats = pool.stats();
        assert_eq!(stats.available_state_buffers, 1);
        assert_eq!(stats.available_obs_buffers, 1);
        assert_eq!(stats.available_action_buffers, 1);
    }
    
    #[test]
    fn test_buffer_pool_stats() {
        let pool = BufferPool::new();
        let initial_stats = pool.stats();
        
        assert_eq!(initial_stats.available_state_buffers, 0);
        assert_eq!(initial_stats.available_obs_buffers, 0);
        assert_eq!(initial_stats.available_action_buffers, 0);
        
        // Return some buffers
        pool.return_state_buffer(Vec::new());
        pool.return_state_buffer(Vec::new());
        pool.return_obs_buffer(Vec::new());
        
        let stats = pool.stats();
        assert_eq!(stats.available_state_buffers, 2);
        assert_eq!(stats.available_obs_buffers, 1);
        assert_eq!(stats.available_action_buffers, 0);
    }
    
    #[test]
    fn test_buffer_pool_clear() {
        let pool = BufferPool::new();
        
        // Add some buffers
        pool.return_state_buffer(Vec::new());
        pool.return_obs_buffer(Vec::new());
        pool.return_action_buffer(Vec::new());
        
        let stats_before = pool.stats();
        assert_eq!(stats_before.available_state_buffers, 1);
        
        pool.clear();
        
        let stats_after = pool.stats();
        assert_eq!(stats_after.available_state_buffers, 0);
        assert_eq!(stats_after.available_obs_buffers, 0);
        assert_eq!(stats_after.available_action_buffers, 0);
    }
    
    #[test]
    fn test_pooled_buffer_raii() {
        let pool = BufferPool::new();
        let initial_stats = pool.stats();
        assert_eq!(initial_stats.available_state_buffers, 0);
        
        {
            let buffer = pool.get_state_buffer();
            let pool_clone = pool.clone();
            let _pooled = PooledBuffer::new(buffer, move |buf| pool_clone.return_state_buffer(buf));
            
            // Buffer should still be checked out
            let stats = pool.stats();
            assert_eq!(stats.available_state_buffers, 0);
        } // PooledBuffer goes out of scope here
        
        // Buffer should be automatically returned
        let final_stats = pool.stats();
        assert_eq!(final_stats.available_state_buffers, 1);
    }
    
    #[test]
    fn test_pooled_buffer_into_inner() {
        let pool = BufferPool::new();
        
        let buffer = pool.get_state_buffer();
        let pool_clone = pool.clone();
        let mut pooled = PooledBuffer::new(buffer, move |buf| pool_clone.return_state_buffer(buf));
        
        pooled.as_mut().extend_from_slice(b"test");
        let inner = pooled.into_inner();
        
        assert_eq!(inner, b"test");
        
        // Buffer should not be returned to pool
        let stats = pool.stats();
        assert_eq!(stats.available_state_buffers, 0);
    }
    
    #[test]
    fn test_pooled_buffer_deref() {
        let pool = BufferPool::new();
        let buffer = pool.get_state_buffer();
        let pool_clone = pool.clone();
        let mut pooled = PooledBuffer::new(buffer, move |buf| pool_clone.return_state_buffer(buf));
        
        // Test DerefMut
        pooled.extend_from_slice(b"hello");
        
        // Test Deref
        assert_eq!(pooled.len(), 5);
        assert_eq!(&pooled[..], b"hello");
    }
}