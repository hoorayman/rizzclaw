package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type MemorySource string

const (
	SourceMemory   MemorySource = "memory"
	SourceSessions MemorySource = "sessions"
)

type SearchResult struct {
	Path      string       `json:"path"`
	StartLine int          `json:"startLine"`
	EndLine   int          `json:"endLine"`
	Score     float64      `json:"score"`
	Snippet   string       `json:"snippet"`
	Source    MemorySource `json:"source"`
	Citation  string       `json:"citation,omitempty"`
}

type Chunk struct {
	ID        string       `json:"id"`
	Path      string       `json:"path"`
	Source    MemorySource `json:"source"`
	StartLine int          `json:"startLine"`
	EndLine   int          `json:"endLine"`
	Hash      string       `json:"hash"`
	Model     string       `json:"model"`
	Text      string       `json:"text"`
	UpdatedAt time.Time    `json:"updatedAt"`
}

type EmbeddingProvider interface {
	ID() string
	Model() string
	EmbedQuery(ctx context.Context, text string) ([]float32, error)
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
}

type MemoryConfig struct {
	Enabled       bool   `json:"enabled"`
	Provider      string `json:"provider"`
	Model         string `json:"model"`
	DBPath        string `json:"dbPath"`
	Sources       []MemorySource `json:"sources"`
	MaxResults    int    `json:"maxResults"`
	MinScore      float64 `json:"minScore"`
	HybridEnabled bool   `json:"hybridEnabled"`
	VectorWeight  float64 `json:"vectorWeight"`
	TextWeight    float64 `json:"textWeight"`
}

type MemoryManager struct {
	db       *sql.DB
	provider EmbeddingProvider
	config   *MemoryConfig
	mu       sync.RWMutex
}

var defaultManager *MemoryManager
var managerOnce sync.Once

func GetMemoryManager() *MemoryManager {
	managerOnce.Do(func() {
		home, _ := os.UserHomeDir()
		dbPath := filepath.Join(home, ".rizzclaw", "memory.db")
		
		config := &MemoryConfig{
			Enabled:       true,
			Provider:      "minimax",
			Model:         "text-embedding-3-small",
			DBPath:        dbPath,
			Sources:       []MemorySource{SourceMemory, SourceSessions},
			MaxResults:    10,
			MinScore:      0.5,
			HybridEnabled: true,
			VectorWeight:  0.7,
			TextWeight:    0.3,
		}
		
		mgr, err := NewMemoryManager(config)
		if err != nil {
			fmt.Printf("Warning: failed to create memory manager: %v\n", err)
			return
		}
		defaultManager = mgr
	})
	return defaultManager
}

