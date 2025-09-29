package actor

import (
	"context"
	"fmt"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/cartridge/actor/internal/config"
	"github.com/cartridge/actor/internal/policy"
	"github.com/cartridge/actor/pkg/proto/engine/v1"
	"github.com/cartridge/actor/pkg/proto/replay/v1"
)

// Actor represents a single game-playing agent
type Actor struct {
	cfg *config.Config

	// gRPC clients
	engineClient engine.EngineClient
	replayClient replay.ReplayClient

	// Connections (for cleanup)
	engineConn *grpc.ClientConn
	replayConn *grpc.ClientConn

	// Policy for action selection
	policy policy.Policy

	// Episode tracking
	episodeCount int
	transitionBuffer []*replay.Transition
}

// New creates a new actor instance
func New(cfg *config.Config) (*Actor, error) {
	// Connect to engine service
	engineConn, err := grpc.Dial(cfg.EngineAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to engine at %s: %w", cfg.EngineAddr, err)
	}

	// Connect to replay service
	replayConn, err := grpc.Dial(cfg.ReplayAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		engineConn.Close()
		return nil, fmt.Errorf("failed to connect to replay at %s: %w", cfg.ReplayAddr, err)
	}

	// Create clients
	engineClient := engine.NewEngineClient(engineConn)
	replayClient := replay.NewReplayClient(replayConn)

	// Get game capabilities to configure policy
	capabilities, err := engineClient.GetCapabilities(context.Background(), &engine.EngineId{
		EnvId:   cfg.EnvID,
		BuildId: "actor",
	})
	if err != nil {
		engineConn.Close()
		replayConn.Close()
		return nil, fmt.Errorf("failed to get capabilities for %s: %w", cfg.EnvID, err)
	}

	// Create random policy based on action space
	randomPolicy, err := policy.NewRandom(capabilities.GetActionSpace())
	if err != nil {
		engineConn.Close()
		replayConn.Close()
		return nil, fmt.Errorf("failed to create policy: %w", err)
	}

	actor := &Actor{
		cfg:              cfg,
		engineClient:     engineClient,
		replayClient:     replayClient,
		engineConn:       engineConn,
		replayConn:       replayConn,
		policy:           randomPolicy,
		transitionBuffer: make([]*replay.Transition, 0, cfg.BatchSize),
	}

	log.Printf("Actor %s initialized for environment %s", cfg.ActorID, cfg.EnvID)
	log.Printf("Game capabilities: max_horizon=%d, preferred_batch=%d",
		capabilities.MaxHorizon, capabilities.PreferredBatch)

	return actor, nil
}

// Close cleans up resources
func (a *Actor) Close() error {
	// Flush any remaining transitions
	if len(a.transitionBuffer) > 0 {
		if err := a.flushBuffer(context.Background()); err != nil {
			log.Printf("Failed to flush buffer on close: %v", err)
		}
	}

	var err1, err2 error
	if a.engineConn != nil {
		err1 = a.engineConn.Close()
	}
	if a.replayConn != nil {
		err2 = a.replayConn.Close()
	}

	if err1 != nil {
		return err1
	}
	return err2
}

// Run starts the actor main loop
func (a *Actor) Run(ctx context.Context) error {
	log.Printf("Actor %s starting main loop", a.cfg.ActorID)

	// Setup flush timer for partial batches
	flushTicker := time.NewTicker(a.cfg.FlushInterval)
	defer flushTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Context cancelled, stopping actor")
			return ctx.Err()

		case <-flushTicker.C:
			// Flush partial batches periodically
			if len(a.transitionBuffer) > 0 {
				if err := a.flushBuffer(ctx); err != nil {
					log.Printf("Failed to flush buffer: %v", err)
				}
			}

		default:
			// Check episode limit
			if a.cfg.MaxEpisodes > 0 && a.episodeCount >= a.cfg.MaxEpisodes {
				log.Printf("Reached maximum episodes (%d), stopping", a.cfg.MaxEpisodes)
				return nil
			}

			// Run an episode
			if err := a.runEpisode(ctx); err != nil {
				log.Printf("Episode %d failed: %v", a.episodeCount+1, err)
				// Continue with next episode rather than stopping
				continue
			}

			a.episodeCount++
			if a.episodeCount%10 == 0 {
				log.Printf("Completed %d episodes", a.episodeCount)
			}
		}
	}
}

