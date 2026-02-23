package context

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type MemoryStore struct {
	db       *sql.DB
	dbPath   string
	config   *ContextConfig
	provider EmbeddingProvider
	mu       sync.RWMutex
}

type EmbeddingProvider interface {
	EmbedQuery(ctx context.Context, text string) ([]float32, error)
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
}

var globalMemoryStore *MemoryStore
var memoryStoreOnce sync.Once

func GetMemoryStore() *MemoryStore {
	memoryStoreOnce.Do(func() {
		home, _ := os.UserHomeDir()
		dbPath := filepath.Join(home, ".rizzclaw", "memory.db")
		os.MkdirAll(filepath.Dir(dbPath), 0755)
		
		globalMemoryStore = &MemoryStore{
			dbPath: dbPath,
			config: DefaultContextConfig(),
		}
		globalMemoryStore.initDB()
	})
	return globalMemoryStore
}

func NewMemoryStore(dbPath string, config *ContextConfig, provider EmbeddingProvider) (*MemoryStore, error) {
	if config == nil {
		config = DefaultContextConfig()
	}
	os.MkdirAll(filepath.Dir(dbPath), 0755)
	
	store := &MemoryStore{
		dbPath:   dbPath,
		config:   config,
		provider: provider,
	}
	
	if err := store.initDB(); err != nil {
		return nil, err
	}
	
	return store, nil
}

