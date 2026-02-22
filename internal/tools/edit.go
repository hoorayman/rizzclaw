package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type EditInput struct {
	Path        string `json:"path"`
	OldString   string `json:"oldString"`
	NewString   string `json:"newString"`
	ReplaceAll  bool   `json:"replaceAll,omitempty"`
}

type EditOutput struct {
	Path       string `json:"path"`
	Success    bool   `json:"success"`
	Replacements int  `json:"replacements"`
	Message    string `json:"message"`
}

func Edit(ctx context.Context, input map[string]any) (string, error) {
	var params EditInput
	data, _ := json.Marshal(input)
	if err := json.Unmarshal(data, &params); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}

	if params.Path == "" {
		return "", fmt.Errorf("path is required")
	}

	if params.OldString == "" {
		return "", fmt.Errorf("oldString is required")
	}

	absPath, err := filepath.Abs(params.Path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path: %w", err)
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	text := string(content)

	if !strings.Contains(text, params.OldString) {
		output := EditOutput{
			Path:       absPath,
			Success:    false,
			Replacements: 0,
			Message:    "oldString not found in file",
		}
		outputJSON, _ := json.MarshalIndent(output, "", "  ")
		return string(outputJSON), nil
	}

	var newText string
	replacements := 0

	if params.ReplaceAll {
		newText = strings.ReplaceAll(text, params.OldString, params.NewString)
		replacements = strings.Count(text, params.OldString)
	} else {
		newText = strings.Replace(text, params.OldString, params.NewString, 1)
		replacements = 1
	}

	if err := os.WriteFile(absPath, []byte(newText), 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	output := EditOutput{
		Path:         absPath,
		Success:      true,
		Replacements: replacements,
		Message:      fmt.Sprintf("Successfully replaced %d occurrence(s)", replacements),
	}

	outputJSON, _ := json.MarshalIndent(output, "", "  ")
	return string(outputJSON), nil
}

type EditRegexInput struct {
	Path      string `json:"path"`
	Pattern   string `json:"pattern"`
	Replace   string `json:"replace"`
	Global    bool   `json:"global,omitempty"`
}

func EditRegex(ctx context.Context, input map[string]any) (string, error) {
	var params EditRegexInput
	data, _ := json.Marshal(input)
	if err := json.Unmarshal(data, &params); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}

	if params.Path == "" {
		return "", fmt.Errorf("path is required")
	}

	if params.Pattern == "" {
		return "", fmt.Errorf("pattern is required")
	}

	absPath, err := filepath.Abs(params.Path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path: %w", err)
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	re, err := regexp.Compile(params.Pattern)
	if err != nil {
		return "", fmt.Errorf("invalid regex pattern: %w", err)
	}

	text := string(content)
	replacements := 0

	var newText string
	if params.Global {
		newText = re.ReplaceAllStringFunc(text, func(match string) string {
			replacements++
			return re.ReplaceAllString(match, params.Replace)
		})
	} else {
		if re.MatchString(text) {
			newText = re.ReplaceAllString(text, params.Replace)
			replacements = 1
		} else {
			newText = text
		}
	}

	if replacements == 0 {
		output := EditOutput{
			Path:       absPath,
			Success:    false,
			Replacements: 0,
			Message:    "pattern not found in file",
		}
		outputJSON, _ := json.MarshalIndent(output, "", "  ")
		return string(outputJSON), nil
	}

	if err := os.WriteFile(absPath, []byte(newText), 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	output := EditOutput{
		Path:         absPath,
		Success:      true,
		Replacements: replacements,
		Message:      fmt.Sprintf("Successfully replaced %d occurrence(s)", replacements),
	}

	outputJSON, _ := json.MarshalIndent(output, "", "  ")
	return string(outputJSON), nil
}

type PatchInput struct {
	Path    string `json:"path"`
	Patch   string `json:"patch"`
}

type PatchHunk struct {
	StartLine int
	Lines     []string
}

func ApplyPatch(ctx context.Context, input map[string]any) (string, error) {
	var params PatchInput
	data, _ := json.Marshal(input)
	if err := json.Unmarshal(data, &params); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}

	if params.Path == "" {
		return "", fmt.Errorf("path is required")
	}

	if params.Patch == "" {
		return "", fmt.Errorf("patch is required")
	}

	absPath, err := filepath.Abs(params.Path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path: %w", err)
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	patchLines := strings.Split(params.Patch, "\n")

	applied := 0
	i := 0
	for i < len(patchLines) {
		line := patchLines[i]

		if strings.HasPrefix(line, "@@") {
			re := regexp.MustCompile(`@@ -(\d+),?\d* \+(\d+),?\d* @@`)
			matches := re.FindStringSubmatch(line)
			if len(matches) >= 3 {
				startLine := 0
				fmt.Sscanf(matches[2], "%d", &startLine)
				if startLine > 0 {
					startLine--
				}

				i++
				currentLine := startLine

				for i < len(patchLines) && !strings.HasPrefix(patchLines[i], "@@") {
					patchLine := patchLines[i]

					if strings.HasPrefix(patchLine, "+") {
						newLine := strings.TrimPrefix(patchLine, "+")
						if currentLine >= len(lines) {
							lines = append(lines, newLine)
						} else {
							lines = append(lines[:currentLine], append([]string{newLine}, lines[currentLine:]...)...)
						}
						applied++
						currentLine++
					} else if strings.HasPrefix(patchLine, "-") {
						if currentLine < len(lines) {
							lines = append(lines[:currentLine], lines[currentLine+1:]...)
						}
						applied++
					} else if strings.HasPrefix(patchLine, " ") {
						currentLine++
					}
					i++
				}
				continue
			}
		}
		i++
	}

	if applied == 0 {
		output := EditOutput{
			Path:       absPath,
			Success:    false,
			Replacements: 0,
			Message:    "no hunks applied from patch",
		}
		outputJSON, _ := json.MarshalIndent(output, "", "  ")
		return string(outputJSON), nil
	}

	newContent := strings.Join(lines, "\n")
	if err := os.WriteFile(absPath, []byte(newContent), 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	output := EditOutput{
		Path:         absPath,
		Success:      true,
		Replacements: applied,
		Message:      fmt.Sprintf("Applied %d patch line(s)", applied),
	}

	outputJSON, _ := json.MarshalIndent(output, "", "  ")
	return string(outputJSON), nil
}

type CreateInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func Create(ctx context.Context, input map[string]any) (string, error) {
	var params CreateInput
	data, _ := json.Marshal(input)
	if err := json.Unmarshal(data, &params); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}

	if params.Path == "" {
		return "", fmt.Errorf("path is required")
	}

	absPath, err := filepath.Abs(params.Path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path: %w", err)
	}

	if _, err := os.Stat(absPath); err == nil {
		return "", fmt.Errorf("file already exists: %s", absPath)
	}

	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(absPath, []byte(params.Content), 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	output := map[string]any{
		"path":    absPath,
		"created": true,
		"size":    len(params.Content),
		"message": fmt.Sprintf("Created file %s", absPath),
	}

	outputJSON, _ := json.MarshalIndent(output, "", "  ")
	return string(outputJSON), nil
}
