package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type FileReadInput struct {
	Path    string `json:"path"`
	Offset  int    `json:"offset,omitempty"`
	Limit   int    `json:"limit,omitempty"`
	Encoding string `json:"encoding,omitempty"`
}

type FileReadOutput struct {
	Content   string `json:"content"`
	Path      string `json:"path"`
	Size      int64  `json:"size"`
	Lines     int    `json:"lines"`
	Truncated bool   `json:"truncated,omitempty"`
}

func FileRead(ctx context.Context, input map[string]any) (string, error) {
	var params FileReadInput
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

	info, err := os.Stat(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to stat file: %w", err)
	}

	if info.IsDir() {
		return "", fmt.Errorf("path is a directory, not a file")
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	text := string(content)
	lines := strings.Split(text, "\n")

	offset := params.Offset
	if offset < 0 {
		offset = 0
	}

	limit := params.Limit
	if limit <= 0 {
		limit = len(lines)
	}

	truncated := false
	if offset > 0 || limit < len(lines) {
		truncated = true
	}

	end := offset + limit
	if end > len(lines) {
		end = len(lines)
	}

	var resultLines []string
	if offset < len(lines) {
		resultLines = lines[offset:end]
	} else {
		resultLines = []string{}
	}

	result := strings.Join(resultLines, "\n")

	output := FileReadOutput{
		Content:   result,
		Path:      absPath,
		Size:      info.Size(),
		Lines:     len(lines),
		Truncated: truncated,
	}

	outputJSON, _ := json.MarshalIndent(output, "", "  ")
	return string(outputJSON), nil
}

type FileWriteInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Mode    string `json:"mode,omitempty"`
}

type FileWriteOutput struct {
	Path     string `json:"path"`
	Size     int    `json:"size"`
	Created  bool   `json:"created"`
	Modified bool   `json:"modified"`
}

func FileWrite(ctx context.Context, input map[string]any) (string, error) {
	var params FileWriteInput
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

	_, err = os.Stat(absPath)
	exists := err == nil

	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	mode := os.FileMode(0644)
	if params.Mode == "executable" {
		mode = 0755
	}

	if err := os.WriteFile(absPath, []byte(params.Content), mode); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	output := FileWriteOutput{
		Path:     absPath,
		Size:     len(params.Content),
		Created:  !exists,
		Modified: exists,
	}

	outputJSON, _ := json.MarshalIndent(output, "", "  ")
	return string(outputJSON), nil
}

type FileListInput struct {
	Path     string `json:"path"`
	Pattern  string `json:"pattern,omitempty"`
	Recursive bool   `json:"recursive,omitempty"`
}

type FileListItem struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
	Size  int64  `json:"size,omitempty"`
}

type FileListOutput struct {
	Path  string         `json:"path"`
	Items []FileListItem `json:"items"`
	Count int            `json:"count"`
}

func FileList(ctx context.Context, input map[string]any) (string, error) {
	var params FileListInput
	data, _ := json.Marshal(input)
	if err := json.Unmarshal(data, &params); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}

	if params.Path == "" {
		params.Path = "."
	}

	absPath, err := filepath.Abs(params.Path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to stat path: %w", err)
	}

	if !info.IsDir() {
		return "", fmt.Errorf("path is not a directory")
	}

	pattern := params.Pattern
	if pattern == "" {
		pattern = "*"
	}

	var items []FileListItem

	if params.Recursive {
		err = filepath.WalkDir(absPath, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			matched, _ := filepath.Match(pattern, d.Name())
			if matched || d.IsDir() {
				item := FileListItem{
					Name:  d.Name(),
					Path:  path,
					IsDir: d.IsDir(),
				}
				if !d.IsDir() {
					if info, err := d.Info(); err == nil {
						item.Size = info.Size()
					}
				}
				items = append(items, item)
			}
			return nil
		})
	} else {
		entries, err := os.ReadDir(absPath)
		if err != nil {
			return "", fmt.Errorf("failed to read directory: %w", err)
		}

		for _, entry := range entries {
			matched, _ := filepath.Match(pattern, entry.Name())
			if matched {
				item := FileListItem{
					Name:  entry.Name(),
					Path:  filepath.Join(absPath, entry.Name()),
					IsDir: entry.IsDir(),
				}
				if !entry.IsDir() {
					if info, err := entry.Info(); err == nil {
						item.Size = info.Size()
					}
				}
				items = append(items, item)
			}
		}
	}

	output := FileListOutput{
		Path:  absPath,
		Items: items,
		Count: len(items),
	}

	outputJSON, _ := json.MarshalIndent(output, "", "  ")
	return string(outputJSON), nil
}

