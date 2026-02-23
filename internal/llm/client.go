package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	DefaultMinimaxBaseURL = "https://api.minimax.io/anthropic"
	DefaultTimeout        = 120 * time.Second
)

type Client struct {
	BaseURL      string
	APIKey       string
	HTTPClient   *http.Client
	Headers      map[string]string
	ToolExecutor func(ctx context.Context, name string, input map[string]any) (string, error)
	Debug        bool
}

type ClientOption func(*Client)

func WithBaseURL(url string) ClientOption {
	return func(c *Client) {
		c.BaseURL = url
	}
}

func WithAPIKey(key string) ClientOption {
	return func(c *Client) {
		c.APIKey = key
	}
}

func WithTimeout(timeout time.Duration) ClientOption {
	return func(c *Client) {
		c.HTTPClient.Timeout = timeout
	}
}

func WithHeaders(headers map[string]string) ClientOption {
	return func(c *Client) {
		for k, v := range c.Headers {
			c.Headers[k] = v
		}
	}
}

func WithToolExecutor(executor func(ctx context.Context, name string, input map[string]any) (string, error)) ClientOption {
	return func(c *Client) {
		c.ToolExecutor = executor
	}
}

func WithDebug(debug bool) ClientOption {
	return func(c *Client) {
		c.Debug = debug
	}
}

func NewClient(opts ...ClientOption) *Client {
	c := &Client{
		BaseURL: DefaultMinimaxBaseURL,
		HTTPClient: &http.Client{
			Timeout: DefaultTimeout,
		},
		Headers: make(map[string]string),
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

func (c *Client) doRequest(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var reqBody io.Reader
	var jsonData []byte
	if body != nil {
		var err error
		jsonData, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(jsonData)
	}

	if c.Debug {
		fmt.Printf("Request JSON: %s\n", string(jsonData))
	}

	url := fmt.Sprintf("%s%s", c.BaseURL, path)
	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.APIKey))
	req.Header.Set("anthropic-version", "2023-06-01")

	for k, v := range c.Headers {
		req.Header.Set(k, v)
	}

	return c.HTTPClient.Do(req)
}

type ChatResponseHandler func(response *ChatResponse) error

type StreamEventHandler func(event *StreamEvent) error

func (c *Client) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	req.Stream = false

	resp, err := c.doRequest(ctx, http.MethodPost, "/v1/messages", req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var response ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &response, nil
}

