package recall

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const DefaultEmbeddingModel = "BAAI/bge-m3"
const DefaultEmbeddingEndpoint = "http://host.docker.internal:18788/v1/embeddings"
const embeddingBatchSize = 5
const maxEmbeddingResponseBytes = 32 << 20

type EmbeddingConfig struct {
	Enabled   bool
	Endpoint  string
	Model     string
	IndexPath string
	Timeout   time.Duration
}

type EmbeddingService struct {
	store  *Store
	cfg    EmbeddingConfig
	client *http.Client
	mu     sync.Mutex
}

type EmbeddingReindexRequest struct {
	Prefix     string `json:"prefix"`
	MaxEntries int    `json:"max_entries"`
}

type EmbeddingReindexResult struct {
	OK        bool      `json:"ok"`
	Enabled   bool      `json:"enabled"`
	Model     string    `json:"model"`
	Endpoint  string    `json:"endpoint,omitempty"`
	IndexPath string    `json:"index_path"`
	Prefix    string    `json:"prefix"`
	Count     int       `json:"count"`
	Dimension int       `json:"dimension,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

type EmbeddingSearchRequest struct {
	Query      string `json:"query"`
	Prefix     string `json:"prefix"`
	MaxResults int    `json:"max_results"`
}

type EmbeddingSearchResult struct {
	OK      bool                    `json:"ok"`
	Enabled bool                    `json:"enabled"`
	Model   string                  `json:"model"`
	Query   string                  `json:"query"`
	Results []EmbeddingSearchHit    `json:"results"`
	Count   int                     `json:"count"`
	Index   EmbeddingIndexSummarize `json:"index"`
}

type EmbeddingSearchHit struct {
	Path        string            `json:"path"`
	Title       string            `json:"title,omitempty"`
	Score       float64           `json:"score"`
	Snippet     string            `json:"snippet"`
	Frontmatter map[string]string `json:"frontmatter,omitempty"`
}

type EmbeddingIndexSummarize struct {
	Model     string    `json:"model"`
	Dimension int       `json:"dimension,omitempty"`
	Count     int       `json:"count"`
	UpdatedAt time.Time `json:"updated_at"`
}

type embeddingIndex struct {
	Model     string                       `json:"model"`
	Dimension int                          `json:"dimension,omitempty"`
	UpdatedAt time.Time                    `json:"updated_at"`
	Documents map[string]embeddingDocument `json:"documents"`
}

type embeddingDocument struct {
	Path        string            `json:"path"`
	Title       string            `json:"title,omitempty"`
	Text        string            `json:"text"`
	Frontmatter map[string]string `json:"frontmatter,omitempty"`
	Vector      []float64         `json:"vector"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

func firstNonEmptyEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func NewEmbeddingService(store *Store, cfg EmbeddingConfig) *EmbeddingService {
	if strings.TrimSpace(cfg.Endpoint) == "" {
		cfg.Endpoint = strings.TrimSpace(os.Getenv("RECALL_EMBEDDING_ENDPOINT"))
	}
	if strings.TrimSpace(cfg.Endpoint) == "" {
		cfg.Endpoint = DefaultEmbeddingEndpoint
	}
	if strings.TrimSpace(cfg.Model) == "" {
		cfg.Model = DefaultEmbeddingModel
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if strings.TrimSpace(cfg.IndexPath) == "" && store != nil {
		cfg.IndexPath = filepath.Join(store.Root(), ".recall", "embedding-index.json")
	}
	return &EmbeddingService{store: store, cfg: cfg, client: &http.Client{Timeout: cfg.Timeout}}
}

func (s *EmbeddingService) Enabled() bool {
	return s != nil && s.cfg.Enabled && strings.TrimSpace(s.cfg.Endpoint) != "" && s.store != nil
}

func (s *EmbeddingService) Status(ctx context.Context) map[string]any {
	status := map[string]any{
		"ok":         true,
		"enabled":    false,
		"model":      DefaultEmbeddingModel,
		"configured": false,
	}
	if s == nil {
		status["reason"] = "embedding service is not configured"
		return status
	}
	status["enabled"] = s.Enabled()
	status["configured"] = strings.TrimSpace(s.cfg.Endpoint) != ""
	status["model"] = s.cfg.Model
	status["endpoint"] = s.cfg.Endpoint
	status["index_path"] = s.cfg.IndexPath
	idx, err := s.loadIndex()
	if err == nil {
		status["index"] = EmbeddingIndexSummarize{Model: idx.Model, Dimension: idx.Dimension, Count: len(idx.Documents), UpdatedAt: idx.UpdatedAt}
	}
	if s.Enabled() {
		ctx, cancel := context.WithTimeout(ctx, s.cfg.Timeout)
		defer cancel()
		if _, err := s.embed(ctx, []string{"health check"}); err != nil {
			status["reachable"] = false
			status["error"] = err.Error()
		} else {
			status["reachable"] = true
		}
	} else {
		status["reason"] = "set RECALL_EMBEDDING_ENABLED=true and RECALL_EMBEDDING_ENDPOINT to enable BGE-M3 indexing"
	}
	return status
}

func (s *EmbeddingService) Reindex(ctx context.Context, req EmbeddingReindexRequest) (EmbeddingReindexResult, error) {
	if !s.Enabled() {
		return EmbeddingReindexResult{}, errors.New("embedding service is disabled or endpoint is empty")
	}
	prefix := strings.TrimSpace(req.Prefix)
	if prefix == "" {
		prefix = "recall/managed/cards"
	}
	maxEntries := req.MaxEntries
	if maxEntries <= 0 || maxEntries > 2000 {
		maxEntries = 1000
	}
	entries, err := s.store.List(prefix, maxEntries)
	if err != nil {
		return EmbeddingReindexResult{}, err
	}
	docs := make([]embeddingDocument, 0, len(entries))
	texts := []string{}
	for _, entry := range entries {
		if entry.Type != "file" || !IsTextFile(entry.Path) {
			continue
		}
		mem, err := s.store.Read(entry.Path)
		if err != nil {
			continue
		}
		text := embeddingText(mem)
		if text == "" {
			continue
		}
		docs = append(docs, embeddingDocument{Path: mem.Path, Title: firstMarkdownTitle(mem.Body), Text: text, Frontmatter: mem.Frontmatter, UpdatedAt: time.Now().UTC()})
		texts = append(texts, text)
	}
	// BGE-M3 对大批量长文本的单次请求耗时会明显放大；小批次可让重建稳定落在 API 超时窗口内。
	vectors := make([][]float64, 0, len(texts))
	for start := 0; start < len(texts); start += embeddingBatchSize {
		end := min(start+embeddingBatchSize, len(texts))
		batch, err := s.embed(ctx, texts[start:end])
		if err != nil {
			return EmbeddingReindexResult{}, err
		}
		vectors = append(vectors, batch...)
	}
	if len(vectors) != len(docs) {
		return EmbeddingReindexResult{}, fmt.Errorf("embedding response count mismatch: got %d want %d", len(vectors), len(docs))
	}
	dimension := 0
	if len(vectors) > 0 {
		dimension = len(vectors[0])
	}
	index := embeddingIndex{Model: s.cfg.Model, UpdatedAt: time.Now().UTC(), Documents: map[string]embeddingDocument{}}
	for i := range docs {
		if len(vectors[i]) != dimension {
			return EmbeddingReindexResult{}, fmt.Errorf("embedding dimension mismatch at result %d: got %d want %d", i, len(vectors[i]), dimension)
		}
		docs[i].Vector = vectors[i]
		index.Documents[docs[i].Path] = docs[i]
	}
	index.Dimension = dimension
	if err := s.writeIndex(index); err != nil {
		return EmbeddingReindexResult{}, err
	}
	return EmbeddingReindexResult{OK: true, Enabled: true, Model: s.cfg.Model, Endpoint: s.cfg.Endpoint, IndexPath: s.cfg.IndexPath, Prefix: prefix, Count: len(index.Documents), Dimension: dimension, UpdatedAt: index.UpdatedAt}, nil
}

func (s *EmbeddingService) Search(ctx context.Context, req EmbeddingSearchRequest) (EmbeddingSearchResult, error) {
	if !s.Enabled() {
		return EmbeddingSearchResult{}, errors.New("embedding service is disabled or endpoint is empty")
	}
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return EmbeddingSearchResult{}, errors.New("query is required")
	}
	maxResults := req.MaxResults
	if maxResults <= 0 || maxResults > 50 {
		maxResults = 8
	}
	idx, err := s.loadIndex()
	if err != nil {
		return EmbeddingSearchResult{}, err
	}
	if idx.Model != s.cfg.Model {
		return EmbeddingSearchResult{}, fmt.Errorf("embedding index model %q does not match configured model %q; rebuild the index", idx.Model, s.cfg.Model)
	}
	if len(idx.Documents) == 0 {
		return EmbeddingSearchResult{
			OK: true, Enabled: true, Model: s.cfg.Model, Query: query,
			Results: []EmbeddingSearchHit{}, Count: 0,
			Index: EmbeddingIndexSummarize{Model: idx.Model, Dimension: idx.Dimension, Count: 0, UpdatedAt: idx.UpdatedAt},
		}, nil
	}
	queryVectors, err := s.embed(ctx, []string{query})
	if err != nil {
		return EmbeddingSearchResult{}, err
	}
	if len(queryVectors) != 1 {
		return EmbeddingSearchResult{}, errors.New("embedding query returned no vector")
	}
	if len(queryVectors[0]) != idx.Dimension {
		return EmbeddingSearchResult{}, fmt.Errorf("embedding query dimension %d does not match index dimension %d; rebuild the index", len(queryVectors[0]), idx.Dimension)
	}
	prefix := strings.TrimSpace(req.Prefix)
	hits := make([]EmbeddingSearchHit, 0, len(idx.Documents))
	for _, doc := range idx.Documents {
		if prefix != "" && !strings.HasPrefix(doc.Path, prefix) {
			continue
		}
		score := cosine(queryVectors[0], doc.Vector)
		hits = append(hits, EmbeddingSearchHit{Path: doc.Path, Title: doc.Title, Score: score, Snippet: snippetFromText(doc.Text), Frontmatter: doc.Frontmatter})
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return hits[i].Path < hits[j].Path
	})
	if len(hits) > maxResults {
		hits = hits[:maxResults]
	}
	return EmbeddingSearchResult{OK: true, Enabled: true, Model: s.cfg.Model, Query: query, Results: hits, Count: len(hits), Index: EmbeddingIndexSummarize{Model: idx.Model, Dimension: idx.Dimension, Count: len(idx.Documents), UpdatedAt: idx.UpdatedAt}}, nil
}

