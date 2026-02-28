package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/hoorayman/rizzclaw/internal/agent"
	"github.com/hoorayman/rizzclaw/internal/config"
	ctxmgr "github.com/hoorayman/rizzclaw/internal/context"
	"github.com/hoorayman/rizzclaw/internal/minimax"
	"github.com/spf13/cobra"
)

var (
	Version = "0.1.0"
)

var rootCmd = &cobra.Command{
	Use:   "rizzclaw",
	Short: "RizzClaw - A Go implementation of OpenClaw with MiniMax support",
	Long: `RizzClaw is a CLI tool for interacting with LLM providers.
Currently supports MiniMax as the LLM provider.`,
	Version: Version,
}

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Start an interactive chat session",
	Long:  `Start an interactive chat session with the MiniMax LLM.`,
	RunE:  runChat,
}

var modelsCmd = &cobra.Command{
	Use:   "models",
	Short: "List available models",
	Long:  `List all available models from configured providers.`,
	RunE:  runModels,
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration",
	Long:  `Manage RizzClaw configuration settings.`,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	RunE:  runConfigShow,
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Args:  cobra.ExactArgs(2),
	RunE:  runConfigSet,
}

var (
	flagModel        string
	flagSystemPrompt string
	flagProvider     string
	flagDebug        bool
)

func init() {
	chatCmd.Flags().StringVarP(&flagModel, "model", "m", minimax.DefaultModel, "Model to use")
	chatCmd.Flags().StringVarP(&flagSystemPrompt, "system", "s", "", "System prompt")
	chatCmd.Flags().StringVarP(&flagProvider, "provider", "p", "minimax", "LLM provider")
	chatCmd.Flags().BoolVarP(&flagDebug, "debug", "d", false, "Enable debug output")

	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configSetCmd)

	rootCmd.AddCommand(chatCmd)
	rootCmd.AddCommand(modelsCmd)
	rootCmd.AddCommand(configCmd)
}

func Execute() error {
	return rootCmd.Execute()
}

func printLogo() {
	logo := `
       ▐██████▌            
      ▐██  ●  ●  ██▌        
      ▝████████▘            
        ▘▘    ▝▝       `
	fmt.Println(logo)
}

func runChat(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	ctxMgr := ctxmgr.GetManager()
	_ = ctxMgr

	memStore := ctxmgr.GetMemoryStore()
	_ = memStore

	sessMgr := ctxmgr.GetSessionManager()
	_ = sessMgr

	sessMgr.CleanupOldSessions(100)
	memStore.CleanupOldMemories(10000)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nReceived interrupt signal, shutting down...")
		cancel()
	}()

	opts := []agent.AgentOption{
		agent.WithModel(flagModel),
	}

	if flagSystemPrompt != "" {
		opts = append(opts, agent.WithSystemPrompt(flagSystemPrompt))
	}

	sessions, err := sessMgr.ListSessions()
	if err == nil && len(sessions) > 0 {
		for i := len(sessions) - 1; i >= 0; i-- {
			sessionID := sessions[i]
			savedSession, err := sessMgr.LoadSession(sessionID)
			if err == nil && savedSession != nil && len(savedSession.Messages) > 0 {
				agentSession := &agent.Session{
					ID:        savedSession.ID,
					CreatedAt: savedSession.CreatedAt,
					UpdatedAt: savedSession.UpdatedAt,
					Messages:  make([]agent.Message, len(savedSession.Messages)),
					Metadata:  make(map[string]any),
				}
				for i, msg := range savedSession.Messages {
					agentSession.Messages[i] = agent.Message{
						Role:      msg.Role,
						Content:   msg.Content,
						Timestamp: msg.Timestamp,
					}
				}
				opts = append(opts, agent.WithSession(agentSession))
				fmt.Printf("Resumed session: %s (%d messages)\n", sessionID, len(savedSession.Messages))
				break
			}
		}
	}

	ag, err := agent.NewAgent("default", opts...)
	if err != nil {
		return fmt.Errorf("failed to create agent: %w", err)
	}

	if flagDebug {
		ag.SetDebug(true)
	}

	printLogo()
	fmt.Println("RizzClaw Chat")
	fmt.Printf("Model: %s\n", flagModel)
	if flagDebug {
		fmt.Println("Debug: enabled")
	}
	fmt.Println("Type your message and press Enter. Type '/exit' to quit, '/clear' to clear session.")
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		fmt.Print("😎: ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		switch input {
		case "/exit", "/quit":
			fmt.Println("Goodbye!")
			return nil
		case "/clear":
			ag.ClearSession()
			fmt.Println("Session cleared.")
			continue
		case "/help":
			fmt.Println("Commands:")
			fmt.Println("  /exit, /quit - Exit the chat")
			fmt.Println("  /clear - Clear the session")
			fmt.Println("  /help - Show this help message")
			continue
		}

		fmt.Print("\n🦞: ")
		response, err := ag.Run(ctx, input)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}

		if response == "" {
			fmt.Println("(no response)")
		}
		fmt.Println()
	}
}

func runModels(cmd *cobra.Command, args []string) error {
	fmt.Println("Available MiniMax Models:")
	fmt.Println()

	models := minimax.ListModels()
	for _, m := range models {
		reasoning := ""
		if m.Reasoning {
			reasoning = " (reasoning)"
		}
		inputTypes := strings.Join(m.Input, ", ")
		fmt.Printf("  %s%s\n", m.ID, reasoning)
		fmt.Printf("    Name: %s\n", m.Name)
		fmt.Printf("    Context: %d tokens\n", m.ContextWindow)
		fmt.Printf("    Max Output: %d tokens\n", m.MaxTokens)
		fmt.Printf("    Input Types: %s\n", inputTypes)
		fmt.Printf("    Cost: $%.2f/1M input, $%.2f/1M output\n", m.Cost.Input, m.Cost.Output)
		fmt.Println()
	}

	return nil
}

func runConfigShow(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	fmt.Println("Current Configuration:")
	fmt.Printf("  Config Path: %s\n", config.GetConfigPath())
	fmt.Println()

	fmt.Println("  Providers:")
	for name, provider := range cfg.Models.Providers {
		fmt.Printf("    %s:\n", name)
		fmt.Printf("      Base URL: %s\n", provider.BaseURL)
		fmt.Printf("      API: %s\n", provider.API)
		fmt.Printf("      Models: %d\n", len(provider.Models))
	}

	return nil
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	key := args[0]
	value := args[1]

	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	parts := strings.Split(key, ".")
	if len(parts) < 2 {
		return fmt.Errorf("invalid config key format. Use 'provider.field' format")
	}

	provider := parts[0]
	field := parts[1]

	if cfg.Models.Providers == nil {
		cfg.Models.Providers = make(map[string]config.ModelProviderConfig)
	}

	p, ok := cfg.Models.Providers[provider]
	if !ok {
		p = config.ModelProviderConfig{}
	}

	switch field {
	case "apiKey":
		p.APIKey = value
	case "baseUrl":
		p.BaseURL = value
	default:
		return fmt.Errorf("unknown config field: %s", field)
	}

	cfg.Models.Providers[provider] = p

	if err := config.SaveConfig(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Set %s = %s\n", key, value)
	return nil
}
