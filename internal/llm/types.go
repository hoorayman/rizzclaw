package llm

type MessageRole string

const (
	RoleSystem    MessageRole = "system"
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleTool      MessageRole = "tool"
)

type ContentBlock struct {
	Type      string    `json:"type"`
	Text      string    `json:"text,omitempty"`
	Thinking  string    `json:"thinking,omitempty"`
	ID        string    `json:"id,omitempty"`
	Name      string    `json:"name,omitempty"`
	Input     any       `json:"input,omitempty"`
	ToolUseID string    `json:"tool_use_id,omitempty"`
	Content   string    `json:"content,omitempty"`
	IsError   bool      `json:"is_error,omitempty"`
	ImageURL  *ImageURL `json:"image_url,omitempty"`
}

type ImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

type ToolUse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Input any    `json:"input"`
}

type ToolResult struct {
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error,omitempty"`
}

type Message struct {
	Role    MessageRole    `json:"role"`
	Content []ContentBlock `json:"content"`
}

type ToolParameterProperty struct {
	Type        string                           `json:"type"`
	Description string                           `json:"description,omitempty"`
	Enum        []string                         `json:"enum,omitempty"`
	Items       *ToolParameterProperty           `json:"items,omitempty"`
	Properties  map[string]ToolParameterProperty `json:"properties,omitempty"`
}

type InputSchema struct {
	Type       string                           `json:"type"`
	Properties map[string]ToolParameterProperty `json:"properties,omitempty"`
	Required   []string                         `json:"required,omitempty"`
}

type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	InputSchema InputSchema `json:"input_schema"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	CacheRead    int `json:"cache_read,omitempty"`
	CacheWrite   int `json:"cache_write,omitempty"`
}

type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
	System      string    `json:"system,omitempty"`
	Tools       []Tool    `json:"tools,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
}

type ChatResponse struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"`
	Role         MessageRole    `json:"role"`
	Content      []ContentBlock `json:"content"`
	Model        string         `json:"model"`
	StopReason   string         `json:"stop_reason,omitempty"`
	StopSequence string         `json:"stop_sequence,omitempty"`
	Usage        Usage          `json:"usage"`
}

type StreamEvent struct {
	Type         string        `json:"type"`
	Index        int           `json:"index,omitempty"`
	Delta        *StreamDelta  `json:"delta,omitempty"`
	ContentBlock *ContentBlock `json:"content_block,omitempty"`
	Message      *ChatResponse `json:"message,omitempty"`
	Usage        *Usage        `json:"usage,omitempty"`
}

type StreamDelta struct {
	Type       string `json:"type,omitempty"`
	Text       string `json:"text,omitempty"`
	Thinking   string `json:"thinking,omitempty"`
	StopReason string `json:"stop_reason,omitempty"`
}

func NewTool(name, description string, schema InputSchema) Tool {
	return Tool{
		Name:        name,
		Description: description,
		InputSchema: schema,
	}
}

func NewToolResultBlock(toolUseID, content string, isError bool) ContentBlock {
	return ContentBlock{
		Type:      "tool_result",
		ToolUseID: toolUseID,
		Content:   content,
		IsError:   isError,
	}
}
