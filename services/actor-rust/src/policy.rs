use crate::proto::engine::v1::Capabilities;
use anyhow::{anyhow, Result};
use rand::prelude::*;
use rand_chacha::ChaCha20Rng;

/// Trait for action selection policies
pub trait Policy: Send + Sync {
    /// Select an action given an observation
    fn select_action(&mut self, observation: &[u8]) -> Result<Vec<u8>>;
}

/// Random policy that selects actions uniformly at random
#[derive(Debug)]
pub struct RandomPolicy {
    rng: ChaCha20Rng,
    action_space: ActionSpace,
}

#[derive(Debug, Clone)]
enum ActionSpace {
    Discrete { n: u32 },
    MultiDiscrete { nvec: Vec<u32> },
    Continuous { low: Vec<f32>, high: Vec<f32> },
}

impl RandomPolicy {
    pub fn new(capabilities: &Capabilities) -> Result<Self> {
        let action_space = match &capabilities.action_space {
            Some(crate::proto::engine::v1::capabilities::ActionSpace::DiscreteN(n)) => {
                ActionSpace::Discrete { n: *n }
            }
            Some(crate::proto::engine::v1::capabilities::ActionSpace::Multi(multi)) => {
                ActionSpace::MultiDiscrete {
                    nvec: multi.nvec.clone(),
                }
            }
            Some(crate::proto::engine::v1::capabilities::ActionSpace::Continuous(box_spec)) => {
                ActionSpace::Continuous {
                    low: box_spec.low.clone(),
                    high: box_spec.high.clone(),
                }
            }
            None => {
                return Err(anyhow!("No action space specified in capabilities"));
            }
        };

        // Use a random seed for the RNG - in production this could be configurable
        let rng = ChaCha20Rng::from_entropy();

        Ok(Self { rng, action_space })
    }

    #[allow(dead_code)]
    pub fn with_seed(capabilities: &Capabilities, seed: u64) -> Result<Self> {
        let action_space = match &capabilities.action_space {
            Some(crate::proto::engine::v1::capabilities::ActionSpace::DiscreteN(n)) => {
                ActionSpace::Discrete { n: *n }
            }
            Some(crate::proto::engine::v1::capabilities::ActionSpace::Multi(multi)) => {
                ActionSpace::MultiDiscrete {
                    nvec: multi.nvec.clone(),
                }
            }
            Some(crate::proto::engine::v1::capabilities::ActionSpace::Continuous(box_spec)) => {
                ActionSpace::Continuous {
                    low: box_spec.low.clone(),
                    high: box_spec.high.clone(),
                }
            }
            None => {
                return Err(anyhow!("No action space specified in capabilities"));
            }
        };

        let rng = ChaCha20Rng::seed_from_u64(seed);

        Ok(Self { rng, action_space })
    }
}