func (s *MemoryStore) initDB() error {
	var err error
	s.db, err = sql.Open("sqlite3", s.dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS memories (
			id TEXT PRIMARY KEY,
			content TEXT NOT NULL,
			embedding BLOB,
			keywords TEXT,
			source TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			metadata TEXT,
			is_evergreen INTEGER DEFAULT 0
		);
		
		CREATE INDEX IF NOT EXISTS idx_memories_created_at ON memories(created_at);
		CREATE INDEX IF NOT EXISTS idx_memories_source ON memories(source);
		
		CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5(
			content,
			keywords,
			content='memories',
			content_rowid='rowid'
		);
		
		CREATE TABLE IF NOT EXISTS memory_chunks (
			id TEXT PRIMARY KEY,
			memory_id TEXT,
			content TEXT NOT NULL,
			embedding BLOB,
			start_offset INTEGER,
			end_offset INTEGER,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (memory_id) REFERENCES memories(id)
		);
		
		CREATE INDEX IF NOT EXISTS idx_chunks_memory_id ON memory_chunks(memory_id);
	`)
	
	return err
}

func (s *MemoryStore) SetEmbeddingProvider(provider EmbeddingProvider) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.provider = provider
}

func (s *MemoryStore) AddMemory(ctx context.Context, entry *MemoryEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if entry.ID == "" {
		entry.ID = generateMemoryID()
	}
	
	now := time.Now()
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = now
	}
	entry.UpdatedAt = now
	
	if entry.Keywords == nil {
		entry.Keywords = extractKeywords(entry.Content)
	}
	
	var embeddingBlob []byte
	if s.provider != nil && entry.Embedding == nil {
		emb, err := s.provider.EmbedQuery(ctx, entry.Content)
		if err == nil {
			entry.Embedding = emb
		}
	}
	if entry.Embedding != nil {
		embeddingBlob = floatsToBlob(entry.Embedding)
	}
	
	keywordsJSON := strings.Join(entry.Keywords, ",")
	isEvergreen := 0
	if entry.IsEvergreen {
		isEvergreen = 1
	}
	
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO memories (id, content, embedding, keywords, source, created_at, updated_at, metadata, is_evergreen)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, entry.ID, entry.Content, embeddingBlob, keywordsJSON, entry.Source, entry.CreatedAt, entry.UpdatedAt, nil, isEvergreen)
	
	if err != nil {
		return fmt.Errorf("failed to insert memory: %w", err)
	}
	
	_, err = s.db.Exec(`
		INSERT INTO memories_fts(rowid, content, keywords)
		SELECT rowid, content, keywords FROM memories WHERE id = ?
	`, entry.ID)
	
	if err != nil {
	}
	
	return nil
}

func (s *MemoryStore) GetMemory(id string) (*MemoryEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	var entry MemoryEntry
	var embeddingBlob []byte
	var keywordsStr string
	var isEvergreen int
	
	err := s.db.QueryRow(`
		SELECT id, content, embedding, keywords, source, created_at, updated_at, is_evergreen
		FROM memories WHERE id = ?
	`, id).Scan(&entry.ID, &entry.Content, &embeddingBlob, &keywordsStr, &entry.Source, &entry.CreatedAt, &entry.UpdatedAt, &isEvergreen)
	
	if err != nil {
		return nil, err
	}
	
	if len(embeddingBlob) > 0 {
		entry.Embedding = blobToFloats(embeddingBlob)
	}
	if keywordsStr != "" {
		entry.Keywords = strings.Split(keywordsStr, ",")
	}
	entry.IsEvergreen = isEvergreen == 1
	
	return &entry, nil
}

func (s *MemoryStore) DeleteMemory(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	_, err := s.db.Exec(`DELETE FROM memories WHERE id = ?`, id)
	return err
}

func (s *MemoryStore) Search(ctx context.Context, opts *SearchOptions) ([]*SearchResult, error) {
	if opts == nil {
		opts = DefaultSearchOptions()
	}
	
	var results []*SearchResult
	
	if s.provider != nil {
		vectorResults, err := s.vectorSearch(ctx, opts)
		if err == nil && len(vectorResults) > 0 {
			results = append(results, vectorResults...)
		}
	}
	
	keywordResults, err := s.keywordSearch(opts)
	if err == nil && len(keywordResults) > 0 {
		results = append(results, keywordResults...)
	}
	
	if len(results) == 0 {
		return nil, nil
	}
	
	results = s.mergeResults(results, opts)
	
	if opts.TemporalDecay {
		s.applyTemporalDecay(results, opts.HalfLifeDays)
	}
	
	if opts.UseMMR && len(results) > 1 {
		results = s.applyMMR(results, opts.MMRLambda)
	}
	
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	
	if len(results) > opts.MaxResults {
		results = results[:opts.MaxResults]
	}
	
	filtered := make([]*SearchResult, 0)
	for _, r := range results {
		if r.Score >= opts.MinScore {
			filtered = append(filtered, r)
		}
	}
	
	return filtered, nil
}

func (s *MemoryStore) vectorSearch(ctx context.Context, opts *SearchOptions) ([]*SearchResult, error) {
	if s.provider == nil {
		return nil, fmt.Errorf("no embedding provider configured")
	}
	
	queryEmbedding, err := s.provider.EmbedQuery(ctx, opts.Query)
	if err != nil {
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}
	
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	rows, err := s.db.Query(`
		SELECT id, content, embedding, keywords, source, created_at, updated_at, is_evergreen
		FROM memories WHERE embedding IS NOT NULL
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var results []*SearchResult
	
	for rows.Next() {
		var entry MemoryEntry
		var embeddingBlob []byte
		var keywordsStr string
		var isEvergreen int
		
		err := rows.Scan(&entry.ID, &entry.Content, &embeddingBlob, &keywordsStr, &entry.Source, &entry.CreatedAt, &entry.UpdatedAt, &isEvergreen)
		if err != nil {
			continue
		}
		
		if len(embeddingBlob) > 0 {
			entry.Embedding = blobToFloats(embeddingBlob)
		}
		if keywordsStr != "" {
			entry.Keywords = strings.Split(keywordsStr, ",")
		}
		entry.IsEvergreen = isEvergreen == 1
		
		score := cosineSimilarity(queryEmbedding, entry.Embedding)
		
		results = append(results, &SearchResult{
			Entry:       &entry,
			VectorScore: score,
			Score:       score,
		})
	}
	
	return results, nil
}

func (s *MemoryStore) keywordSearch(opts *SearchOptions) ([]*SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	keywords := extractKeywords(opts.Query)
	if len(keywords) == 0 {
		return nil, nil
	}
	
	query := strings.Join(keywords, " OR ")
	
	rows, err := s.db.Query(`
		SELECT m.id, m.content, m.keywords, m.source, m.created_at, m.updated_at, m.is_evergreen, bm25(memories_fts) as rank
		FROM memories m
		JOIN memories_fts fts ON m.rowid = fts.rowid
		WHERE memories_fts MATCH ?
		ORDER BY rank
		LIMIT ?
	`, query, opts.MaxResults*2)
	
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var results []*SearchResult
	
	for rows.Next() {
		var entry MemoryEntry
		var keywordsStr string
		var isEvergreen int
		var rank float64
		
		err := rows.Scan(&entry.ID, &entry.Content, &keywordsStr, &entry.Source, &entry.CreatedAt, &entry.UpdatedAt, &isEvergreen, &rank)
		if err != nil {
			continue
		}
		
		if keywordsStr != "" {
			entry.Keywords = strings.Split(keywordsStr, ",")
		}
		entry.IsEvergreen = isEvergreen == 1
		
		score := bm25RankToScore(rank)
		
		results = append(results, &SearchResult{
			Entry:        &entry,
			KeywordScore: score,
			Score:        score,
		})
	}
	
	return results, nil
}

func (s *MemoryStore) mergeResults(results []*SearchResult, opts *SearchOptions) []*SearchResult {
	merged := make(map[string]*SearchResult)
	
	for _, r := range results {
		id := r.Entry.ID
		if existing, ok := merged[id]; ok {
			existing.VectorScore = existing.VectorScore * opts.VectorWeight
			existing.KeywordScore = existing.KeywordScore * opts.KeywordWeight
			existing.Score = existing.VectorScore + existing.KeywordScore
		} else {
			r.VectorScore = r.VectorScore * opts.VectorWeight
			r.KeywordScore = r.KeywordScore * opts.KeywordWeight
			r.Score = r.VectorScore + r.KeywordScore
			merged[id] = r
		}
	}
	
	mergedList := make([]*SearchResult, 0, len(merged))
	for _, r := range merged {
		mergedList = append(mergedList, r)
	}
	
	return mergedList
}

func (s *MemoryStore) applyTemporalDecay(results []*SearchResult, halfLifeDays float64) {
	now := time.Now()
	
	for _, r := range results {
		if r.Entry.IsEvergreen {
			r.DecayMultiplier = 1.0
			continue
		}
		
		ageInDays := now.Sub(r.Entry.CreatedAt).Hours() / 24
		r.DecayMultiplier = CalculateTemporalDecay(ageInDays, halfLifeDays)
		r.Score = r.Score * r.DecayMultiplier
	}
}

func (s *MemoryStore) applyMMR(results []*SearchResult, lambda float64) []*SearchResult {
	if len(results) <= 1 {
		return results
	}
	
	selected := make([]*SearchResult, 0, len(results))
	selected = append(selected, results[0])
	
	remaining := make([]*SearchResult, len(results)-1)
	copy(remaining, results[1:])
	
	for len(remaining) > 0 && len(selected) < len(results) {
		var bestIdx int
		var bestScore float64 = -1
		
		for i, candidate := range remaining {
			relevance := candidate.Score
			
			var maxSimilarity float64
			for _, s := range selected {
				if candidate.Entry.Embedding != nil && s.Entry.Embedding != nil {
					sim := cosineSimilarity(candidate.Entry.Embedding, s.Entry.Embedding)
					if sim > maxSimilarity {
						maxSimilarity = sim
					}
				}
			}
			
			mmrScore := lambda*relevance - (1-lambda)*maxSimilarity
			
			if mmrScore > bestScore {
				bestScore = mmrScore
				bestIdx = i
			}
		}
		
		selected = append(selected, remaining[bestIdx])
		remaining = append(remaining[:bestIdx], remaining[bestIdx+1:]...)
	}
	
	return selected
}

func (s *MemoryStore) ListMemories(limit, offset int) ([]*MemoryEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	if limit <= 0 {
		limit = 100
	}
	
	rows, err := s.db.Query(`
		SELECT id, content, keywords, source, created_at, updated_at, is_evergreen
		FROM memories
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var entries []*MemoryEntry
	
	for rows.Next() {
		var entry MemoryEntry
		var keywordsStr string
		var isEvergreen int
		
		err := rows.Scan(&entry.ID, &entry.Content, &keywordsStr, &entry.Source, &entry.CreatedAt, &entry.UpdatedAt, &isEvergreen)
		if err != nil {
			continue
		}
		
		if keywordsStr != "" {
			entry.Keywords = strings.Split(keywordsStr, ",")
		}
		entry.IsEvergreen = isEvergreen == 1
		
		entries = append(entries, &entry)
	}
	
	return entries, nil
}

func (s *MemoryStore) Count() (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM memories`).Scan(&count)
	return count, err
}

func (s *MemoryStore) CleanupOldMemories(maxMemories int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM memories`).Scan(&count)
	if err != nil {
		return err
	}
	
	if count <= maxMemories {
		return nil
	}
	
	deleteCount := count - maxMemories
	
	_, err = s.db.Exec(`
		DELETE FROM memories WHERE id IN (
			SELECT id FROM memories 
			WHERE is_evergreen = 0 
			ORDER BY created_at ASC 
			LIMIT ?
		)
	`, deleteCount)
	
	return err
}

func (s *MemoryStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Close()
}

func generateMemoryID() string {
	return fmt.Sprintf("mem-%d", time.Now().UnixNano())
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
	
	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

func bm25RankToScore(rank float64) float64 {
	normalized := rank
	if normalized < 0 {
		normalized = 0
	}
	return 1 / (1 + normalized)
}

func floatsToBlob(floats []float32) []byte {
	blob := make([]byte, len(floats)*4)
	for i, f := range floats {
		bits := float32ToBits(f)
		blob[i*4] = byte(bits)
		blob[i*4+1] = byte(bits >> 8)
		blob[i*4+2] = byte(bits >> 16)
		blob[i*4+3] = byte(bits >> 24)
	}
	return blob
}

func blobToFloats(blob []byte) []float32 {
	floats := make([]float32, len(blob)/4)
	for i := range floats {
		bits := uint32(blob[i*4]) | uint32(blob[i*4+1])<<8 | uint32(blob[i*4+2])<<16 | uint32(blob[i*4+3])<<24
		floats[i] = bitsToFloat32(bits)
	}
	return floats
}

func float32ToBits(f float32) uint32 {
	return uint32(int32(math.Float32bits(f)))
}

func bitsToFloat32(bits uint32) float32 {
	return math.Float32frombits(uint32(int32(bits)))
}

var stopWords = map[string]bool{
	"the": true, "a": true, "an": true, "is": true, "are": true, "was": true, "were": true,
	"be": true, "been": true, "being": true, "have": true, "has": true, "had": true,
	"do": true, "does": true, "did": true, "will": true, "would": true, "could": true,
	"should": true, "may": true, "might": true, "must": true, "shall": true,
	"can": true, "need": true, "dare": true, "ought": true, "used": true,
	"to": true, "of": true, "in": true, "for": true, "on": true, "with": true,
	"at": true, "by": true, "from": true, "as": true, "into": true, "through": true,
	"during": true, "before": true, "after": true, "above": true, "below": true,
	"between": true, "under": true, "again": true, "further": true, "then": true,
	"once": true, "here": true, "there": true, "when": true, "where": true,
	"why": true, "how": true, "all": true, "each": true, "few": true, "more": true,
	"most": true, "other": true, "some": true, "such": true, "no": true, "nor": true,
	"not": true, "only": true, "own": true, "same": true, "so": true, "than": true,
	"too": true, "very": true, "just": true, "and": true, "but": true, "if": true,
	"or": true, "because": true, "until": true, "while": true, "although": true,
	"though": true, "this": true, "that": true, "these": true, "those": true,
	"i": true, "me": true, "my": true, "myself": true, "we": true, "our": true,
	"ours": true, "ourselves": true, "you": true, "your": true, "yours": true,
	"yourself": true, "yourselves": true, "he": true, "him": true, "his": true,
	"himself": true, "she": true, "her": true, "hers": true, "herself": true,
	"it": true, "its": true, "itself": true, "they": true, "them": true,
	"their": true, "theirs": true, "themselves": true, "what": true, "which": true,
	"who": true, "whom": true, "的": true, "是": true, "在": true, "了": true,
	"和": true, "与": true, "或": true, "我": true, "你": true, "他": true,
	"她": true, "它": true, "们": true, "这": true, "那": true, "有": true,
	"个": true, "上": true, "下": true, "不": true, "就": true, "也": true,
}

func extractKeywords(text string) []string {
	re := regexp.MustCompile(`[\w\u4e00-\u9fff]+`)
	words := re.FindAllString(strings.ToLower(text), -1)
	
	keywords := make([]string, 0)
	seen := make(map[string]bool)
	
	for _, word := range words {
		if len(word) < 2 {
			continue
		}
		if stopWords[word] {
			continue
		}
		if !seen[word] {
			seen[word] = true
			keywords = append(keywords, word)
		}
	}
	
	return keywords
}

func (s *MemoryStore) LoadFromMemoryFile(content string) error {
	sections := strings.Split(content, "\n## ")
	
	for _, section := range sections {
		if strings.TrimSpace(section) == "" {
			continue
		}
		
		lines := strings.SplitN(section, "\n", 2)
		if len(lines) < 2 {
			continue
		}
		
		header := strings.TrimSpace(lines[0])
		body := strings.TrimSpace(lines[1])
		
		isEvergreen := strings.Contains(header, "EVERGREEN")
		
		entry := &MemoryEntry{
			ID:          generateMemoryID(),
			Content:     body,
			Source:      "MEMORY.md",
			CreatedAt:   time.Now(),
			IsEvergreen: isEvergreen,
		}
		
		if err := s.AddMemory(context.Background(), entry); err != nil {
			continue
		}
	}
	
	return nil
}
