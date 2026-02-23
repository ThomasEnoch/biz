package invoice

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"biz/internal/platform/clock"
	"biz/internal/tax"
)

type fakeNotion struct{ draft InvoiceDraft }

func (f fakeNotion) GetInvoice(ctx context.Context, id string) (InvoiceDraft, error) {
	return f.draft, nil
}
func (f fakeNotion) ListInvoices(ctx context.Context, status string, limit int, cursor string) ([]InvoiceSummary, string, error) {
	return nil, "", nil
}
func (f fakeNotion) UploadInvoicePDF(ctx context.Context, pageID, path string) error    { return nil }
func (f fakeNotion) MarkInvoiceStatus(ctx context.Context, pageID, status string) error { return nil }

type fakeTemplate struct{}

func (fakeTemplate) RenderInvoiceHTML(ctx context.Context, doc InvoiceDocument) ([]byte, error) {
	return []byte("<html><body>ok</body></html>"), nil
}

type fakePDF struct{}

func (fakePDF) RenderPDF(ctx context.Context, html []byte) ([]byte, error) {
	return []byte("%PDF-1.4\n%fake"), nil
}

type fakeStore struct{}

func (fakeStore) Save(ctx context.Context, invoiceNumber string, issueDate time.Time, idempotencyKey string, pdf []byte, outDir string) (string, int64, error) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", 0, err
	}
	p := filepath.Join(outDir, "out.pdf")
	if err := os.WriteFile(p, pdf, 0o644); err != nil {
		return "", 0, err
	}
	return p, int64(len(pdf)), nil
}

type fakeLocalData struct{ draft InvoiceDraft }

func (f fakeLocalData) GetByID(ctx context.Context, path, id string) (InvoiceDraft, error) {
	return f.draft, nil
}
func (f fakeLocalData) ListByStatus(ctx context.Context, path, status string, limit int, cursor string) ([]InvoiceSummary, string, error) {
	return nil, "", nil
}

type fixedClock struct{}

func (fixedClock) Now() time.Time { return time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC) }

var _ clock.Clock = fixedClock{}

func TestCreateIdempotentReplay(t *testing.T) {
	draft := InvoiceDraft{
		PageID:         "page-1",
		InvoiceNumber:  "INV-100",
		ClientName:     "Acme",
		ClientLocation: "US",
		IssueDate:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		DueDate:        time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		Currency:       "USD",
		Status:         "Ready to Invoice",
		LastEditedTime: time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC),
		LineItems:      []LineItem{{Description: "Consulting", Quantity: 2, UnitRate: 100}},
	}
	tmp := t.TempDir()
	s := &Service{
		Clock:             fixedClock{},
		Notion:            fakeNotion{draft: draft},
		Tax:               tax.Service{Rates: map[string]float64{"US": 0}, DefaultRegion: "US"},
		Template:          fakeTemplate{},
		PDF:               fakePDF{},
		LocalPDF:          fakeStore{},
		NotionPDF:         nil,
		LocalData:         fakeLocalData{draft: draft},
		CurrencyAllowList: []string{"USD"},
		DefaultSource:     "notion",
		TemplateVersion:   "v1",
		IdempotencyPath:   filepath.Join(tmp, "idem.json"),
		CreateTimeout:     5 * time.Second,
		PDFTimeout:        2 * time.Second,
	}

	first, err := s.Create(context.Background(), CreateRequest{ID: "page-1", OutDir: tmp})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	second, err := s.Create(context.Background(), CreateRequest{ID: "page-1", OutDir: tmp})
	if err != nil {
		t.Fatalf("second create failed: %v", err)
	}
	if first.IdempotencyKey != second.IdempotencyKey {
		t.Fatalf("expected same key: %s vs %s", first.IdempotencyKey, second.IdempotencyKey)
	}
	if v, ok := second.Meta["idempotent_replay"]; !ok || v != true {
		t.Fatalf("expected idempotent replay meta, got %+v", second.Meta)
	}
}

func TestCreateUploadPolicyRequiresEnableAndConfirm(t *testing.T) {
	draft := InvoiceDraft{
		PageID:         "page-1",
		InvoiceNumber:  "INV-100",
		ClientName:     "Acme",
		ClientLocation: "US",
		IssueDate:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		DueDate:        time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		Currency:       "USD",
		Status:         "Ready to Invoice",
		LastEditedTime: time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC),
		LineItems:      []LineItem{{Description: "Consulting", Quantity: 2, UnitRate: 100}},
	}
	tmp := t.TempDir()
	s := &Service{
		Clock:                  fixedClock{},
		Notion:                 fakeNotion{draft: draft},
		Tax:                    tax.Service{Rates: map[string]float64{"US": 0}, DefaultRegion: "US"},
		Template:               fakeTemplate{},
		PDF:                    fakePDF{},
		LocalPDF:               fakeStore{},
		NotionPDF:              nil,
		LocalData:              fakeLocalData{draft: draft},
		CurrencyAllowList:      []string{"USD"},
		DefaultSource:          "notion",
		TemplateVersion:        "v1",
		IdempotencyPath:        filepath.Join(tmp, "idem.json"),
		CreateTimeout:          5 * time.Second,
		PDFTimeout:             2 * time.Second,
		AllowMutations:         false,
		RequireMutationConfirm: true,
	}

	if _, err := s.Create(context.Background(), CreateRequest{ID: "page-1", OutDir: tmp, UploadNotion: true}); err == nil {
		t.Fatal("expected policy error when mutations disabled")
	}

	s.AllowMutations = true
	if _, err := s.Create(context.Background(), CreateRequest{ID: "page-1", OutDir: tmp, UploadNotion: true}); err == nil {
		t.Fatal("expected confirmation error when confirm flag missing")
	}
}

