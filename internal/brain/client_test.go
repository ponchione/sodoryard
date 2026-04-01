package brain

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestReadDocumentSuccess(t *testing.T) {
	content := "---\ntags: [test]\n---\n# Hello\nWorld"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if !strings.HasPrefix(r.URL.Path, "/vault/") {
			t.Fatalf("path = %s, want /vault/ prefix", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("auth = %q, want Bearer test-key", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(content))
	}))
	defer srv.Close()

	client := NewObsidianClient(srv.URL, "test-key")
	got, err := client.ReadDocument(context.Background(), "notes/hello.md")
	if err != nil {
		t.Fatalf("ReadDocument returned error: %v", err)
	}
	if got != content {
		t.Fatalf("content = %q, want %q", got, content)
	}
}

func TestReadDocumentNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewObsidianClient(srv.URL, "test-key")
	_, err := client.ReadDocument(context.Background(), "missing.md")
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !strings.Contains(err.Error(), "Document not found: missing.md") {
		t.Fatalf("error = %q, want Document not found message", err.Error())
	}
}

func TestReadDocumentAuthFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	client := NewObsidianClient(srv.URL, "bad-key")
	_, err := client.ReadDocument(context.Background(), "notes/hello.md")
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("error = %q, want authentication failed", err.Error())
	}
}

func TestReadDocumentServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal failure"))
	}))
	defer srv.Close()

	client := NewObsidianClient(srv.URL, "test-key")
	_, err := client.ReadDocument(context.Background(), "notes/hello.md")
	if err == nil {
		t.Fatal("expected error for 500")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("error = %q, want 500 status", err.Error())
	}
}

func TestWriteDocumentSuccess(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("method = %s, want PUT", r.Method)
		}
		body, _ := readBodyFromRequest(r)
		gotBody = body
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := NewObsidianClient(srv.URL, "test-key")
	content := "# New Doc\nContent here"
	err := client.WriteDocument(context.Background(), "notes/new.md", content)
	if err != nil {
		t.Fatalf("WriteDocument returned error: %v", err)
	}
	if gotBody != content {
		t.Fatalf("body = %q, want %q", gotBody, content)
	}
}

func TestSearchKeywordSuccess(t *testing.T) {
	results := []searchResultItem{
		{
			Filename: "arch/design.md",
			Score:    0.85,
			Matches: []searchResultContext{
				{Matches: []searchResultMatch{{Context: "The design uses a pipeline..."}}},
			},
		},
		{
			Filename: "notes/auth.md",
			Score:    0.72,
			Matches:  nil,
		},
	}
	data, _ := json.Marshal(results)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if !strings.Contains(r.URL.String(), "search/simple") {
			t.Fatalf("URL = %s, want /search/simple/", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	}))
	defer srv.Close()

	client := NewObsidianClient(srv.URL, "test-key")
	hits, err := client.SearchKeyword(context.Background(), "design pipeline")
	if err != nil {
		t.Fatalf("SearchKeyword returned error: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("hits count = %d, want 2", len(hits))
	}
	if hits[0].Path != "arch/design.md" {
		t.Fatalf("hits[0].Path = %q, want arch/design.md", hits[0].Path)
	}
	if hits[0].Snippet != "The design uses a pipeline..." {
		t.Fatalf("hits[0].Snippet = %q, want snippet text", hits[0].Snippet)
	}
	if hits[0].Score != 0.85 {
		t.Fatalf("hits[0].Score = %v, want 0.85", hits[0].Score)
	}
	if hits[1].Snippet != "" {
		t.Fatalf("hits[1].Snippet = %q, want empty (no matches)", hits[1].Snippet)
	}
}

func TestListDocumentsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"files": ["notes/a.md", "notes/b.md"]}`))
	}))
	defer srv.Close()

	client := NewObsidianClient(srv.URL, "test-key")
	files, err := client.ListDocuments(context.Background(), "notes")
	if err != nil {
		t.Fatalf("ListDocuments returned error: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("files count = %d, want 2", len(files))
	}
	if files[0] != "notes/a.md" {
		t.Fatalf("files[0] = %q, want notes/a.md", files[0])
	}
}

func TestConnectionRefused(t *testing.T) {
	client := NewObsidianClient("http://127.0.0.1:1", "test-key")
	_, err := client.ReadDocument(context.Background(), "notes/hello.md")
	if err == nil {
		t.Fatal("expected connection error")
	}
	if !strings.Contains(err.Error(), "Cannot connect to Obsidian REST API") {
		t.Fatalf("error = %q, want connection message", err.Error())
	}
}

func TestRequestTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewObsidianClient(srv.URL, "test-key")
	client.httpClient.Timeout = 50 * time.Millisecond

	_, err := client.ReadDocument(context.Background(), "notes/slow.md")
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func readBodyFromRequest(r *http.Request) (string, error) {
	data, err := readAll(r.Body)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func readAll(r interface{ Read([]byte) (int, error) }) ([]byte, error) {
	var buf strings.Builder
	b := make([]byte, 4096)
	for {
		n, err := r.Read(b)
		if n > 0 {
			buf.Write(b[:n])
		}
		if err != nil {
			break
		}
	}
	return []byte(buf.String()), nil
}