// runEpisode runs a single game episode and collects transitions
func (a *Actor) runEpisode(ctx context.Context) error {
	episodeCtx, cancel := context.WithTimeout(ctx, a.cfg.EpisodeTimeout)
	defer cancel()

	// Reset the game
	resetResp, err := a.engineClient.Reset(episodeCtx, &engine.ResetRequest{
		Id: &engine.EngineId{
			EnvId:   a.cfg.EnvID,
			BuildId: "actor",
		},
		Seed: uint64(time.Now().UnixNano()), // Random seed for variety
		Hint: nil,
	})
	if err != nil {
		return fmt.Errorf("failed to reset game: %w", err)
	}

	episodeID := fmt.Sprintf("%s-ep-%d-%d", a.cfg.ActorID, a.episodeCount, time.Now().Unix())
	currentState := resetResp.State
	currentObs := resetResp.Obs
	stepNumber := uint32(0)

	for {
		select {
		case <-episodeCtx.Done():
			return fmt.Errorf("episode timed out")
		default:
		}

		// Select action using policy
		action, err := a.policy.SelectAction(currentObs)
		if err != nil {
			return fmt.Errorf("failed to select action: %w", err)
		}

		// Take step in environment
		stepResp, err := a.engineClient.Step(episodeCtx, &engine.StepRequest{
			Id: &engine.EngineId{
				EnvId:   a.cfg.EnvID,
				BuildId: "actor",
			},
			State:  currentState,
			Action: action,
		})
		if err != nil {
			return fmt.Errorf("failed to step environment: %w", err)
		}

		// Create transition
		transition := &replay.Transition{
			Id:              fmt.Sprintf("%s-step-%d", episodeID, stepNumber),
			EnvId:           a.cfg.EnvID,
			EpisodeId:       episodeID,
			StepNumber:      stepNumber,
			State:           currentState,
			Action:          action,
			NextState:       stepResp.State,
			Observation:     currentObs,
			NextObservation: stepResp.Obs,
			Reward:          stepResp.Reward,
			Done:            stepResp.Done,
			Priority:        1.0, // Default priority
			Timestamp:       uint64(time.Now().Unix()),
		}

		// Add to buffer
		a.transitionBuffer = append(a.transitionBuffer, transition)

		// Flush buffer if full
		if len(a.transitionBuffer) >= a.cfg.BatchSize {
			if err := a.flushBuffer(episodeCtx); err != nil {
				return fmt.Errorf("failed to flush buffer: %w", err)
			}
		}

		// Check if episode is done
		if stepResp.Done {
			log.Printf("Episode %s completed in %d steps, final reward: %.2f",
				episodeID, stepNumber+1, stepResp.Reward)
			break
		}

		// Update state for next step
		currentState = stepResp.State
		currentObs = stepResp.Obs
		stepNumber++
	}

	return nil
}

// flushBuffer sends accumulated transitions to replay service
func (a *Actor) flushBuffer(ctx context.Context) error {
	if len(a.transitionBuffer) == 0 {
		return nil
	}

	log.Printf("Flushing %d transitions to replay service", len(a.transitionBuffer))

	_, err := a.replayClient.StoreBatch(ctx, &replay.StoreBatchRequest{
		Transitions: a.transitionBuffer,
	})
	if err != nil {
		return fmt.Errorf("failed to store batch: %w", err)
	}

	// Clear buffer
	a.transitionBuffer = a.transitionBuffer[:0]
	return nil
}