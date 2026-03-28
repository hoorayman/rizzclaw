package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/hoorayman/rizzclaw/internal/skills"
	"github.com/spf13/cobra"
)

var skillsCmd = &cobra.Command{
	Use:   "skills",
	Short: "Manage skills",
	Long:  `Manage RizzClaw skills - list, enable, disable, and check skills.`,
}

var skillsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all skills",
	Long:  `List all available skills from all sources.`,
	RunE:  runSkillsList,
}

var skillsEnableCmd = &cobra.Command{
	Use:   "enable <skill-id>",
	Short: "Enable a skill",
	Args:  cobra.ExactArgs(1),
	RunE:  runSkillsEnable,
}

var skillsDisableCmd = &cobra.Command{
	Use:   "disable <skill-id>",
	Short: "Disable a skill",
	Args:  cobra.ExactArgs(1),
	RunE:  runSkillsDisable,
}

var skillsCheckCmd = &cobra.Command{
	Use:   "check [skill-id]",
	Short: "Check skill dependencies",
	Long:  `Check if skill dependencies are satisfied. If no skill-id is provided, checks all skills.`,
	RunE:  runSkillsCheck,
}

var skillsInfoCmd = &cobra.Command{
	Use:   "info <skill-id>",
	Short: "Show skill details",
	Args:  cobra.ExactArgs(1),
	RunE:  runSkillsInfo,
}

var skillsReloadCmd = &cobra.Command{
	Use:   "reload",
	Short: "Reload skills from disk",
	Long:  `Reload all skills from disk, including from ~/.agent-skills and project directories.`,
	RunE:  runSkillsReload,
}

var (
	flagSkillsAll      bool
	flagSkillsEnabled  bool
	flagSkillsEligible bool
)

func init() {
	skillsListCmd.Flags().BoolVarP(&flagSkillsAll, "all", "a", false, "Show all skills including disabled")
	skillsListCmd.Flags().BoolVarP(&flagSkillsEnabled, "enabled", "e", false, "Show only enabled skills")
	skillsListCmd.Flags().BoolVarP(&flagSkillsEligible, "eligible", "l", false, "Show only eligible skills (enabled + dependencies met)")

	skillsCmd.AddCommand(skillsListCmd)
	skillsCmd.AddCommand(skillsEnableCmd)
	skillsCmd.AddCommand(skillsDisableCmd)
	skillsCmd.AddCommand(skillsCheckCmd)
	skillsCmd.AddCommand(skillsInfoCmd)
	skillsCmd.AddCommand(skillsReloadCmd)

	rootCmd.AddCommand(skillsCmd)
}

func runSkillsList(cmd *cobra.Command, args []string) error {
	if err := skills.LoadAllSkillsFromDisk(); err != nil {
		fmt.Printf("Warning: failed to load skills from disk: %v\n", err)
	}

	registry := skills.GetSkillRegistry()
	allSkills := registry.List()

	var filtered []*skills.Skill
	switch {
	case flagSkillsEligible:
		filtered = skills.GetEligibleSkills()
	case flagSkillsEnabled:
		filtered = registry.ListEnabled()
	case flagSkillsAll:
		filtered = allSkills
	default:
		filtered = allSkills
	}

	if len(filtered) == 0 {
		fmt.Println("No skills found.")
		fmt.Println()
		fmt.Println("Skills are loaded from:")
		fmt.Println("  - ~/.rizzclaw/skills/")
		fmt.Println("  - ~/.agents/skills/        (npx skills add global)")
		fmt.Println("  - ./.rizzclaw/skills/")
		fmt.Println("  - ./.agents/skills/        (npx skills add local)")
		fmt.Println()
		fmt.Println("Install skills using: npx skills add <owner/repo>")
		return nil
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].ID < filtered[j].ID
	})

	fmt.Println("Skills:")
	fmt.Println()

	for _, skill := range filtered {
		status := "disabled"
		eligible := skill.IsEligible()

		if skill.Enabled {
			if eligible {
				status = "enabled ✓"
			} else {
				status = "enabled (deps missing)"
			}
		}

		emoji := ""
		if skill.Emoji != "" {
			emoji = skill.Emoji + " "
		}

		fmt.Printf("  %s%s [%s]\n", emoji, skill.ID, status)
		if skill.Description != "" {
			fmt.Printf("    %s\n", skill.Description)
		}
		if skill.SourcePath != "" {
			fmt.Printf("    Source: %s\n", skill.SourcePath)
		}
		if len(skill.Requires.Bins) > 0 || len(skill.Requires.Env) > 0 {
			fmt.Printf("    Requires: ")
			var reqs []string
			for _, b := range skill.Requires.Bins {
				reqs = append(reqs, "bin:"+b)
			}
			for _, e := range skill.Requires.Env {
				reqs = append(reqs, "env:"+e)
			}
			fmt.Printf("%s\n", strings.Join(reqs, ", "))
		}
		fmt.Println()
	}

	fmt.Printf("Total: %d skills\n", len(filtered))
	return nil
}

