use anyhow::{anyhow, Result};
use rand::prelude::*;
use rand_chacha::ChaCha20Rng;
use crate::proto::engine::v1::Capabilities;

/// Trait for action selection policies
pub trait Policy: Send + Sync {
    /// Select an action given an observation
    fn select_action(&mut self, observation: &[u8]) -> Result<Vec<u8>>;
}

/// Random policy that selects actions uniformly at random
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
                    nvec: multi.nvec.clone()
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
                    nvec: multi.nvec.clone()
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
                let action = self.rng.gen_range(0..*n);
                Ok(action.to_le_bytes().to_vec())
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
                    return Err(anyhow!("Continuous action space low and high bounds must have same length"));
                }
                let mut action_bytes = Vec::new();
                for (&low_val, &high_val) in low.iter().zip(high.iter()) {
                    if low_val >= high_val {
                        return Err(anyhow!("Continuous action space low bound must be less than high bound"));
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
    use crate::proto::engine::v1::{Capabilities, EngineId, Encoding, MultiDiscrete, BoxSpec};

    fn create_test_capabilities(action_space: crate::proto::engine::v1::capabilities::ActionSpace) -> Capabilities {
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
            crate::proto::engine::v1::capabilities::ActionSpace::DiscreteN(4)
        );
        let mut policy = RandomPolicy::with_seed(&caps, 42).unwrap();

        for _ in 0..10 {
            let action_bytes = policy.select_action(&[]).unwrap();
            assert_eq!(action_bytes.len(), 4); // u32 = 4 bytes
            let action = u32::from_le_bytes(action_bytes.try_into().unwrap());
            assert!(action < 4);
        }
    }

    #[test]
    fn test_multi_discrete_action_space() {
        let caps = create_test_capabilities(
            crate::proto::engine::v1::capabilities::ActionSpace::Multi(MultiDiscrete {
                nvec: vec![2, 3, 4],
            })
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
            })
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
}