func (s *EmbeddingService) embed(ctx context.Context, texts []string) ([][]float64, error) {
	if len(texts) == 0 {
		return [][]float64{}, nil
	}
	endpoint := strings.TrimSpace(s.cfg.Endpoint)
	if endpoint == "" {
		return nil, errors.New("embedding endpoint is empty")
	}
	endpointURL, payload, err := embeddingRequest(endpoint, s.cfg.Model, texts)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode embedding request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, maxEmbeddingResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read embedding response: %w", err)
	}
	if len(data) > maxEmbeddingResponseBytes {
		return nil, fmt.Errorf("embedding response exceeds %d bytes", maxEmbeddingResponseBytes)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("embedding endpoint returned HTTP %d: %s", res.StatusCode, truncateUTF8(strings.TrimSpace(string(data)), 4096))
	}
	return parseEmbeddingResponse(data)
}

func embeddingRequest(endpoint, model string, texts []string) (string, map[string]any, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", nil, err
	}
	path := strings.TrimRight(u.Path, "/")
	if path == "" {
		u.Path = "/v1/embeddings"
		return u.String(), map[string]any{"model": model, "input": texts}, nil
	}
	if strings.HasSuffix(path, "/embed") {
		return u.String(), map[string]any{"model": model, "input": texts}, nil
	}
	return u.String(), map[string]any{"model": model, "input": texts}, nil
}