func runSkillsEnable(cmd *cobra.Command, args []string) error {
	skillID := args[0]

	if err := skills.LoadAllSkillsFromDisk(); err != nil {
		fmt.Printf("Warning: failed to load skills from disk: %v\n", err)
	}

	registry := skills.GetSkillRegistry()
	skill := registry.Get(skillID)
	if skill == nil {
		return fmt.Errorf("skill not found: %s", skillID)
	}

	if err := registry.Enable(skillID); err != nil {
		return err
	}

	fmt.Printf("Skill '%s' enabled.\n", skillID)

	if missing := skill.CheckDependencies(); len(missing) > 0 {
		fmt.Println()
		fmt.Println("Warning: Some dependencies are missing:")
		for _, m := range missing {
			fmt.Printf("  - %s\n", m)
		}
	}

	return nil
}

func runSkillsDisable(cmd *cobra.Command, args []string) error {
	skillID := args[0]

	registry := skills.GetSkillRegistry()
	if registry.Get(skillID) == nil {
		return fmt.Errorf("skill not found: %s", skillID)
	}

	if err := registry.Disable(skillID); err != nil {
		return err
	}

	fmt.Printf("Skill '%s' disabled.\n", skillID)
	return nil
}

func runSkillsCheck(cmd *cobra.Command, args []string) error {
	if err := skills.LoadAllSkillsFromDisk(); err != nil {
		fmt.Printf("Warning: failed to load skills from disk: %v\n", err)
	}

	registry := skills.GetSkillRegistry()

	if len(args) > 0 {
		skillID := args[0]
		skill := registry.Get(skillID)
		if skill == nil {
			return fmt.Errorf("skill not found: %s", skillID)
		}

		fmt.Printf("Checking skill: %s\n", skillID)
		fmt.Println()

		missing := skill.CheckDependencies()
		if len(missing) == 0 {
			fmt.Println("✓ All dependencies are satisfied.")
		} else {
			fmt.Println("✗ Missing dependencies:")
			for _, m := range missing {
				fmt.Printf("  - %s\n", m)
			}
		}
		return nil
	}

	allSkills := registry.ListEnabled()
	if len(allSkills) == 0 {
		fmt.Println("No enabled skills to check.")
		return nil
	}

	fmt.Println("Checking all enabled skills:")
	fmt.Println()

	for _, skill := range allSkills {
		missing := skill.CheckDependencies()
		if len(missing) == 0 {
			fmt.Printf("  ✓ %s\n", skill.ID)
		} else {
			fmt.Printf("  ✗ %s (missing: %s)\n", skill.ID, strings.Join(missing, ", "))
		}
	}

	return nil
}

