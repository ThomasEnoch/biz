package notion

import (
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUploadInvoicePDF(t *testing.T) {
	t.Parallel()

	var (
		created  bool
		sent     bool
		attached bool
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token-1" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		if got := r.Header.Get("Notion-Version"); got != notionVersionUpload {
			t.Fatalf("unexpected notion version: %q", got)
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/file_uploads":
			created = true
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode create body: %v", err)
			}
			if body["mode"] != "single_part" {
				t.Fatalf("unexpected upload mode: %#v", body["mode"])
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"fu_123","status":"pending"}`)
			return
		case r.Method == http.MethodPost && r.URL.Path == "/v1/file_uploads/fu_123/send":
			sent = true
			mt, params, err := mimeParseMediaType(r.Header.Get("Content-Type"))
			if err != nil {
				t.Fatalf("parse media type: %v", err)
			}
			if mt != "multipart/form-data" {
				t.Fatalf("unexpected content type: %q", mt)
			}
			mr := multipart.NewReader(r.Body, params["boundary"])
			part, err := mr.NextPart()
			if err != nil {
				t.Fatalf("read multipart part: %v", err)
			}
			if part.FormName() != "file" {
				t.Fatalf("unexpected form name: %q", part.FormName())
			}
			content, err := io.ReadAll(part)
			if err != nil {
				t.Fatalf("read multipart payload: %v", err)
			}
			if len(content) == 0 {
				t.Fatal("empty file payload")
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"fu_123","status":"uploaded"}`)
			return
		case r.Method == http.MethodPatch && r.URL.Path == "/v1/pages/page_123":
			attached = true
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode attach body: %v", err)
			}
			properties, ok := body["properties"].(map[string]any)
			if !ok {
				t.Fatalf("missing properties payload: %#v", body["properties"])
			}
			filesProp, ok := properties["Files & media"].(map[string]any)
			if !ok {
				t.Fatalf("missing files property payload: %#v", properties)
			}
			files, ok := filesProp["files"].([]any)
			if !ok || len(files) != 1 {
				t.Fatalf("unexpected files payload: %#v", filesProp["files"])
			}
			fileObj, ok := files[0].(map[string]any)
			if !ok {
				t.Fatalf("unexpected file object payload: %#v", files[0])
			}
			fileUpload, ok := fileObj["file_upload"].(map[string]any)
			if !ok || fileUpload["id"] != "fu_123" {
				t.Fatalf("unexpected file upload payload: %#v", fileObj["file_upload"])
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{}`)
			return
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	tmp := t.TempDir()
	p := filepath.Join(tmp, "invoice.pdf")
	if err := os.WriteFile(p, []byte("%PDF-1.7\nfake\n"), 0o600); err != nil {
		t.Fatalf("write temp pdf: %v", err)
	}

	c := New("token-1", srv.URL+"/v1", "db_123", 2*time.Second, 1, 10)
	if err := c.UploadInvoicePDF(context.Background(), "page_123", p); err != nil {
		t.Fatalf("UploadInvoicePDF failed: %v", err)
	}
	if !created || !sent || !attached {
		t.Fatalf("expected create/send/attach flow, got created=%v sent=%v attached=%v", created, sent, attached)
	}
}

func TestUploadInvoicePDFRejectsEmptyFile(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	p := filepath.Join(tmp, "invoice.pdf")
	if err := os.WriteFile(p, nil, 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	c := New("token-1", "https://api.notion.com/v1", "db_123", 2*time.Second, 1, 10)
	if err := c.UploadInvoicePDF(context.Background(), "page_123", p); err == nil {
		t.Fatal("expected error for empty file")
	}
}

func mimeParseMediaType(v string) (string, map[string]string, error) {
	semi := strings.Index(v, ";")
	if semi < 0 {
		return strings.TrimSpace(v), map[string]string{}, nil
	}
	mt := strings.TrimSpace(v[:semi])
	params := map[string]string{}
	rest := strings.TrimSpace(v[semi+1:])
	for _, p := range strings.Split(rest, ";") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		kv := strings.SplitN(p, "=", 2)
		if len(kv) != 2 {
			continue
		}
		params[strings.TrimSpace(kv[0])] = strings.Trim(strings.TrimSpace(kv[1]), "\"")
	}
	return mt, params, nil
}