type FileSearchInput struct {
	Path    string `json:"path"`
	Pattern string `json:"pattern"`
	Type    string `json:"type,omitempty"`
}

type FileSearchResult struct {
	Path    string `json:"path"`
	Line    int    `json:"line,omitempty"`
	Content string `json:"content,omitempty"`
}

type FileSearchOutput struct {
	Path    string            `json:"path"`
	Pattern string            `json:"pattern"`
	Results []FileSearchResult `json:"results"`
	Count   int               `json:"count"`
}

func FileSearch(ctx context.Context, input map[string]any) (string, error) {
	var params FileSearchInput
	data, _ := json.Marshal(input)
	if err := json.Unmarshal(data, &params); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}

	if params.Path == "" {
		params.Path = "."
	}

	if params.Pattern == "" {
		return "", fmt.Errorf("pattern is required")
	}

	absPath, err := filepath.Abs(params.Path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path: %w", err)
	}

	var results []FileSearchResult

	err = filepath.WalkDir(absPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if d.IsDir() {
			return nil
		}

		if params.Type == "filename" {
			matched, _ := filepath.Match(params.Pattern, d.Name())
			if matched {
				results = append(results, FileSearchResult{Path: path})
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		binaryExts := map[string]bool{
			".exe": true, ".dll": true, ".so": true, ".dylib": true,
			".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".ico": true,
			".zip": true, ".tar": true, ".gz": true, ".rar": true,
			".pdf": true, ".doc": true, ".docx": true,
		}
		if binaryExts[ext] {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		text := string(content)
		lines := strings.Split(text, "\n")

		for i, line := range lines {
			if strings.Contains(line, params.Pattern) {
				results = append(results, FileSearchResult{
					Path:    path,
					Line:    i + 1,
					Content: strings.TrimSpace(line),
				})
			}
		}

		return nil
	})

	if err != nil {
		return "", fmt.Errorf("search failed: %w", err)
	}

	output := FileSearchOutput{
		Path:    absPath,
		Pattern: params.Pattern,
		Results: results,
		Count:   len(results),
	}

	outputJSON, _ := json.MarshalIndent(output, "", "  ")
	return string(outputJSON), nil
}

type FileDeleteInput struct {
	Path     string `json:"path"`
	Recursive bool   `json:"recursive,omitempty"`
}

type FileDeleteOutput struct {
	Path    string `json:"path"`
	Deleted bool   `json:"deleted"`
}

func FileDelete(ctx context.Context, input map[string]any) (string, error) {
	var params FileDeleteInput
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

	info, err := os.Stat(absPath)
	if err != nil {
		return "", fmt.Errorf("path does not exist: %w", err)
	}

	if info.IsDir() {
		if params.Recursive {
			err = os.RemoveAll(absPath)
		} else {
			err = os.Remove(absPath)
		}
	} else {
		err = os.Remove(absPath)
	}

	if err != nil {
		return "", fmt.Errorf("failed to delete: %w", err)
	}

	output := FileDeleteOutput{
		Path:    absPath,
		Deleted: true,
	}

	outputJSON, _ := json.MarshalIndent(output, "", "  ")
	return string(outputJSON), nil
}