func TestCreateIdempotentReplayWithSigningKey(t *testing.T) {
	draft := InvoiceDraft{
		PageID:         "page-1",
		InvoiceNumber:  "INV-100",
		ClientName:     "Acme",
		ClientLocation: "US",
		IssueDate:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		DueDate:        time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		Currency:       "USD",
		Status:         "Ready to Invoice",
		LastEditedTime: time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC),
		LineItems:      []LineItem{{Description: "Consulting", Quantity: 2, UnitRate: 100}},
	}
	tmp := t.TempDir()
	s := &Service{
		Clock:                 fixedClock{},
		Notion:                fakeNotion{draft: draft},
		Tax:                   tax.Service{Rates: map[string]float64{"US": 0}, DefaultRegion: "US"},
		Template:              fakeTemplate{},
		PDF:                   fakePDF{},
		LocalPDF:              fakeStore{},
		NotionPDF:             nil,
		LocalData:             fakeLocalData{draft: draft},
		CurrencyAllowList:     []string{"USD"},
		DefaultSource:         "notion",
		TemplateVersion:       "v1",
		IdempotencyPath:       filepath.Join(tmp, "idem.json"),
		IdempotencySigningKey: "test-secret",
		CreateTimeout:         5 * time.Second,
		PDFTimeout:            2 * time.Second,
	}
	first, err := s.Create(context.Background(), CreateRequest{ID: "page-1", OutDir: tmp})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	second, err := s.Create(context.Background(), CreateRequest{ID: "page-1", OutDir: tmp})
	if err != nil {
		t.Fatalf("second create failed: %v", err)
	}
	if first.IdempotencyKey != second.IdempotencyKey {
		t.Fatalf("expected same key: %s vs %s", first.IdempotencyKey, second.IdempotencyKey)
	}
	if v, ok := second.Meta["idempotent_replay"]; !ok || v != true {
		t.Fatalf("expected idempotent replay meta, got %+v", second.Meta)
	}
}

func TestCreateFailsWhenSignedIdempotencyRecordIsTampered(t *testing.T) {
	draft := InvoiceDraft{
		PageID:         "page-1",
		InvoiceNumber:  "INV-100",
		ClientName:     "Acme",
		ClientLocation: "US",
		IssueDate:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		DueDate:        time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		Currency:       "USD",
		Status:         "Ready to Invoice",
		LastEditedTime: time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC),
		LineItems:      []LineItem{{Description: "Consulting", Quantity: 2, UnitRate: 100}},
	}
	tmp := t.TempDir()
	idemPath := filepath.Join(tmp, "idem.json")
	s := &Service{
		Clock:                 fixedClock{},
		Notion:                fakeNotion{draft: draft},
		Tax:                   tax.Service{Rates: map[string]float64{"US": 0}, DefaultRegion: "US"},
		Template:              fakeTemplate{},
		PDF:                   fakePDF{},
		LocalPDF:              fakeStore{},
		NotionPDF:             nil,
		LocalData:             fakeLocalData{draft: draft},
		CurrencyAllowList:     []string{"USD"},
		DefaultSource:         "notion",
		TemplateVersion:       "v1",
		IdempotencyPath:       idemPath,
		IdempotencySigningKey: "test-secret",
		CreateTimeout:         5 * time.Second,
		PDFTimeout:            2 * time.Second,
	}
	if _, err := s.Create(context.Background(), CreateRequest{ID: "page-1", OutDir: tmp}); err != nil {
		t.Fatalf("create failed: %v", err)
	}

	raw, err := os.ReadFile(idemPath)
	if err != nil {
		t.Fatalf("read idempotency file: %v", err)
	}
	var store map[string]map[string]any
	if err := json.Unmarshal(raw, &store); err != nil {
		t.Fatalf("unmarshal idempotency file: %v", err)
	}
	for _, rec := range store {
		if result, ok := rec["result"].(map[string]any); ok {
			result["invoice_number"] = "INV-TAMPERED"
			break
		}
	}
	updated, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		t.Fatalf("marshal tampered file: %v", err)
	}
	if err := os.WriteFile(idemPath, updated, 0o600); err != nil {
		t.Fatalf("write tampered file: %v", err)
	}

	if _, err := s.Create(context.Background(), CreateRequest{ID: "page-1", OutDir: tmp}); err == nil {
		t.Fatal("expected create to fail when idempotency signature is invalid")
	}
}
