package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type MiniMaxEmbeddingProvider struct {
	apiKey  string
	baseURL string
	model   string
	client  *http.Client
}

type MiniMaxEmbeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type MiniMaxEmbeddingResponse struct {
	Object string `json:"object"`
	Data   []struct {
		Object    string    `json:"object"`
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Model string `json:"model"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

func NewMiniMaxEmbeddingProvider(apiKey, model string) *MiniMaxEmbeddingProvider {
	if model == "" {
		model = "text-embedding-3-small"
	}
	return &MiniMaxEmbeddingProvider{
		apiKey:  apiKey,
		baseURL: "https://api.minimaxi.com/v1",
		model:   model,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (p *MiniMaxEmbeddingProvider) ID() string {
	return "minimax"
}

func (p *MiniMaxEmbeddingProvider) Model() string {
	return p.model
}

func (p *MiniMaxEmbeddingProvider) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := p.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}
	return embeddings[0], nil
}

func (p *MiniMaxEmbeddingProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	reqBody := MiniMaxEmbeddingRequest{
		Model: p.model,
		Input: texts,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/embeddings", bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.apiKey))

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result MiniMaxEmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	embeddings := make([][]float32, len(result.Data))
	for i, d := range result.Data {
		embeddings[i] = d.Embedding
	}

	return embeddings, nil
}

type MockEmbeddingProvider struct {
	dimension int
}

func NewMockEmbeddingProvider(dimension int) *MockEmbeddingProvider {
	if dimension <= 0 {
		dimension = 384
	}
	return &MockEmbeddingProvider{dimension: dimension}
}

func (p *MockEmbeddingProvider) ID() string {
	return "mock"
}

func (p *MockEmbeddingProvider) Model() string {
	return "mock-embedding"
}

func (p *MockEmbeddingProvider) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := p.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}
	return embeddings[0], nil
}

func (p *MockEmbeddingProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	embeddings := make([][]float32, len(texts))
	for i := range texts {
		embedding := make([]float32, p.dimension)
		for j := range embedding {
			embedding[j] = float32(i+j) / float32(p.dimension)
		}
		embeddings[i] = embedding
	}
	return embeddings, nil
}