func NewMemoryManager(config *MemoryConfig) (*MemoryManager, error) {
	if config.DBPath == "" {
		home, _ := os.UserHomeDir()
		config.DBPath = filepath.Join(home, ".rizzclaw", "memory.db")
	}

	dir := filepath.Dir(config.DBPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create memory directory: %w", err)
	}

	db, err := sql.Open("sqlite3", config.DBPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := initSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return &MemoryManager{
		db:     db,
		config: config,
	}, nil
}

func initSchema(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS files (
		path TEXT PRIMARY KEY,
		source TEXT NOT NULL DEFAULT 'memory',
		hash TEXT NOT NULL,
		mtime INTEGER NOT NULL,
		size INTEGER NOT NULL
	);

	CREATE TABLE IF NOT EXISTS chunks (
		id TEXT PRIMARY KEY,
		path TEXT NOT NULL,
		source TEXT NOT NULL DEFAULT 'memory',
		start_line INTEGER NOT NULL,
		end_line INTEGER NOT NULL,
		hash TEXT NOT NULL,
		model TEXT NOT NULL,
		text TEXT NOT NULL,
		embedding BLOB,
		updated_at INTEGER NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_chunks_path ON chunks(path);
	CREATE INDEX IF NOT EXISTS idx_chunks_source ON chunks(source);
	CREATE INDEX IF NOT EXISTS idx_chunks_model ON chunks(model);

	CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5(
		text,
		id UNINDEXED,
		path UNINDEXED,
		source UNINDEXED,
		content='chunks',
		content_rowid='rowid'
	);

	CREATE TRIGGER IF NOT EXISTS chunks_ai AFTER INSERT ON chunks BEGIN
		INSERT INTO chunks_fts(rowid, text, id, path, source)
		VALUES (new.rowid, new.text, new.id, new.path, new.source);
	END;

	CREATE TRIGGER IF NOT EXISTS chunks_ad AFTER DELETE ON chunks BEGIN
		INSERT INTO chunks_fts(chunks_fts, rowid, text, id, path, source)
		VALUES('delete', old.rowid, old.text, old.id, old.path, old.source);
	END;

	CREATE TRIGGER IF NOT EXISTS chunks_au AFTER UPDATE ON chunks BEGIN
		INSERT INTO chunks_fts(chunks_fts, rowid, text, id, path, source)
		VALUES('delete', old.rowid, old.text, old.id, old.path, old.source);
		INSERT INTO chunks_fts(rowid, text, id, path, source)
		VALUES (new.rowid, new.text, new.id, new.path, new.source);
	END;
	`

	_, err := db.Exec(schema)
	return err
}

func (m *MemoryManager) SetProvider(provider EmbeddingProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.provider = provider
}

func (m *MemoryManager) Search(ctx context.Context, query string, opts ...SearchOption) ([]SearchResult, error) {
	options := &SearchOptions{
		MaxResults: m.config.MaxResults,
		MinScore:   m.config.MinScore,
	}
	for _, opt := range opts {
		opt(options)
	}

	if m.provider == nil {
		return m.searchKeyword(query, options)
	}

	return m.searchHybrid(ctx, query, options)
}

type SearchOptions struct {
	MaxResults int
	MinScore   float64
	SessionKey string
	Sources    []MemorySource
}

type SearchOption func(*SearchOptions)

func WithMaxResults(n int) SearchOption {
	return func(o *SearchOptions) { o.MaxResults = n }
}

func WithMinScore(score float64) SearchOption {
	return func(o *SearchOptions) { o.MinScore = score }
}

func WithSessionKey(key string) SearchOption {
	return func(o *SearchOptions) { o.SessionKey = key }
}

func WithSources(sources ...MemorySource) SearchOption {
	return func(o *SearchOptions) { o.Sources = sources }
}

func (m *MemoryManager) searchKeyword(query string, opts *SearchOptions) ([]SearchResult, error) {
	keywords := extractKeywords(query)
	if len(keywords) == 0 {
		keywords = []string{query}
	}

	var results []SearchResult
	for _, keyword := range keywords {
		rows, err := m.db.Query(`
			SELECT id, path, source, start_line, end_line, text
			FROM chunks_fts
			WHERE chunks_fts MATCH ?
			ORDER BY rank
			LIMIT ?
		`, keyword, opts.MaxResults)
		if err != nil {
			continue
		}
		defer rows.Close()

		for rows.Next() {
			var r SearchResult
			var text string
			if err := rows.Scan(&r.Path, &r.Path, &r.Source, &r.StartLine, &r.EndLine, &text); err != nil {
				continue
			}
			r.Snippet = truncateText(text, 200)
			r.Score = 0.5
			results = append(results, r)
		}
	}

	return deduplicateResults(results, opts.MaxResults), nil
}

func (m *MemoryManager) searchHybrid(ctx context.Context, query string, opts *SearchOptions) ([]SearchResult, error) {
	keywordResults, err := m.searchKeyword(query, opts)
	if err != nil {
		keywordResults = nil
	}

	vectorResults, err := m.searchVector(ctx, query, opts)
	if err != nil {
		vectorResults = nil
	}

	return m.mergeResults(keywordResults, vectorResults, opts)
}

func (m *MemoryManager) searchVector(ctx context.Context, query string, opts *SearchOptions) ([]SearchResult, error) {
	if m.provider == nil {
		return nil, fmt.Errorf("no embedding provider")
	}

	embedding, err := m.provider.EmbedQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}

	rows, err := m.db.Query(`
		SELECT id, path, source, start_line, end_line, text, embedding
		FROM chunks
		WHERE embedding IS NOT NULL
		ORDER BY ?
		LIMIT ?
	`, "", opts.MaxResults*2)
	if err != nil {
		return nil, fmt.Errorf("failed to query chunks: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var id, path, text string
		var source MemorySource
		var startLine, endLine int
		var embeddingBlob []byte

		if err := rows.Scan(&id, &path, &source, &startLine, &endLine, &text, &embeddingBlob); err != nil {
			continue
		}

		var chunkEmbedding []float32
		if err := json.Unmarshal(embeddingBlob, &chunkEmbedding); err != nil {
			continue
		}

		score := cosineSimilarity(embedding, chunkEmbedding)
		if score >= opts.MinScore {
			results = append(results, SearchResult{
				Path:      path,
				StartLine: startLine,
				EndLine:   endLine,
				Score:     score,
				Snippet:   truncateText(text, 200),
				Source:    source,
			})
		}
	}

	return results, nil
}

func (m *MemoryManager) mergeResults(keyword, vector []SearchResult, opts *SearchOptions) ([]SearchResult, error) {
	merged := make(map[string]SearchResult)

	for _, r := range keyword {
		key := fmt.Sprintf("%s:%d-%d", r.Path, r.StartLine, r.EndLine)
		r.Score = r.Score * m.config.TextWeight
		merged[key] = r
	}

	for _, r := range vector {
		key := fmt.Sprintf("%s:%d-%d", r.Path, r.StartLine, r.EndLine)
		if existing, ok := merged[key]; ok {
			existing.Score += r.Score * m.config.VectorWeight
			merged[key] = existing
		} else {
			r.Score = r.Score * m.config.VectorWeight
			merged[key] = r
		}
	}

	results := make([]SearchResult, 0, len(merged))
	for _, r := range merged {
		results = append(results, r)
	}

	sortResultsByScore(results)
	if len(results) > opts.MaxResults {
		results = results[:opts.MaxResults]
	}

	return results, nil
}

func (m *MemoryManager) AddChunk(ctx context.Context, chunk *Chunk, embedding []float32) error {
	embeddingJSON, err := json.Marshal(embedding)
	if err != nil {
		return fmt.Errorf("failed to marshal embedding: %w", err)
	}

	_, err = m.db.Exec(`
		INSERT OR REPLACE INTO chunks (id, path, source, start_line, end_line, hash, model, text, embedding, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, chunk.ID, chunk.Path, chunk.Source, chunk.StartLine, chunk.EndLine, chunk.Hash, chunk.Model, chunk.Text, embeddingJSON, chunk.UpdatedAt.Unix())

	return err
}

func (m *MemoryManager) AddText(ctx context.Context, text, path string, source MemorySource) error {
	if m.provider == nil {
		return fmt.Errorf("no embedding provider")
	}

	embedding, err := m.provider.EmbedQuery(ctx, text)
	if err != nil {
		return fmt.Errorf("failed to embed text: %w", err)
	}

	chunk := &Chunk{
		ID:        generateID(path, 0, 0),
		Path:      path,
		Source:    source,
		StartLine: 0,
		EndLine:   0,
		Hash:      hashText(text),
		Model:     m.provider.Model(),
		Text:      text,
		UpdatedAt: time.Now(),
	}

	return m.AddChunk(ctx, chunk, embedding)
}

func (m *MemoryManager) Close() error {
	if m.db != nil {
		return m.db.Close()
	}
	return nil
}

func extractKeywords(text string) []string {
	words := strings.Fields(strings.ToLower(text))
	var keywords []string
	for _, word := range words {
		if len(word) > 2 {
			keywords = append(keywords, word)
		}
	}
	return keywords
}

func truncateText(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen] + "..."
}

func deduplicateResults(results []SearchResult, maxResults int) []SearchResult {
	seen := make(map[string]bool)
	var deduped []SearchResult
	for _, r := range results {
		key := fmt.Sprintf("%s:%d-%d", r.Path, r.StartLine, r.EndLine)
		if !seen[key] {
			seen[key] = true
			deduped = append(deduped, r)
			if len(deduped) >= maxResults {
				break
			}
		}
	}
	return deduped
}

func sortResultsByScore(results []SearchResult) {
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (sqrt(normA) * sqrt(normB))
}

func sqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}
	z := x
	for i := 0; i < 10; i++ {
		z = (z + x/z) / 2
	}
	return z
}

func generateID(path string, start, end int) string {
	return fmt.Sprintf("%s:%d:%d", path, start, end)
}

func hashText(text string) string {
	return fmt.Sprintf("%x", len(text))
}
