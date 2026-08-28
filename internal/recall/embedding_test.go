package recall

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestEmbeddingServiceDisabledStatus(t *testing.T) {
	store := newTestStore(t)
	svc := NewEmbeddingService(store, EmbeddingConfig{})
	status := svc.Status(context.Background())
	if status["enabled"] != false || status["model"] != DefaultEmbeddingModel {
		t.Fatalf("unexpected disabled status: %#v", status)
	}
}

func TestEmbeddingReindexAndSearchWithOpenAICompatibleEndpoint(t *testing.T) {
	store := newTestStore(t)
	if _, err := store.WriteCard(CardRequest{Title: "Deploy verification", Content: "Deployment should verify the final web endpoint.", Type: CardDeployNote, Scope: ScopeProject, Project: "chatdock", Status: StatusInbox, Confirmed: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.WriteCard(CardRequest{Title: "Database backup", Content: "Before database migration, create and verify a backup snapshot.", Type: CardRunbook, Scope: ScopeProject, Project: "vitapulse", Status: StatusInbox, Confirmed: true}); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		var req struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.Model != DefaultEmbeddingModel {
			t.Fatalf("unexpected model %q", req.Model)
		}
		data := []map[string]any{}
		for i, text := range req.Input {
			data = append(data, map[string]any{"index": i, "embedding": fakeEmbeddingVector(text)})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	defer server.Close()

	svc := NewEmbeddingService(store, EmbeddingConfig{Enabled: true, Endpoint: server.URL, IndexPath: filepath.Join(t.TempDir(), "embedding-index.json")})
	indexed, err := svc.Reindex(context.Background(), EmbeddingReindexRequest{Prefix: "recall/managed/cards"})
	if err != nil {
		t.Fatal(err)
	}
	if indexed.Count != 2 || indexed.Model != DefaultEmbeddingModel || indexed.Dimension != 2 {
		t.Fatalf("unexpected reindex result: %#v", indexed)
	}
	result, err := svc.Search(context.Background(), EmbeddingSearchRequest{Query: "deploy endpoint", Prefix: "recall/managed/cards", MaxResults: 2})
	if err != nil {
		t.Fatal(err)
	}
	if result.Count == 0 || !strings.Contains(result.Results[0].Path, "deploy-verification") {
		t.Fatalf("expected deploy card first: %#v", result)
	}
}

func TestParseSimpleEmbeddingResponse(t *testing.T) {
	vectors, err := parseEmbeddingResponse([]byte(`{"embeddings":[[1,0],[0,1]]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(vectors) != 2 || vectors[0][0] != 1 || vectors[1][1] != 1 {
		t.Fatalf("unexpected vectors: %#v", vectors)
	}
}

func fakeEmbeddingVector(text string) []float64 {
	lower := strings.ToLower(text)
	if strings.Contains(lower, "deploy") || strings.Contains(lower, "endpoint") {
		return []float64{1, 0}
	}
	return []float64{0, 1}
}

func TestEmbeddingReindexBatchesLargeCardSets(t *testing.T) {
	store := newTestStore(t)
	for i := 0; i < embeddingBatchSize*2+2; i++ {
		if _, err := store.WriteCard(CardRequest{
			Title:     fmt.Sprintf("Card %02d", i),
			Content:   fmt.Sprintf("Reusable recall content %02d", i),
			Type:      CardRunbook,
			Scope:     ScopeProject,
			Project:   "agentdock",
			Status:    StatusInbox,
			Confirmed: true,
		}); err != nil {
			t.Fatal(err)
		}
	}

	requestSizes := []int{}
	activeRequests := 0
	maxActiveRequests := 0
	var requestMu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		requestMu.Lock()
		requestSizes = append(requestSizes, len(req.Input))
		activeRequests++
		maxActiveRequests = max(maxActiveRequests, activeRequests)
		requestMu.Unlock()
		defer func() {
			requestMu.Lock()
			activeRequests--
			requestMu.Unlock()
		}()
		if len(req.Input) > embeddingBatchSize {
			t.Fatalf("embedding batch too large: %d", len(req.Input))
		}
		// 保持两个请求有重叠，验证实现确实是有限并发，而不是又退回串行。
		time.Sleep(20 * time.Millisecond)
		data := make([]map[string]any, 0, len(req.Input))
		for i, text := range req.Input {
			data = append(data, map[string]any{"index": i, "embedding": fakeEmbeddingVector(text)})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	defer server.Close()

	svc := NewEmbeddingService(store, EmbeddingConfig{Enabled: true, Endpoint: server.URL, IndexPath: filepath.Join(t.TempDir(), "embedding-index.json")})
	indexed, err := svc.Reindex(context.Background(), EmbeddingReindexRequest{Prefix: "recall/managed/cards"})
	if err != nil {
		t.Fatal(err)
	}
	if indexed.Count != embeddingBatchSize*2+2 {
		t.Fatalf("unexpected indexed count %d", indexed.Count)
	}
	requestMu.Lock()
	sort.Ints(requestSizes)
	gotMaxActive := maxActiveRequests
	requestMu.Unlock()
	if len(requestSizes) != 3 || requestSizes[0] != 2 || requestSizes[1] != embeddingBatchSize || requestSizes[2] != embeddingBatchSize {
		t.Fatalf("unexpected embedding batches: %#v", requestSizes)
	}
	if gotMaxActive != embeddingBatchConcurrency {
		t.Fatalf("embedding batch concurrency=%d want=%d", gotMaxActive, embeddingBatchConcurrency)
	}
}

func TestEmbeddingReindexReusesUnchangedVectors(t *testing.T) {
	store := newTestStore(t)
	if _, err := store.Write(WriteRequest{Path: "profile.md", Content: "# Profile\n\nkeep this preference\n", Confirmed: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.WriteCard(CardRequest{
		Title: "Reusable runbook", Content: "This reusable runbook remains unchanged between reindex operations.", Type: CardRunbook,
		Scope: ScopeProject, Project: "agentdock", Status: StatusInbox, Confirmed: true,
	}); err != nil {
		t.Fatal(err)
	}

	embeddedTexts := 0
	var embeddedMu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		embeddedMu.Lock()
		embeddedTexts += len(req.Input)
		embeddedMu.Unlock()
		data := make([]map[string]any, 0, len(req.Input))
		for i, text := range req.Input {
			data = append(data, map[string]any{"index": i, "embedding": fakeEmbeddingVector(text)})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	defer server.Close()

	svc := NewEmbeddingService(store, EmbeddingConfig{Enabled: true, Endpoint: server.URL, IndexPath: filepath.Join(t.TempDir(), "embedding-index.json")})
	if _, err := svc.Reindex(context.Background(), EmbeddingReindexRequest{}); err != nil {
		t.Fatal(err)
	}
	embeddedMu.Lock()
	firstEmbedded := embeddedTexts
	embeddedTexts = 0
	embeddedMu.Unlock()
	if firstEmbedded != 2 {
		t.Fatalf("first reindex embedded %d texts, want 2", firstEmbedded)
	}

	if _, err := svc.Reindex(context.Background(), EmbeddingReindexRequest{}); err != nil {
		t.Fatal(err)
	}
	embeddedMu.Lock()
	unchangedEmbedded := embeddedTexts
	embeddedTexts = 0
	embeddedMu.Unlock()
	if unchangedEmbedded != 0 {
		t.Fatalf("unchanged reindex embedded %d texts, want 0", unchangedEmbedded)
	}

	if _, err := store.Write(WriteRequest{Path: "profile.md", Content: "# Profile\n\nupdated preference\n", Confirmed: true, Overwrite: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Reindex(context.Background(), EmbeddingReindexRequest{}); err != nil {
		t.Fatal(err)
	}
	embeddedMu.Lock()
	changedEmbedded := embeddedTexts
	embeddedMu.Unlock()
	if changedEmbedded != 1 {
		t.Fatalf("changed reindex embedded %d texts, want 1", changedEmbedded)
	}
}

func TestEmbeddingReindexDefaultsToAllRecallDocuments(t *testing.T) {
	store := newTestStore(t)
	if _, err := store.Write(WriteRequest{Path: "profile.md", Content: "# Profile\n\nuser preference\n", Confirmed: true}); err != nil {
		t.Fatal(err)
	}
	card, err := store.WriteCard(CardRequest{
		Title: "Recall card", Content: "reusable card content", Type: CardRunbook,
		Scope: ScopeProject, Project: "agentdock", Status: StatusInbox, Confirmed: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		data := make([]map[string]any, 0, len(req.Input))
		for i := range req.Input {
			data = append(data, map[string]any{"index": i, "embedding": []float64{1, 0}})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	defer server.Close()

	svc := NewEmbeddingService(store, EmbeddingConfig{Enabled: true, Endpoint: server.URL, IndexPath: filepath.Join(t.TempDir(), "embedding-index.json")})
	indexed, err := svc.Reindex(context.Background(), EmbeddingReindexRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if indexed.Prefix != "" || indexed.Count != 2 {
		t.Fatalf("default reindex should cover all Recall documents: %#v", indexed)
	}
	idx, err := svc.loadIndex()
	if err != nil {
		t.Fatal(err)
	}
	if idx.Documents["profile.md"].Path != "profile.md" || idx.Documents[card.Recall.Path].Path != card.Recall.Path {
		t.Fatalf("default index missing profile or card: %#v", idx.Documents)
	}
}

func TestHybridSearchUsesSemanticEnhancementAndLexicalFallback(t *testing.T) {
	store := newTestStore(t)
	const projectPath = "recall/docs/projects/agentdock/project.md"
	if _, err := store.Write(WriteRequest{Path: projectPath, Content: "# Project\n\nhidden-concept\n", Confirmed: true}); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		data := make([]map[string]any, 0, len(req.Input))
		for i, text := range req.Input {
			vector := []float64{0, 1}
			if strings.Contains(text, "hidden-concept") || strings.Contains(text, "semantic intent") {
				vector = []float64{1, 0}
			}
			data = append(data, map[string]any{"index": i, "embedding": vector})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))

	svc := NewEmbeddingService(store, EmbeddingConfig{Enabled: true, Endpoint: server.URL, IndexPath: filepath.Join(t.TempDir(), "embedding-index.json")})
	if _, err := svc.Reindex(context.Background(), EmbeddingReindexRequest{}); err != nil {
		t.Fatal(err)
	}
	semanticOnly, err := svc.HybridSearch(context.Background(), SearchOptions{Query: "semantic intent", MaxResults: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(semanticOnly) == 0 || semanticOnly[0].Path != projectPath {
		t.Fatalf("semantic enhancement did not recover project memory: %#v", semanticOnly)
	}
	if _, err := store.Write(WriteRequest{Path: projectPath, Content: "# Project\n\nupdated-concept\n", Confirmed: true, Overwrite: true}); err != nil {
		t.Fatal(err)
	}
	staleSemantic, err := svc.HybridSearch(context.Background(), SearchOptions{Query: "semantic intent", MaxResults: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(staleSemantic) != 0 {
		t.Fatalf("stale semantic index should not return outdated document: %#v", staleSemantic)
	}

	server.Close()
	fallback, err := svc.HybridSearch(context.Background(), SearchOptions{Query: "updated-concept", MaxResults: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(fallback) == 0 || fallback[0].Path != projectPath {
		t.Fatalf("embedding failure should keep lexical results: %#v", fallback)
	}
}
