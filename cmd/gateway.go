package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/hoorayman/rizzclaw/internal/agent"
	"github.com/hoorayman/rizzclaw/internal/config"
	"github.com/hoorayman/rizzclaw/pkg/bus"
	"github.com/hoorayman/rizzclaw/pkg/channels"
	"github.com/spf13/cobra"
)

var gatewayCmd = &cobra.Command{
	Use:   "gateway",
	Short: "Start RizzClaw gateway server",
	Long:  `Start RizzClaw gateway server with multi-channel support (console, feishu, etc.)`,
	RunE:  runGateway,
}

func init() {
	rootCmd.AddCommand(gatewayCmd)
}

func runGateway(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	// Load config
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Create message bus
	msgBus := bus.NewMessageBus()
	defer msgBus.Close()

	// Create channel manager
	channelManager, err := channels.NewManager(cfg, msgBus)
	if err != nil {
		return fmt.Errorf("failed to create channel manager: %w", err)
	}

	// Create agent
	ag, err := agent.NewAgent("gateway", agent.WithModel(flagModel))
	if err != nil {
		return fmt.Errorf("failed to create agent: %w", err)
	}

	if flagDebug {
		ag.SetDebug(true)
	}

	// Print console banner first (before starting channels)
	if consoleCh, ok := channelManager.GetChannel("console"); ok {
		if console, ok := consoleCh.(*channels.ConsoleChannel); ok {
			console.PrintBanner()
		}
	}

	// Start channels (this will start the input loop)
	if err := channelManager.StartAll(ctx); err != nil {
		return fmt.Errorf("failed to start channels: %w", err)
	}

	enabledChannels := channelManager.GetEnabledChannels()
	if len(enabledChannels) > 0 {
		fmt.Printf("✓ Channels enabled: %v\n", enabledChannels)
	} else {
		fmt.Println("⚠ Warning: No channels enabled")
	}

	fmt.Println("🦞 RizzClaw Gateway started")
	fmt.Println("Press Ctrl+C to stop")
	fmt.Println()

	// Signal console channel to start accepting input (after all banner messages)
	if consoleCh, ok := channelManager.GetChannel("console"); ok {
		if console, ok := consoleCh.(*channels.ConsoleChannel); ok {
			console.SignalStart()
		}
	}

	// Start agent message processing loop
	go runAgentLoop(ctx, ag, msgBus)

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	<-sigChan

	fmt.Println("\nShutting down...")
	cancel()

	// Stop channels
	if err := channelManager.StopAll(ctx); err != nil {
		return fmt.Errorf("failed to stop channels: %w", err)
	}

	fmt.Println("✓ Gateway stopped")
	return nil
}

// runAgentLoop processes messages from the bus
func runAgentLoop(ctx context.Context, ag *agent.Agent, msgBus *bus.MessageBus) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		msg, ok := msgBus.ConsumeInbound(ctx)
		if !ok {
			return
		}

		// Process message with agent (silent mode for gateway)
		response, err := ag.RunSilent(ctx, msg.Content)
		if err != nil {
			// Send error response back
			msgBus.PublishOutbound(bus.OutboundMessage{
				Channel: msg.Channel,
				ChatID:  msg.ChatID,
				Content: fmt.Sprintf("Error: %v", err),
			})
			continue
		}

		// Send response back to the channel
		msgBus.PublishOutbound(bus.OutboundMessage{
			Channel: msg.Channel,
			ChatID:  msg.ChatID,
			Content: response,
		})
	}
}
