package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/hoorayman/rizzclaw/internal/agent"
	"github.com/hoorayman/rizzclaw/internal/agent/multiagent"
	"github.com/hoorayman/rizzclaw/internal/config"
	ctxmgr "github.com/hoorayman/rizzclaw/internal/context"
	"github.com/hoorayman/rizzclaw/pkg/bus"
	"github.com/hoorayman/rizzclaw/pkg/channels"
	"github.com/spf13/cobra"
)

var (
	gatewayDebug bool
)

var gatewayCmd = &cobra.Command{
	Use:   "gateway",
	Short: "Start RizzClaw gateway server",
	Long:  `Start RizzClaw gateway server with multi-channel support (feishu, etc.)`,
	RunE:  runGateway,
}

var gatewayConsoleCmd = &cobra.Command{
	Use:   "console",
	Short: "Start RizzClaw gateway in console mode",
	Long:  `Start RizzClaw gateway with console interface only`,
	RunE:  runGatewayConsole,
}

func init() {
	rootCmd.AddCommand(gatewayCmd)
	gatewayCmd.AddCommand(gatewayConsoleCmd)
	gatewayCmd.Flags().BoolVarP(&gatewayDebug, "debug", "d", false, "Enable debug mode to show message logs")
	gatewayConsoleCmd.Flags().BoolVarP(&gatewayDebug, "debug", "d", false, "Enable debug mode to show message logs")
}

func runGateway(cmd *cobra.Command, args []string) error {
	return startGateway(cmd.Context(), false)
}

func runGatewayConsole(cmd *cobra.Command, args []string) error {
	return startGateway(cmd.Context(), true)
}

func startGateway(ctx context.Context, consoleMode bool) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	msgBus := bus.NewMessageBus()
	defer msgBus.Close()

	var channelManager *channels.Manager
	if consoleMode {
		channelManager, err = channels.NewManager(cfg, msgBus, channels.WithConsole())
	} else {
		channelManager, err = channels.NewManager(cfg, msgBus)
	}
	if err != nil {
		return fmt.Errorf("failed to create channel manager: %w", err)
	}

	registry := multiagent.GetRegistry()
	registry.RegisterCallback(func(announce *multiagent.AnnounceMessage) {
		fmt.Printf("\n%s\n\n", announce.Message)

		if announce.Channel != "" && announce.ChatID != "" {
			msgBus.PublishOutbound(bus.OutboundMessage{
				Channel: announce.Channel,
				ChatID:  announce.ChatID,
				Content: announce.Message,
			})
		}
	})

	ag, err := agent.NewAgent("gateway", agent.WithModel(flagModel))
	if err != nil {
		return fmt.Errorf("failed to create agent: %w", err)
	}

	if flagDebug {
		ag.SetDebug(true)
	}

	if err := channelManager.StartAll(ctx); err != nil {
		return fmt.Errorf("failed to start channels: %w", err)
	}

	enabledChannels := channelManager.GetEnabledChannels()
	if len(enabledChannels) > 0 {
		fmt.Printf("✓ Channels enabled: %v\n", enabledChannels)
	} else {
		fmt.Println("⚠ Warning: No channels enabled")
	}

	sessionManager := agent.NewSessionManager()

	if consoleCh, ok := channelManager.GetChannel("console"); ok {
		if cc, ok := consoleCh.(*channels.ConsoleChannel); ok {
			cc.PrintBanner()

			cc.SetClearCallback(func() error {
				sessionKey := agent.BuildSessionKey("console", "console_chat", "console_user")

				sessionManager.ClearSession(sessionKey)

				ctxSessionMgr := ctxmgr.GetSessionManager()
				if err := ctxSessionMgr.DeleteSession(sessionKey); err != nil {
					return fmt.Errorf("failed to delete session file: %w", err)
				}

				return nil
			})
		}
	}

	fmt.Println("🦞 RizzClaw Gateway started")
	fmt.Println("Press Ctrl+C to stop")
	fmt.Println()

	go runAgentLoop(ctx, ag, msgBus, sessionManager, gatewayDebug)

	if consoleCh, ok := channelManager.GetChannel("console"); ok {
		if cc, ok := consoleCh.(*channels.ConsoleChannel); ok {
			cc.SignalStart()
		}
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	<-sigChan

	fmt.Println("\nShutting down...")
	cancel()

	if err := channelManager.StopAll(ctx); err != nil {
		return fmt.Errorf("failed to stop channels: %w", err)
	}

	fmt.Println("✓ Gateway stopped")
	return nil
}

func runAgentLoop(ctx context.Context, ag *agent.Agent, msgBus *bus.MessageBus, sessionManager *agent.SessionManager, debug bool) {
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

		sessionKey := agent.BuildSessionKey(msg.Channel, msg.ChatID, msg.UserID)

		session := sessionManager.GetOrLoadSession(sessionKey)

		if debug {
			fmt.Printf("[%s] 😎 %s (session: %s): %s\n", msg.Channel, msg.UserID, sessionKey, truncateString(msg.Content, 100))
		}

		toolCtx := multiagent.ContextWithChannel(ctx, msg.Channel, msg.ChatID)

		response, err := ag.RunWithSession(toolCtx, session, msg.Content)
		if err != nil {
			msgBus.PublishOutbound(bus.OutboundMessage{
				Channel: msg.Channel,
				ChatID:  msg.ChatID,
				Content: fmt.Sprintf("Error: %v", err),
			})
			if debug {
				fmt.Printf("[%s] ❌ Error: %v\n", msg.Channel, err)
			}
			continue
		}

		if debug {
			fmt.Printf("[%s] 🦞 Response (session: %s): %s\n", msg.Channel, sessionKey, truncateString(response, 500))
		}

		msgBus.PublishOutbound(bus.OutboundMessage{
			Channel: msg.Channel,
			ChatID:  msg.ChatID,
			Content: response,
		})
	}
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
