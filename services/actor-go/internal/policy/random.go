package policy

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"time"

	"github.com/cartridge/actor/pkg/proto/engine/v1"
)

// RandomPolicy selects random valid actions
type RandomPolicy struct {
	rng        *rand.Rand
	actionType ActionSpaceType

	// Action space parameters
	discreteN     uint32
	multiNvec     []uint32
	continuousLow []float32
	continuousHigh []float32
}

type ActionSpaceType int

const (
	ActionSpaceDiscrete ActionSpaceType = iota
	ActionSpaceMultiDiscrete
	ActionSpaceContinuous
)

// NewRandom creates a new random policy for the given action space
func NewRandom(actionSpace *engine.Capabilities_ActionSpace) (*RandomPolicy, error) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	policy := &RandomPolicy{
		rng: rng,
	}

	switch space := actionSpace.(type) {
	case *engine.Capabilities_DiscreteN:
		policy.actionType = ActionSpaceDiscrete
		policy.discreteN = space.DiscreteN

	case *engine.Capabilities_Multi:
		policy.actionType = ActionSpaceMultiDiscrete
		policy.multiNvec = space.Multi.Nvec

	case *engine.Capabilities_Continuous:
		policy.actionType = ActionSpaceContinuous
		policy.continuousLow = space.Continuous.Low
		policy.continuousHigh = space.Continuous.High

	default:
		return nil, fmt.Errorf("unsupported action space type: %T", actionSpace)
	}

	return policy, nil
}

// SelectAction implements Policy interface
func (p *RandomPolicy) SelectAction(observation []byte) ([]byte, error) {
	switch p.actionType {
	case ActionSpaceDiscrete:
		return p.selectDiscreteAction(observation)
	case ActionSpaceMultiDiscrete:
		return p.selectMultiDiscreteAction(observation)
	case ActionSpaceContinuous:
		return p.selectContinuousAction(observation)
	default:
		return nil, fmt.Errorf("unknown action space type")
	}
}

// selectDiscreteAction selects a random discrete action
func (p *RandomPolicy) selectDiscreteAction(observation []byte) ([]byte, error) {
	// For games like TicTacToe, we need to check legal moves
	// For now, we'll select from all possible actions and let the engine handle invalid moves
	// TODO: Parse observation to find legal moves (game-specific logic)

	action := uint32(p.rng.Intn(int(p.discreteN)))

	// Encode as single byte for simple discrete actions (like TicTacToe)
	if p.discreteN <= 256 {
		return []byte{byte(action)}, nil
	}

	// For larger action spaces, use little-endian encoding
	actionBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(actionBytes, action)
	return actionBytes, nil
}

// selectMultiDiscreteAction selects random actions for multi-discrete space
func (p *RandomPolicy) selectMultiDiscreteAction(observation []byte) ([]byte, error) {
	actionBytes := make([]byte, 0, len(p.multiNvec)*4)

	for _, n := range p.multiNvec {
		action := uint32(p.rng.Intn(int(n)))

		// Encode each sub-action as 4 bytes
		subActionBytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(subActionBytes, action)
		actionBytes = append(actionBytes, subActionBytes...)
	}

	return actionBytes, nil
}

// selectContinuousAction selects random continuous actions
func (p *RandomPolicy) selectContinuousAction(observation []byte) ([]byte, error) {
	if len(p.continuousLow) != len(p.continuousHigh) {
		return nil, fmt.Errorf("continuous action space bounds mismatch")
	}

	actionBytes := make([]byte, 0, len(p.continuousLow)*4)

	for i := range p.continuousLow {
		low := p.continuousLow[i]
		high := p.continuousHigh[i]

		// Random value in [low, high]
		action := low + p.rng.Float32()*(high-low)

		// Encode as little-endian float32
		actionBytes = binary.LittleEndian.AppendUint32(actionBytes,
			binary.Float32bits(action))
	}

	return actionBytes, nil
}

// TODO: Implement game-specific legal action filtering
// For TicTacToe: parse observation to find empty squares
// For other games: implement game-specific logic
func (p *RandomPolicy) getLegalActions(observation []byte) ([]uint32, error) {
	// This is a placeholder - in practice, we'd need game-specific logic
	// to parse the observation and determine legal moves

	// For now, assume all actions are legal
	switch p.actionType {
	case ActionSpaceDiscrete:
		actions := make([]uint32, p.discreteN)
		for i := uint32(0); i < p.discreteN; i++ {
			actions[i] = i
		}
		return actions, nil
	default:
		return nil, fmt.Errorf("legal action filtering not implemented for this action space")
	}
}