impl Policy for RandomPolicy {
    fn select_action(&mut self, _observation: &[u8]) -> Result<Vec<u8>> {
        match &self.action_space {
            ActionSpace::Discrete { n } => {
                if *n == 0 {
                    return Err(anyhow!("Discrete action space must have n > 0"));
                }
                if *n > 255 {
                    return Err(anyhow!("Discrete action space with n > 255 not supported with single byte encoding"));
                }
                let action = self.rng.gen_range(0..*n) as u8;
                Ok(vec![action])
            }
            ActionSpace::MultiDiscrete { nvec } => {
                let mut action_bytes = Vec::new();
                for &n in nvec {
                    if n == 0 {
                        return Err(anyhow!("Multi-discrete action space must have all n > 0"));
                    }
                    let action = self.rng.gen_range(0..n);
                    action_bytes.extend_from_slice(&action.to_le_bytes());
                }
                Ok(action_bytes)
            }
            ActionSpace::Continuous { low, high } => {
                if low.len() != high.len() {
                    return Err(anyhow!(
                        "Continuous action space low and high bounds must have same length"
                    ));
                }
                let mut action_bytes = Vec::new();
                for (&low_val, &high_val) in low.iter().zip(high.iter()) {
                    if low_val >= high_val {
                        return Err(anyhow!(
                            "Continuous action space low bound must be less than high bound"
                        ));
                    }
                    let action: f32 = self.rng.gen_range(low_val..high_val);
                    action_bytes.extend_from_slice(&action.to_le_bytes());
                }
                Ok(action_bytes)
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::proto::engine::v1::{BoxSpec, Capabilities, Encoding, EngineId, MultiDiscrete};

    fn create_test_capabilities(
        action_space: crate::proto::engine::v1::capabilities::ActionSpace,
    ) -> Capabilities {
        Capabilities {
            id: Some(EngineId {
                env_id: "test".to_string(),
                build_id: "0.1.0".to_string(),
            }),
            enc: Some(Encoding {
                state: "test:v1".to_string(),
                action: "test:v1".to_string(),
                obs: "test:v1".to_string(),
                schema_version: 1,
            }),
            max_horizon: 100,
            action_space: Some(action_space),
            preferred_batch: 32,
        }
    }

    #[test]
    fn test_discrete_action_space() {
        let caps = create_test_capabilities(
            crate::proto::engine::v1::capabilities::ActionSpace::DiscreteN(4),
        );
        let mut policy = RandomPolicy::with_seed(&caps, 42).unwrap();

        for _ in 0..10 {
            let action_bytes = policy.select_action(&[]).unwrap();
            assert_eq!(action_bytes.len(), 1); // u8 = 1 byte
            let action = action_bytes[0] as u32;
            assert!(action < 4);
        }
    }

    #[test]
    fn test_multi_discrete_action_space() {
        let caps = create_test_capabilities(
            crate::proto::engine::v1::capabilities::ActionSpace::Multi(MultiDiscrete {
                nvec: vec![2, 3, 4],
            }),
        );
        let mut policy = RandomPolicy::with_seed(&caps, 42).unwrap();

        for _ in 0..10 {
            let action_bytes = policy.select_action(&[]).unwrap();
            assert_eq!(action_bytes.len(), 12); // 3 * u32 = 12 bytes

            let action1 = u32::from_le_bytes(action_bytes[0..4].try_into().unwrap());
            let action2 = u32::from_le_bytes(action_bytes[4..8].try_into().unwrap());
            let action3 = u32::from_le_bytes(action_bytes[8..12].try_into().unwrap());

            assert!(action1 < 2);
            assert!(action2 < 3);
            assert!(action3 < 4);
        }
    }

    #[test]
    fn test_continuous_action_space() {
        let caps = create_test_capabilities(
            crate::proto::engine::v1::capabilities::ActionSpace::Continuous(BoxSpec {
                low: vec![-1.0, 0.0],
                high: vec![1.0, 2.0],
                shape: vec![2],
            }),
        );
        let mut policy = RandomPolicy::with_seed(&caps, 42).unwrap();

        for _ in 0..10 {
            let action_bytes = policy.select_action(&[]).unwrap();
            assert_eq!(action_bytes.len(), 8); // 2 * f32 = 8 bytes

            let action1 = f32::from_le_bytes(action_bytes[0..4].try_into().unwrap());
            let action2 = f32::from_le_bytes(action_bytes[4..8].try_into().unwrap());

            assert!(action1 >= -1.0 && action1 < 1.0);
            assert!(action2 >= 0.0 && action2 < 2.0);
        }
    }

    #[test]
    fn test_policy_determinism_with_same_seed() {
        let caps = create_test_capabilities(
            crate::proto::engine::v1::capabilities::ActionSpace::DiscreteN(10),
        );

        // Create two policies with the same seed
        let mut policy1 = RandomPolicy::with_seed(&caps, 12345).unwrap();
        let mut policy2 = RandomPolicy::with_seed(&caps, 12345).unwrap();

        // Both policies should produce the same sequence of actions
        for _ in 0..20 {
            let action1 = policy1.select_action(&[]).unwrap();
            let action2 = policy2.select_action(&[]).unwrap();
            assert_eq!(action1, action2, "policies with same seed should produce same actions");
        }
    }

    #[test]
    fn test_policy_different_with_different_seeds() {
        let caps = create_test_capabilities(
            crate::proto::engine::v1::capabilities::ActionSpace::DiscreteN(10),
        );

        let mut policy1 = RandomPolicy::with_seed(&caps, 11111).unwrap();
        let mut policy2 = RandomPolicy::with_seed(&caps, 22222).unwrap();

        // With different seeds, at least one action should differ in 20 samples
        let mut found_difference = false;
        for _ in 0..20 {
            let action1 = policy1.select_action(&[]).unwrap();
            let action2 = policy2.select_action(&[]).unwrap();
            if action1 != action2 {
                found_difference = true;
                break;
            }
        }
        assert!(found_difference, "policies with different seeds should produce different actions");
    }

    #[test]
    fn test_discrete_action_space_with_n_zero_fails() {
        let caps = create_test_capabilities(
            crate::proto::engine::v1::capabilities::ActionSpace::DiscreteN(0),
        );
        let mut policy = RandomPolicy::with_seed(&caps, 42).unwrap();

        let result = policy.select_action(&[]);
        assert!(result.is_err());
        assert!(result.unwrap_err().to_string().contains("must have n > 0"));
    }

    #[test]
    fn test_discrete_action_space_with_large_n_fails() {
        let caps = create_test_capabilities(
            crate::proto::engine::v1::capabilities::ActionSpace::DiscreteN(256),
        );
        let mut policy = RandomPolicy::with_seed(&caps, 42).unwrap();

        let result = policy.select_action(&[]);
        assert!(result.is_err());
        assert!(result.unwrap_err().to_string().contains("n > 255 not supported"));
    }

    #[test]
    fn test_multi_discrete_with_zero_element_fails() {
        let caps = create_test_capabilities(
            crate::proto::engine::v1::capabilities::ActionSpace::Multi(MultiDiscrete {
                nvec: vec![2, 0, 4], // One element is 0
            }),
        );
        let mut policy = RandomPolicy::with_seed(&caps, 42).unwrap();

        let result = policy.select_action(&[]);
        assert!(result.is_err());
        assert!(result.unwrap_err().to_string().contains("must have all n > 0"));
    }

    #[test]
    fn test_continuous_action_space_invalid_bounds_fails() {
        let caps = create_test_capabilities(
            crate::proto::engine::v1::capabilities::ActionSpace::Continuous(BoxSpec {
                low: vec![1.0, 0.0],  // low >= high for first element
                high: vec![0.0, 2.0],
                shape: vec![2],
            }),
        );
        let mut policy = RandomPolicy::with_seed(&caps, 42).unwrap();

        let result = policy.select_action(&[]);
        assert!(result.is_err());
        assert!(result.unwrap_err().to_string().contains("low bound must be less than high bound"));
    }

    #[test]
    fn test_continuous_action_space_mismatched_bounds_fails() {
        let caps = create_test_capabilities(
            crate::proto::engine::v1::capabilities::ActionSpace::Continuous(BoxSpec {
                low: vec![-1.0, 0.0],
                high: vec![1.0, 2.0, 3.0], // Different length
                shape: vec![2],
            }),
        );
        let mut policy = RandomPolicy::with_seed(&caps, 42).unwrap();

        let result = policy.select_action(&[]);
        assert!(result.is_err());
        assert!(result.unwrap_err().to_string().contains("must have same length"));
    }

    #[test]
    fn test_no_action_space_fails() {
        let mut caps = create_test_capabilities(
            crate::proto::engine::v1::capabilities::ActionSpace::DiscreteN(4),
        );
        caps.action_space = None;

        let result = RandomPolicy::with_seed(&caps, 42);
        assert!(result.is_err());
        assert!(result.unwrap_err().to_string().contains("No action space specified"));
    }
}