func parseEmbeddingResponse(data []byte) ([][]float64, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if dataValue, ok := raw["data"]; ok {
		items, ok := dataValue.([]any)
		if !ok {
			return nil, errors.New("embedding response data is not an array")
		}
		vectors := make([][]float64, len(items))
		indexMode := -1
		for position, item := range items {
			obj, ok := item.(map[string]any)
			if !ok {
				return nil, errors.New("embedding response item is not an object")
			}
			vector, err := vectorFromAny(obj["embedding"])
			if err != nil {
				return nil, err
			}
			rawIndex, hasIndex := obj["index"]
			mode := 0
			if hasIndex {
				mode = 1
			}
			if indexMode == -1 {
				indexMode = mode
			} else if indexMode != mode {
				return nil, errors.New("embedding response mixes indexed and unindexed items")
			}
			target := position
			if hasIndex {
				index, ok := rawIndex.(float64)
				if !ok || index != math.Trunc(index) || index < 0 || int(index) >= len(items) {
					return nil, fmt.Errorf("embedding response index is invalid: %v", rawIndex)
				}
				target = int(index)
			}
			if vectors[target] != nil {
				return nil, fmt.Errorf("embedding response index %d is duplicated", target)
			}
			vectors[target] = vector
		}
		for index, vector := range vectors {
			if vector == nil {
				return nil, fmt.Errorf("embedding response index %d is missing", index)
			}
		}
		return vectors, nil
	}
	if values, ok := raw["embeddings"]; ok {
		items, ok := values.([]any)
		if !ok {
			return nil, errors.New("embedding response embeddings is not an array")
		}
		vectors := make([][]float64, 0, len(items))
		for _, item := range items {
			vector, err := vectorFromAny(item)
			if err != nil {
				return nil, err
			}
			vectors = append(vectors, vector)
		}
		return vectors, nil
	}
	if value, ok := raw["embedding"]; ok {
		vector, err := vectorFromAny(value)
		if err != nil {
			return nil, err
		}
		return [][]float64{vector}, nil
	}
	return nil, errors.New("embedding response missing data, embeddings, or embedding")
}

