package invoice

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"biz/internal/platform/clock"
	perr "biz/internal/platform/errors"
	"biz/internal/tax"
)

type NotionClient interface {
	GetInvoice(ctx context.Context, id string) (InvoiceDraft, error)
	ListInvoices(ctx context.Context, status string, limit int, cursor string) ([]InvoiceSummary, string, error)
	UploadInvoicePDF(ctx context.Context, pageID, path string) error
	MarkInvoiceStatus(ctx context.Context, pageID, status string) error
}

type TaxApplier interface {
	Apply(ctx context.Context, in tax.TaxInput) (tax.TaxOutput, error)
}

type TemplateRenderer interface {
	RenderInvoiceHTML(ctx context.Context, doc InvoiceDocument) ([]byte, error)
}

type PDFRenderer interface {
	RenderPDF(ctx context.Context, html []byte) ([]byte, error)
}

type LocalPDFStore interface {
	Save(ctx context.Context, invoiceNumber string, issueDate time.Time, idempotencyKey string, pdf []byte, outDir string) (string, int64, error)
}

type NotionPDFStore interface {
	Store(ctx context.Context, pageID, pdfPath string) error
}

type LocalDataSource interface {
	GetByID(ctx context.Context, path, id string) (InvoiceDraft, error)
	ListByStatus(ctx context.Context, path, status string, limit int, cursor string) ([]InvoiceSummary, string, error)
}

type idempotencyRecord struct {
	Key       string       `json:"key"`
	Result    CreateResult `json:"result"`
	StoredAt  time.Time    `json:"stored_at"`
	Signature string       `json:"signature,omitempty"`
}

type idempotencyStore map[string]idempotencyRecord

type Service struct {
	Clock                  clock.Clock
	Notion                 NotionClient
	Tax                    TaxApplier
	Template               TemplateRenderer
	PDF                    PDFRenderer
	LocalPDF               LocalPDFStore
	NotionPDF              NotionPDFStore
	LocalData              LocalDataSource
	CurrencyAllowList      []string
	DefaultSource          string
	FallbackFile           string
	TemplateVersion        string
	IdempotencyPath        string
	IdempotencySigningKey  string
	CreateTimeout          time.Duration
	PDFTimeout             time.Duration
	MaxRenderHTMLBytes     int
	AllowMutations         bool
	RequireMutationConfirm bool
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (CreateResult, error) {
	ctx, cancel := context.WithTimeout(ctx, s.CreateTimeout)
	defer cancel()
	return s.workflowCreate(ctx, req)
}

func (s *Service) List(ctx context.Context, req ListRequest) (ListResult, error) {
	return s.workflowList(ctx, req)
}

func (s *Service) Preview(ctx context.Context, req PreviewRequest) (PreviewResult, error) {
	return s.workflowPreview(ctx, req)
}

func (s *Service) resolveSource(reqSource string) string {
	src := strings.ToLower(strings.TrimSpace(reqSource))
	if src == "" {
		src = strings.ToLower(strings.TrimSpace(s.DefaultSource))
	}
	if src == "" {
		src = "notion"
	}
	return src
}

func (s *Service) fetchDraft(ctx context.Context, id, source, sourceFile string) (InvoiceDraft, bool, error) {
	if source == "local" {
		path := sourceFile
		if path == "" {
			path = s.FallbackFile
		}
		if path == "" {
			return InvoiceDraft{}, false, perr.New(perr.KindValidation, "source-file is required when source=local")
		}
		d, err := s.LocalData.GetByID(ctx, path, id)
		return d, false, err
	}
	d, err := s.Notion.GetInvoice(ctx, id)
	if err == nil {
		return d, false, nil
	}
	if source != "notion" || s.FallbackFile == "" {
		return InvoiceDraft{}, false, err
	}
	fallback, ferr := s.LocalData.GetByID(ctx, s.FallbackFile, id)
	if ferr != nil {
		return InvoiceDraft{}, false, err
	}
	return fallback, true, nil
}

func (s *Service) idempotencyKey(d InvoiceDraft) string {
	base := fmt.Sprintf("%s:%s:%s", d.PageID, d.LastEditedTime.UTC().Format(time.RFC3339Nano), s.TemplateVersion)
	sum := sha256.Sum256([]byte(base))
	return hex.EncodeToString(sum[:])
}

func (s *Service) computeSubtotal(d InvoiceDraft) float64 {
	total := 0.0
	for _, li := range d.LineItems {
		total += (li.Quantity * li.UnitRate) - li.Discount
	}
	return round2(total)
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func normalizeStatus(v string) string {
	switch strings.TrimSpace(strings.ToLower(v)) {
	case "":
		return ""
	case "ready", "ready to invoice":
		return "Ready to Invoice"
	case "draft":
		return "Draft"
	case "sent":
		return "Sent"
	case "paid":
		return "Paid"
	case "overdue":
		return "Overdue"
	default:
		return v
	}
}

func (s *Service) loadIdempotency() (idempotencyStore, error) {
	store := idempotencyStore{}
	if s.IdempotencyPath == "" {
		return store, nil
	}
	b, err := os.ReadFile(s.IdempotencyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return store, nil
		}
		return nil, err
	}
	if len(b) == 0 {
		return store, nil
	}
	if err := json.Unmarshal(b, &store); err != nil {
		return nil, err
	}
	if strings.TrimSpace(s.IdempotencySigningKey) == "" {
		return store, nil
	}
	for k, rec := range store {
		ok, err := s.verifyIdempotencyRecord(rec)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, perr.New(perr.KindValidation, "idempotency record signature invalid: "+k)
		}
	}
	return store, nil
}

func (s *Service) saveIdempotency(store idempotencyStore) error {
	if s.IdempotencyPath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.IdempotencyPath), 0o700); err != nil {
		return err
	}
	if strings.TrimSpace(s.IdempotencySigningKey) != "" {
		for k, rec := range store {
			sig, err := s.signIdempotencyRecord(rec)
			if err != nil {
				return err
			}
			rec.Signature = sig
			store[k] = rec
		}
	}
	b, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.IdempotencyPath, b, 0o600)
}

func (s *Service) signIdempotencyRecord(rec idempotencyRecord) (string, error) {
	payload := struct {
		Key      string       `json:"key"`
		Result   CreateResult `json:"result"`
		StoredAt string       `json:"stored_at"`
	}{
		Key:      rec.Key,
		Result:   rec.Result,
		StoredAt: rec.StoredAt.UTC().Format(time.RFC3339Nano),
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, []byte(s.IdempotencySigningKey))
	if _, err := mac.Write(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func (s *Service) verifyIdempotencyRecord(rec idempotencyRecord) (bool, error) {
	if strings.TrimSpace(rec.Signature) == "" {
		return false, nil
	}
	want, err := s.signIdempotencyRecord(rec)
	if err != nil {
		return false, err
	}
	return hmac.Equal([]byte(strings.ToLower(strings.TrimSpace(rec.Signature))), []byte(want)), nil
}