func runSkillsInfo(cmd *cobra.Command, args []string) error {
	skillID := args[0]

	if err := skills.LoadAllSkillsFromDisk(); err != nil {
		fmt.Printf("Warning: failed to load skills from disk: %v\n", err)
	}

	registry := skills.GetSkillRegistry()
	skill := registry.Get(skillID)
	if skill == nil {
		return fmt.Errorf("skill not found: %s", skillID)
	}

	emoji := ""
	if skill.Emoji != "" {
		emoji = skill.Emoji + " "
	}

	fmt.Printf("%s%s\n", emoji, skill.ID)
	fmt.Println(strings.Repeat("-", len(skill.ID)+len(emoji)))

	if skill.Name != "" && skill.Name != skill.ID {
		fmt.Printf("Name: %s\n", skill.Name)
	}
	if skill.Version != "" {
		fmt.Printf("Version: %s\n", skill.Version)
	}
	if skill.Author != "" {
		fmt.Printf("Author: %s\n", skill.Author)
	}
	if skill.Description != "" {
		fmt.Printf("Description: %s\n", skill.Description)
	}
	if skill.Homepage != "" {
		fmt.Printf("Homepage: %s\n", skill.Homepage)
	}
	if skill.SourcePath != "" {
		fmt.Printf("Source: %s\n", skill.SourcePath)
	}

	fmt.Printf("Enabled: %v\n", skill.Enabled)
	fmt.Printf("Eligible: %v\n", skill.IsEligible())

	if len(skill.OS) > 0 {
		fmt.Printf("Supported OS: %s\n", strings.Join(skill.OS, ", "))
	}

	if len(skill.Tags) > 0 {
		fmt.Printf("Tags: %s\n", strings.Join(skill.Tags, ", "))
	}

	if len(skill.Tools) > 0 {
		fmt.Printf("Tools: %s\n", strings.Join(skill.Tools, ", "))
	}

	if len(skill.Requires.Bins) > 0 || len(skill.Requires.AnyBins) > 0 || len(skill.Requires.Env) > 0 || len(skill.Requires.Config) > 0 {
		fmt.Println("\nDependencies:")
		for _, b := range skill.Requires.Bins {
			found := "✓"
			if _, err := os.Stat(b); os.IsNotExist(err) {
				found = "✗"
			}
			fmt.Printf("  bin: %s [%s]\n", b, found)
		}
		if len(skill.Requires.AnyBins) > 0 {
			fmt.Printf("  any of: %s\n", strings.Join(skill.Requires.AnyBins, " | "))
		}
		for _, e := range skill.Requires.Env {
			found := "✓"
			if os.Getenv(e) == "" {
				found = "✗"
			}
			fmt.Printf("  env: %s [%s]\n", e, found)
		}
	}

	if len(skill.Install) > 0 {
		fmt.Println("\nInstallation:")
		for _, inst := range skill.Install {
			fmt.Printf("  %s: %s", inst.Kind, inst.Formula)
			if inst.DocURL != "" {
				fmt.Printf(" (%s)", inst.DocURL)
			}
			fmt.Println()
		}
	}

	if skill.When != "" {
		fmt.Printf("\nWhen to use: %s\n", skill.When)
	}

	if skill.Prompt != "" {
		fmt.Println("\nPrompt Preview:")
		lines := strings.Split(skill.Prompt, "\n")
		maxLines := 20
		if len(lines) > maxLines {
			for i := 0; i < maxLines; i++ {
				fmt.Printf("  %s\n", lines[i])
			}
			fmt.Printf("  ... (%d more lines)\n", len(lines)-maxLines)
		} else {
			for _, line := range lines {
				fmt.Printf("  %s\n", line)
			}
		}
	}

	return nil
}

func runSkillsReload(cmd *cobra.Command, args []string) error {
	loader := skills.GetSkillLoader()
	loader.ClearCache()

	if err := skills.LoadAllSkillsFromDisk(); err != nil {
		return fmt.Errorf("failed to reload skills: %w", err)
	}

	registry := skills.GetSkillRegistry()
	count := len(registry.List())

	fmt.Printf("Reloaded %d skills from disk.\n", count)
	return nil
}