func vectorFromAny(value any) ([]float64, error) {
	items, ok := value.([]any)
	if !ok {
		return nil, errors.New("embedding vector is not an array")
	}
	vector := make([]float64, 0, len(items))
	for _, item := range items {
		number, ok := item.(float64)
		if !ok {
			return nil, errors.New("embedding vector contains a non-number")
		}
		vector = append(vector, number)
	}
	if len(vector) == 0 {
		return nil, errors.New("embedding vector is empty")
	}
	return vector, nil
}

func (s *EmbeddingService) loadIndex() (embeddingIndex, error) {
	data, err := os.ReadFile(s.cfg.IndexPath)
	if err != nil {
		return embeddingIndex{}, err
	}
	var idx embeddingIndex
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&idx); err != nil {
		return embeddingIndex{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return embeddingIndex{}, errors.New("embedding index contains multiple JSON values")
		}
		return embeddingIndex{}, fmt.Errorf("read trailing embedding index data: %w", err)
	}
	if err := validateEmbeddingIndex(idx); err != nil {
		return embeddingIndex{}, err
	}
	return idx, nil
}

func validateEmbeddingIndex(idx embeddingIndex) error {
	if strings.TrimSpace(idx.Model) == "" {
		return errors.New("embedding index model is empty")
	}
	if idx.Documents == nil {
		return errors.New("embedding index documents are missing")
	}
	if len(idx.Documents) == 0 {
		if idx.Dimension != 0 {
			return errors.New("empty embedding index must have dimension 0")
		}
		return nil
	}
	if idx.Dimension <= 0 {
		return errors.New("embedding index dimension must be positive")
	}
	for path, document := range idx.Documents {
		if path == "" || document.Path != path {
			return fmt.Errorf("embedding index document path mismatch for %q", path)
		}
		if len(document.Vector) != idx.Dimension {
			return fmt.Errorf("embedding index document %q has dimension %d, want %d", path, len(document.Vector), idx.Dimension)
		}
	}
	return nil
}

func (s *EmbeddingService) writeIndex(idx embeddingIndex) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validateEmbeddingIndex(idx); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.cfg.IndexPath), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(s.cfg.IndexPath, append(data, '\n'), 0o600)
}

func embeddingText(mem Recall) string {
	parts := []string{mem.Path, firstMarkdownTitle(mem.Body), frontmatterText(mem.Frontmatter), mem.Body}
	text := strings.TrimSpace(strings.Join(parts, "\n"))
	if len(text) > 8000 {
		text = truncateUTF8(text, 8000)
	}
	return text
}

func snippetFromText(text string) string {
	text = strings.TrimSpace(text)
	if len(text) > 260 {
		return strings.TrimSpace(truncateUTF8(text, 260))
	}
	return text
}

func cosine(a, b []float64) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
