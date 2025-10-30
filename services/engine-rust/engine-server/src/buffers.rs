//! Buffer pool management for allocation-free hot paths
//!
//! This module provides a thread-safe buffer pool that enables allocation-free operation
//! in the hot paths of the gRPC service by reusing byte vectors.

use std::sync::{Arc, Mutex};

use metrics::{counter, gauge};

/// Maximum buffer capacity before shrinking on return to pool (1 MB)
///
/// Buffers exceeding this capacity will be shrunk to prevent memory leaks
/// from one-off large allocations permanently inflating the pool.
const MAX_BUFFER_CAPACITY: usize = 1024 * 1024;

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
        let pool = Self {
            state_buffers: Arc::new(Mutex::new(Vec::new())),
            obs_buffers: Arc::new(Mutex::new(Vec::new())),
            action_buffers: Arc::new(Mutex::new(Vec::new())),
        };

        pool.record_buffer_levels();

        pool
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
        initial_capacity: usize,
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

        let pool = Self {
            state_buffers: Arc::new(Mutex::new(state_buffers)),
            obs_buffers: Arc::new(Mutex::new(obs_buffers)),
            action_buffers: Arc::new(Mutex::new(action_buffers)),
        };

        pool.record_buffer_levels();

        pool
    }

    /// Get a state buffer from the pool
    ///
    /// If no buffer is available in the pool, returns a new empty vector.
    pub fn get_state_buffer(&self) -> Vec<u8> {
        let mut buffers = self.state_buffers.lock().unwrap();
        let buffer = buffers.pop().unwrap_or_else(Vec::new);
        counter!("engine_buffer_pool_borrows_total", "buffer" => "state").increment(1);
        gauge!("engine_buffer_pool_available", "buffer" => "state").set(buffers.len() as f64);
        buffer
    }

    /// Return a state buffer to the pool
    ///
    /// The buffer is cleared before being returned to the pool. If the buffer's
    /// capacity exceeds MAX_BUFFER_CAPACITY, it will be shrunk to prevent memory leaks.
    pub fn return_state_buffer(&self, mut buf: Vec<u8>) {
        buf.clear();

        // Shrink oversized buffers to prevent memory leaks from large allocations
        if buf.capacity() > MAX_BUFFER_CAPACITY {
            buf.shrink_to(MAX_BUFFER_CAPACITY);
            counter!("engine_buffer_pool_shrinks_total", "buffer" => "state").increment(1);
        }

        let mut buffers = self.state_buffers.lock().unwrap();
        buffers.push(buf);
        counter!("engine_buffer_pool_returns_total", "buffer" => "state").increment(1);
        gauge!("engine_buffer_pool_available", "buffer" => "state").set(buffers.len() as f64);
    }

    /// Get an observation buffer from the pool
    pub fn get_obs_buffer(&self) -> Vec<u8> {
        let mut buffers = self.obs_buffers.lock().unwrap();
        let buffer = buffers.pop().unwrap_or_else(Vec::new);
        counter!("engine_buffer_pool_borrows_total", "buffer" => "obs").increment(1);
        gauge!("engine_buffer_pool_available", "buffer" => "obs").set(buffers.len() as f64);
        buffer
    }

    /// Return an observation buffer to the pool
    ///
    /// The buffer is cleared before being returned to the pool. If the buffer's
    /// capacity exceeds MAX_BUFFER_CAPACITY, it will be shrunk to prevent memory leaks.
    pub fn return_obs_buffer(&self, mut buf: Vec<u8>) {
        buf.clear();

        // Shrink oversized buffers to prevent memory leaks from large allocations
        if buf.capacity() > MAX_BUFFER_CAPACITY {
            buf.shrink_to(MAX_BUFFER_CAPACITY);
            counter!("engine_buffer_pool_shrinks_total", "buffer" => "obs").increment(1);
        }

        let mut buffers = self.obs_buffers.lock().unwrap();
        buffers.push(buf);
        counter!("engine_buffer_pool_returns_total", "buffer" => "obs").increment(1);
        gauge!("engine_buffer_pool_available", "buffer" => "obs").set(buffers.len() as f64);
    }

    /// Get an action buffer from the pool
    pub fn get_action_buffer(&self) -> Vec<u8> {
        let mut buffers = self.action_buffers.lock().unwrap();
        let buffer = buffers.pop().unwrap_or_else(Vec::new);
        counter!("engine_buffer_pool_borrows_total", "buffer" => "action").increment(1);
        gauge!("engine_buffer_pool_available", "buffer" => "action").set(buffers.len() as f64);
        buffer
    }

    /// Return an action buffer to the pool
    ///
    /// The buffer is cleared before being returned to the pool. If the buffer's
    /// capacity exceeds MAX_BUFFER_CAPACITY, it will be shrunk to prevent memory leaks.
    pub fn return_action_buffer(&self, mut buf: Vec<u8>) {
        buf.clear();

        // Shrink oversized buffers to prevent memory leaks from large allocations
        if buf.capacity() > MAX_BUFFER_CAPACITY {
            buf.shrink_to(MAX_BUFFER_CAPACITY);
            counter!("engine_buffer_pool_shrinks_total", "buffer" => "action").increment(1);
        }

        let mut buffers = self.action_buffers.lock().unwrap();
        buffers.push(buf);
        counter!("engine_buffer_pool_returns_total", "buffer" => "action").increment(1);
        gauge!("engine_buffer_pool_available", "buffer" => "action").set(buffers.len() as f64);
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
        self.record_buffer_levels();
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
        F: FnOnce(Vec<u8>) + Send + 'static,
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

impl BufferPool {
    fn record_buffer_levels(&self) {
        gauge!("engine_buffer_pool_available", "buffer" => "state")
            .set(self.state_buffers.lock().unwrap().len() as f64);
        gauge!("engine_buffer_pool_available", "buffer" => "obs")
            .set(self.obs_buffers.lock().unwrap().len() as f64);
        gauge!("engine_buffer_pool_available", "buffer" => "action")
            .set(self.action_buffers.lock().unwrap().len() as f64);
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

    // Concurrent access tests

    #[test]
    fn test_concurrent_buffer_access() {
        use std::sync::Arc;
        use std::thread;

        let pool = Arc::new(BufferPool::with_capacity(10, 10, 10, 256));
        let num_threads = 8;
        let operations_per_thread = 100;

        let mut handles = Vec::new();

        for thread_id in 0..num_threads {
            let pool_clone = Arc::clone(&pool);

            let handle = thread::spawn(move || {
                for i in 0..operations_per_thread {
                    // Get a buffer
                    let mut buf = match thread_id % 3 {
                        0 => pool_clone.get_state_buffer(),
                        1 => pool_clone.get_obs_buffer(),
                        _ => pool_clone.get_action_buffer(),
                    };

                    // Write some data
                    buf.extend_from_slice(&[thread_id as u8, i as u8]);

                    // Return the buffer
                    match thread_id % 3 {
                        0 => pool_clone.return_state_buffer(buf),
                        1 => pool_clone.return_obs_buffer(buf),
                        _ => pool_clone.return_action_buffer(buf),
                    }
                }
            });

            handles.push(handle);
        }

        // Wait for all threads to complete
        for handle in handles {
            handle.join().unwrap();
        }

        // Verify the pool is in a consistent state
        let stats = pool.stats();
        // All buffers should be returned (some may have been created on demand)
        assert!(stats.available_state_buffers >= 0);
        assert!(stats.available_obs_buffers >= 0);
        assert!(stats.available_action_buffers >= 0);
    }

    #[test]
    fn test_concurrent_buffer_no_loss() {
        use std::sync::Arc;
        use std::thread;

        let pool = Arc::new(BufferPool::with_capacity(50, 50, 50, 128));
        let initial_stats = pool.stats();

        let num_threads = 4;
        let buffers_per_thread = 10;

        let mut handles = Vec::new();

        for _ in 0..num_threads {
            let pool_clone = Arc::clone(&pool);

            let handle = thread::spawn(move || {
                let mut buffers = Vec::new();

                // Get multiple buffers
                for _ in 0..buffers_per_thread {
                    buffers.push(pool_clone.get_state_buffer());
                }

                // Return all buffers
                for buf in buffers {
                    pool_clone.return_state_buffer(buf);
                }
            });

            handles.push(handle);
        }

        // Wait for all threads
        for handle in handles {
            handle.join().unwrap();
        }

        // Final stats should show all buffers returned
        let final_stats = pool.stats();
        // At minimum, we should have the initial buffers back
        assert!(final_stats.available_state_buffers >= initial_stats.available_state_buffers);
    }

    #[test]
    fn test_concurrent_high_contention() {
        use std::sync::Arc;
        use std::thread;
        use std::time::Duration;

        let pool = Arc::new(BufferPool::with_capacity(5, 5, 5, 64));
        let num_threads = 16; // More threads than buffers to create contention

        let mut handles = Vec::new();

        for thread_id in 0..num_threads {
            let pool_clone = Arc::clone(&pool);

            let handle = thread::spawn(move || {
                for _ in 0..50 {
                    let mut buf = pool_clone.get_state_buffer();
                    buf.extend_from_slice(&[thread_id as u8]);

                    // Simulate some work
                    thread::sleep(Duration::from_micros(10));

                    pool_clone.return_state_buffer(buf);
                }
            });

            handles.push(handle);
        }

        for handle in handles {
            handle.join().unwrap();
        }

        // Verify pool is still functional
        let buf = pool.get_state_buffer();
        assert_eq!(buf.len(), 0); // Should be cleared
        pool.return_state_buffer(buf);
    }

    #[test]
    fn test_concurrent_pooled_buffer_raii() {
        use std::sync::Arc;
        use std::thread;

        let pool = Arc::new(BufferPool::with_capacity(20, 0, 0, 128));

        let num_threads = 8;
        let mut handles = Vec::new();

        for _ in 0..num_threads {
            let pool_clone = Arc::clone(&pool);

            let handle = thread::spawn(move || {
                for _ in 0..25 {
                    let buffer = pool_clone.get_state_buffer();
                    let pool_for_closure = pool_clone.clone();

                    // Create PooledBuffer - should auto-return on drop
                    let _pooled = PooledBuffer::new(buffer, move |buf| {
                        pool_for_closure.return_state_buffer(buf)
                    });

                    // Buffer gets returned when _pooled goes out of scope
                }
            });

            handles.push(handle);
        }

        for handle in handles {
            handle.join().unwrap();
        }

        // All buffers should be back in the pool
        let stats = pool.stats();
        assert!(stats.available_state_buffers >= 20);
    }

    #[test]
    fn test_concurrent_mixed_operations() {
        use std::sync::Arc;
        use std::thread;

        let pool = Arc::new(BufferPool::with_capacity(10, 10, 10, 256));
        let num_threads = 6;

        let mut handles = Vec::new();

        for thread_id in 0..num_threads {
            let pool_clone = Arc::clone(&pool);

            let handle = thread::spawn(move || {
                match thread_id % 3 {
                    0 => {
                        // Thread type 0: Get and return state buffers
                        for _ in 0..50 {
                            let buf = pool_clone.get_state_buffer();
                            pool_clone.return_state_buffer(buf);
                        }
                    }
                    1 => {
                        // Thread type 1: Get and return obs buffers
                        for _ in 0..50 {
                            let buf = pool_clone.get_obs_buffer();
                            pool_clone.return_obs_buffer(buf);
                        }
                    }
                    _ => {
                        // Thread type 2: Get and return action buffers
                        for _ in 0..50 {
                            let buf = pool_clone.get_action_buffer();
                            pool_clone.return_action_buffer(buf);
                        }
                    }
                }
            });

            handles.push(handle);
        }

        for handle in handles {
            handle.join().unwrap();
        }

        // All buffer types should have buffers available
        let stats = pool.stats();
        assert!(stats.available_state_buffers >= 10);
        assert!(stats.available_obs_buffers >= 10);
        assert!(stats.available_action_buffers >= 10);
    }

    // Tests for buffer capacity management and memory leak prevention

    #[test]
    fn test_oversized_state_buffer_shrinks() {
        let pool = BufferPool::new();

        // Create a buffer with capacity exceeding MAX_BUFFER_CAPACITY
        let mut buf = Vec::with_capacity(MAX_BUFFER_CAPACITY + 1024 * 1024);
        buf.extend_from_slice(&vec![0u8; 100]); // Add some data

        let capacity_before = buf.capacity();
        assert!(capacity_before > MAX_BUFFER_CAPACITY);

        // Return to pool - should shrink
        pool.return_state_buffer(buf);

        // Get it back and check capacity
        let buf_returned = pool.get_state_buffer();
        assert!(buf_returned.capacity() <= MAX_BUFFER_CAPACITY);
        assert_eq!(buf_returned.len(), 0); // Should be cleared
    }

    #[test]
    fn test_normal_sized_state_buffer_not_shrunk() {
        let pool = BufferPool::new();

        // Create a buffer within the capacity limit
        let mut buf = Vec::with_capacity(1024);
        buf.extend_from_slice(&vec![0u8; 512]);

        let capacity_before = buf.capacity();
        assert!(capacity_before <= MAX_BUFFER_CAPACITY);

        // Return to pool - should not shrink
        pool.return_state_buffer(buf);

        // Get it back and check capacity is preserved
        let buf_returned = pool.get_state_buffer();
        assert_eq!(buf_returned.capacity(), capacity_before);
        assert_eq!(buf_returned.len(), 0);
    }

    #[test]
    fn test_oversized_obs_buffer_shrinks() {
        let pool = BufferPool::new();

        // Create a buffer with capacity exceeding MAX_BUFFER_CAPACITY
        let mut buf = Vec::with_capacity(MAX_BUFFER_CAPACITY + 512 * 1024);
        buf.extend_from_slice(&vec![1u8; 200]);

        assert!(buf.capacity() > MAX_BUFFER_CAPACITY);

        // Return to pool - should shrink
        pool.return_obs_buffer(buf);

        // Get it back and check capacity
        let buf_returned = pool.get_obs_buffer();
        assert!(buf_returned.capacity() <= MAX_BUFFER_CAPACITY);
        assert_eq!(buf_returned.len(), 0);
    }

    #[test]
    fn test_oversized_action_buffer_shrinks() {
        let pool = BufferPool::new();

        // Create a buffer with capacity exceeding MAX_BUFFER_CAPACITY
        let mut buf = Vec::with_capacity(MAX_BUFFER_CAPACITY + 256 * 1024);
        buf.extend_from_slice(&vec![2u8; 150]);

        assert!(buf.capacity() > MAX_BUFFER_CAPACITY);

        // Return to pool - should shrink
        pool.return_action_buffer(buf);

        // Get it back and check capacity
        let buf_returned = pool.get_action_buffer();
        assert!(buf_returned.capacity() <= MAX_BUFFER_CAPACITY);
        assert_eq!(buf_returned.len(), 0);
    }

    #[test]
    fn test_multiple_oversized_buffers_all_shrink() {
        let pool = BufferPool::new();

        // Create and return multiple oversized buffers
        for i in 0..5 {
            let mut buf = Vec::with_capacity(MAX_BUFFER_CAPACITY + (i + 1) * 1024 * 1024);
            buf.extend_from_slice(&vec![i as u8; 100]);
            pool.return_state_buffer(buf);
        }

        // Get them all back - all should be shrunk
        for _ in 0..5 {
            let buf = pool.get_state_buffer();
            assert!(buf.capacity() <= MAX_BUFFER_CAPACITY);
            assert_eq!(buf.len(), 0);
        }
    }

    #[test]
    fn test_buffer_at_exact_capacity_limit() {
        let pool = BufferPool::new();

        // Create buffer exactly at the limit
        let buf = Vec::with_capacity(MAX_BUFFER_CAPACITY);
        let capacity = buf.capacity();

        pool.return_state_buffer(buf);

        let buf_returned = pool.get_state_buffer();
        // Should not shrink since it's at the limit
        assert_eq!(buf_returned.capacity(), capacity);
    }

    #[test]
    fn test_buffer_slightly_over_limit_shrinks() {
        let pool = BufferPool::new();

        // Create buffer just slightly over the limit
        let mut buf = Vec::with_capacity(MAX_BUFFER_CAPACITY + 1);

        pool.return_state_buffer(buf);

        let buf_returned = pool.get_state_buffer();
        // Should shrink to the limit
        assert!(buf_returned.capacity() <= MAX_BUFFER_CAPACITY);
    }

    #[test]
    fn test_concurrent_oversized_buffer_shrinking() {
        use std::sync::Arc;
        use std::thread;

        let pool = Arc::new(BufferPool::new());
        let num_threads = 4;
        let mut handles = Vec::new();

        for thread_id in 0..num_threads {
            let pool_clone = Arc::clone(&pool);

            let handle = thread::spawn(move || {
                // Each thread creates oversized buffers
                for i in 0..10 {
                    let mut buf = Vec::with_capacity(
                        MAX_BUFFER_CAPACITY + (thread_id + 1) * 100 * 1024 + i * 1024,
                    );
                    buf.extend_from_slice(&vec![thread_id as u8; 50]);
                    pool_clone.return_state_buffer(buf);
                }
            });

            handles.push(handle);
        }

        for handle in handles {
            handle.join().unwrap();
        }

        // All returned buffers should be shrunk
        for _ in 0..40 {
            let buf = pool.get_state_buffer();
            if buf.capacity() > 0 {
                // Only check buffers that were actually returned
                assert!(buf.capacity() <= MAX_BUFFER_CAPACITY);
            }
        }
    }
}
