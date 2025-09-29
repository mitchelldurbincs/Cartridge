package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cartridge/actor/internal/actor"
	"github.com/cartridge/actor/internal/config"
)

var cfg *config.Config

var rootCmd = &cobra.Command{
	Use:   "actor",
	Short: "Cartridge RL Actor Service",
	Long: `Actor service that runs game episodes and collects experience data.

The actor connects to the engine service to simulate games and sends
transition data to the replay service for training.`,
	Run: runActor,
}

func init() {
	cfg = config.Default()

	// Engine settings
	rootCmd.Flags().StringVar(&cfg.EngineAddr, "engine-addr", cfg.EngineAddr, "Engine service address")
	rootCmd.Flags().StringVar(&cfg.ReplayAddr, "replay-addr", cfg.ReplayAddr, "Replay service address")

	// Actor settings
	rootCmd.Flags().StringVar(&cfg.ActorID, "actor-id", cfg.ActorID, "Unique actor identifier")
	rootCmd.Flags().StringVar(&cfg.EnvID, "env-id", cfg.EnvID, "Environment ID to run (e.g., tictactoe)")

	// Episode settings
	rootCmd.Flags().IntVar(&cfg.MaxEpisodes, "max-episodes", cfg.MaxEpisodes, "Maximum episodes to run (-1 for unlimited)")
	rootCmd.Flags().DurationVar(&cfg.EpisodeTimeout, "episode-timeout", cfg.EpisodeTimeout, "Timeout per episode")

	// Batch settings
	rootCmd.Flags().IntVar(&cfg.BatchSize, "batch-size", cfg.BatchSize, "Batch size for replay buffer")
	rootCmd.Flags().DurationVar(&cfg.FlushInterval, "flush-interval", cfg.FlushInterval, "Interval to flush partial batches")

	// Logging
	rootCmd.Flags().StringVar(&cfg.LogLevel, "log-level", cfg.LogLevel, "Log level (debug, info, warn, error)")

	// Bind flags to viper for environment variable support
	viper.BindPFlags(rootCmd.Flags())
	viper.SetEnvPrefix("ACTOR")
	viper.AutomaticEnv()
}

func runActor(cmd *cobra.Command, args []string) {
	// Validate configuration
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}

	log.Printf("Starting actor %s for environment %s", cfg.ActorID, cfg.EnvID)
	log.Printf("Engine: %s, Replay: %s", cfg.EngineAddr, cfg.ReplayAddr)

	// Create actor instance
	actorInstance, err := actor.New(cfg)
	if err != nil {
		log.Fatalf("Failed to create actor: %v", err)
	}
	defer actorInstance.Close()

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutdown signal received, stopping actor...")
		cancel()
	}()

	// Run the actor
	if err := actorInstance.Run(ctx); err != nil {
		log.Fatalf("Actor failed: %v", err)
	}

	log.Println("Actor stopped gracefully")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}