func (c *Client) ChatStream(ctx context.Context, req *ChatRequest, handler StreamEventHandler) error {
	req.Stream = true

	resp, err := c.doRequest(ctx, http.MethodPost, "/v1/messages", req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "" || data == "[DONE]" {
			continue
		}

		var event StreamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		if err := handler(&event); err != nil {
			return fmt.Errorf("event handler error: %w", err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scanner error: %w", err)
	}

	return nil
}

type ContentBlockDelta struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	Thinking    string `json:"thinking,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
	StopReason  string `json:"stop_reason,omitempty"`
}

type StreamEventEx struct {
	Type         string             `json:"type"`
	Index        int                `json:"index,omitempty"`
	Delta        *ContentBlockDelta `json:"delta,omitempty"`
	ContentBlock *ContentBlock      `json:"content_block,omitempty"`
	Message      *ChatResponse      `json:"message,omitempty"`
	Usage        *Usage             `json:"usage,omitempty"`
}

func (c *Client) ChatWithTools(ctx context.Context, req *ChatRequest, maxIterations int, handler StreamEventHandler) (*ChatResponse, error) {
	messages := make([]Message, len(req.Messages))
	copy(messages, req.Messages)

	for i := 0; i < maxIterations; i++ {
		req.Messages = messages

		var response ChatResponse
		var contentBlocks []ContentBlock
		var toolUseJSON strings.Builder

		err := c.ChatStreamRaw(ctx, req, func(event *StreamEventEx) error {
			if handler != nil {
				evt := &StreamEvent{
					Type:         event.Type,
					Index:        event.Index,
					ContentBlock: event.ContentBlock,
					Message:      event.Message,
					Usage:        event.Usage,
				}
				if event.Delta != nil {
					evt.Delta = &StreamDelta{
						Type:       event.Delta.Type,
						Text:       event.Delta.Text,
						Thinking:   event.Delta.Thinking,
						StopReason: event.Delta.StopReason,
					}
				}
				if err := handler(evt); err != nil {
					return err
				}
			}

			switch event.Type {
			case "content_block_start":
				if event.ContentBlock != nil {
					contentBlocks = append(contentBlocks, *event.ContentBlock)
					if event.ContentBlock.Type == "tool_use" {
						toolUseJSON.Reset()
					}
				}
			case "content_block_delta":
				if event.Delta != nil && len(contentBlocks) > event.Index {
					block := &contentBlocks[event.Index]
					switch event.Delta.Type {
					case "text_delta":
						if event.Delta.Text != "" {
							block.Text += event.Delta.Text
						}
					case "thinking_delta":
						if event.Delta.Thinking != "" {
							block.Thinking += event.Delta.Thinking
						}
					case "input_json_delta":
						if event.Delta.PartialJSON != "" {
							toolUseJSON.WriteString(event.Delta.PartialJSON)
						}
					}
				}
			case "content_block_stop":
				if len(contentBlocks) > event.Index {
					block := &contentBlocks[event.Index]
					if block.Type == "tool_use" {
						jsonStr := toolUseJSON.String()
						if jsonStr != "" {
							var input map[string]any
							if err := json.Unmarshal([]byte(jsonStr), &input); err == nil {
								block.Input = input
							} else {
								block.Input = jsonStr
							}
						}
					}
				}
			case "message_start":
				if event.Message != nil {
					response = *event.Message
				}
			case "message_delta":
				if event.Usage != nil {
					response.Usage = *event.Usage
				}
				if event.Delta != nil && event.Delta.StopReason != "" {
					response.StopReason = event.Delta.StopReason
				}
			}
			return nil
		})

		if err != nil {
			return nil, err
		}

		response.Content = contentBlocks

		if response.StopReason != "tool_use" {
			return &response, nil
		}

		messages = append(messages, Message{
			Role:    RoleAssistant,
			Content: contentBlocks,
		})

		for _, block := range contentBlocks {
			if block.Type == "tool_use" {
				var toolResultContent string
				var toolIsError bool

				if c.ToolExecutor != nil {
					var input map[string]any
					switch v := block.Input.(type) {
					case string:
						json.Unmarshal([]byte(v), &input)
					case map[string]any:
						input = v
					}

					result, err := c.ToolExecutor(ctx, block.Name, input)
					if err != nil {
						toolResultContent = fmt.Sprintf("Error: %v", err)
						toolIsError = true
					} else {
						toolResultContent = result
					}
				} else {
					toolResultContent = fmt.Sprintf("Tool %s executed", block.Name)
				}

				messages = append(messages, Message{
					Role: RoleUser,
					Content: []ContentBlock{
						NewToolResultBlock(block.ID, toolResultContent, toolIsError),
					},
				})
			}
		}
	}

	return nil, fmt.Errorf("max tool iterations reached")
}

func (c *Client) ChatStreamRaw(ctx context.Context, req *ChatRequest, handler func(event *StreamEventEx) error) error {
	req.Stream = true

	resp, err := c.doRequest(ctx, http.MethodPost, "/v1/messages", req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "" || data == "[DONE]" {
			continue
		}

		if c.Debug {
			fmt.Printf("Stream Event: %s\n", data)
		}

		var event StreamEventEx
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		if err := handler(&event); err != nil {
			return fmt.Errorf("event handler error: %w", err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scanner error: %w", err)
	}

	return nil
}
