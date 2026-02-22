package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/hoorayman/rizzclaw/internal/context"
	"github.com/spf13/cobra"
)

var contextCmd = &cobra.Command{
	Use:   "context",
	Short: "Manage context files",
	Long:  `Manage RizzClaw context files (AGENTS.md, MEMORY.md, SOUL.md, USER.md, IDENTITY.md)`,
}

var contextInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize context files",
	Long:  `Initialize all context files with default templates in the workspace directory.`,
	RunE:  runContextInit,
}

var contextShowCmd = &cobra.Command{
	Use:   "show [file]",
	Short: "Show context file content",
	Long:  `Show the content of a specific context file. If no file specified, shows all.`,
	RunE:  runContextShow,
}

var contextListCmd = &cobra.Command{
	Use:   "list",
	Short: "List context files",
	Long:  `List all context files and their status.`,
	RunE:  runContextList,
}

var contextEditCmd = &cobra.Command{
	Use:   "edit <file>",
	Short: "Edit a context file",
	Long:  `Open a context file in the default editor.`,
	RunE:  runContextEdit,
}

var contextPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Show workspace path",
	Long:  `Show the path to the workspace directory.`,
	RunE:  runContextPath,
}

var contextMemoryCmd = &cobra.Command{
	Use:   "memory <content>",
	Short: "Add a memory entry",
	Long:  `Add a new entry to MEMORY.md.`,
	RunE:  runContextMemory,
}

var flagWorkspaceDir string
var flagEvergreen bool

func init() {
	contextCmd.PersistentFlags().StringVarP(&flagWorkspaceDir, "dir", "d", "", "Workspace directory (default: ~/.rizzclaw/workspace)")
	contextMemoryCmd.Flags().BoolVarP(&flagEvergreen, "evergreen", "e", false, "Mark as evergreen memory (no time decay)")

	contextCmd.AddCommand(contextInitCmd)
	contextCmd.AddCommand(contextShowCmd)
	contextCmd.AddCommand(contextListCmd)
	contextCmd.AddCommand(contextEditCmd)
	contextCmd.AddCommand(contextPathCmd)
	contextCmd.AddCommand(contextMemoryCmd)

	rootCmd.AddCommand(contextCmd)
}

func getWorkspaceDir() string {
	if flagWorkspaceDir != "" {
		return flagWorkspaceDir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".rizzclaw", "workspace")
}

func runContextInit(cmd *cobra.Command, args []string) error {
	workspaceDir := getWorkspaceDir()
	
	fmt.Printf("Initializing workspace: %s\n\n", workspaceDir)
	
	if err := context.InitializeWorkspace(workspaceDir); err != nil {
		return fmt.Errorf("failed to initialize workspace: %w", err)
	}
	
	fmt.Println("Created context files:")
	for _, filename := range context.BootstrapFilenames {
		path := filepath.Join(workspaceDir, filename)
		fmt.Printf("  ✓ %s\n", path)
	}
	
	fmt.Println("\nWorkspace initialized successfully!")
	return nil
}

func runContextShow(cmd *cobra.Command, args []string) error {
	workspaceDir := getWorkspaceDir()
	mgr := context.NewManager(workspaceDir, nil)
	
	if len(args) > 0 {
		filename := args[0]
		cf := mgr.GetFile(filename)
		if cf == nil || cf.Content == "" {
			return fmt.Errorf("file not found or empty: %s", filename)
		}
		fmt.Printf("=== %s ===\n\n", filename)
		fmt.Println(cf.Content)
		return nil
	}
	
	for _, filename := range context.BootstrapFilenames {
		cf := mgr.GetFile(filename)
		fmt.Printf("=== %s ===\n\n", filename)
		if cf.Content != "" {
			fmt.Println(cf.Content)
		} else {
			fmt.Println("(empty or not found)")
		}
		fmt.Println()
	}
	
	return nil
}

func runContextList(cmd *cobra.Command, args []string) error {
	workspaceDir := getWorkspaceDir()
	
	fmt.Printf("Workspace: %s\n\n", workspaceDir)
	
	fmt.Println("Context Files:")
	fmt.Println()
	
	for _, filename := range context.BootstrapFilenames {
		path := filepath.Join(workspaceDir, filename)
		info, err := os.Stat(path)
		
		status := "✗ not found"
		size := ""
		
		if err == nil {
			status = "✓ exists"
			size = fmt.Sprintf("(%d bytes)", info.Size())
		}
		
		fmt.Printf("  %-15s %s %s\n", filename, status, size)
	}
	
	fmt.Println()
	
	sessionsDir := filepath.Join(filepath.Dir(workspaceDir), "sessions")
	if entries, err := os.ReadDir(sessionsDir); err == nil {
		fmt.Printf("Sessions: %d saved\n", len(entries))
	}
	
	memoryDB := filepath.Join(filepath.Dir(workspaceDir), "memory.db")
	if info, err := os.Stat(memoryDB); err == nil {
		fmt.Printf("Memory DB: %d bytes\n", info.Size())
	}
	
	return nil
}

func runContextEdit(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("please specify a file to edit")
	}
	
	filename := args[0]
	workspaceDir := getWorkspaceDir()
	path := filepath.Join(workspaceDir, filename)
	
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		editor = "notepad"
	}
	
	fmt.Printf("Opening %s with %s...\n", path, editor)
	
	return nil
}

func runContextPath(cmd *cobra.Command, args []string) error {
	workspaceDir := getWorkspaceDir()
	fmt.Println(workspaceDir)
	return nil
}

func runContextMemory(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("please provide memory content")
	}
	
	content := args[0]
	if len(args) > 1 {
		content = ""
		for _, arg := range args {
			if content != "" {
				content += " "
			}
			content += arg
		}
	}
	
	sessionMgr := context.GetSessionManager()
	
	if err := sessionMgr.SaveImportantMemory(content, flagEvergreen); err != nil {
		return fmt.Errorf("failed to save memory: %w", err)
	}
	
	marker := ""
	if flagEvergreen {
		marker = " [EVERGREEN]"
	}
	
	fmt.Printf("Memory saved%s: %s\n", marker, content)
	return nil
}
