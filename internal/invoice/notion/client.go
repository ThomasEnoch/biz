package notion

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"time"

	"biz/internal/invoice"
	perr "biz/internal/platform/errors"
)

const (
	notionVersionDefault = "2022-06-28"
	notionVersionUpload  = "2025-09-03"
	maxSinglePartBytes   = 20 * 1024 * 1024
)

type Client struct {
	HTTP       *http.Client
	Token      string
	BaseURL    string
	InvoiceDB  string
	RetryCount int
	BackoffMS  int
	rand       *rand.Rand
}

func New(token, baseURL, invoiceDB string, timeout time.Duration, retryCount, backoffMS int) *Client {
	if baseURL == "" {
		baseURL = "https://api.notion.com/v1"
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if retryCount <= 0 {
		retryCount = 3
	}
	if backoffMS <= 0 {
		backoffMS = 200
	}
	return &Client{
		HTTP:       &http.Client{Timeout: timeout},
		Token:      token,
		BaseURL:    strings.TrimRight(baseURL, "/"),
		InvoiceDB:  invoiceDB,
		RetryCount: retryCount,
		BackoffMS:  backoffMS,
		rand:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (c *Client) ensureToken() error {
	if strings.TrimSpace(c.Token) == "" {
		return perr.New(perr.KindDependencyUnavailable, "notion not configured")
	}
	return nil
}

func (c *Client) ensureInvoiceDB() error {
	if strings.TrimSpace(c.InvoiceDB) == "" {
		return perr.New(perr.KindDependencyUnavailable, "notion invoice_db_id is not configured")
	}
	return nil
}

func (c *Client) do(ctx context.Context, method, url string, body []byte) ([]byte, error) {
	return c.doWithVersion(ctx, method, url, body, notionVersionDefault, "application/json")
}

func (c *Client) doWithVersion(ctx context.Context, method, url string, body []byte, notionVersion, contentType string) ([]byte, error) {
	if err := c.ensureToken(); err != nil {
		return nil, err
	}
	if notionVersion == "" {
		notionVersion = notionVersionDefault
	}
	if contentType == "" {
		contentType = "application/json"
	}
	for i := 0; i < c.RetryCount; i++ {
		req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
		if err != nil {
			return nil, perr.Wrap(perr.KindInternal, "failed to build notion request", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.Token)
		req.Header.Set("Content-Type", contentType)
		req.Header.Set("Notion-Version", notionVersion)

		resp, err := c.HTTP.Do(req)
		if err != nil {
			if i < c.RetryCount-1 {
				time.Sleep(c.backoff(i))
				continue
			}
			return nil, perr.Wrap(perr.KindDependencyUnavailable, "notion request failed", err)
		}
		b, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			if i < c.RetryCount-1 {
				time.Sleep(c.backoff(i))
				continue
			}
			return nil, perr.Wrap(perr.KindDependencyUnavailable, "failed reading notion response", readErr)
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return b, nil
		}
		if i < c.RetryCount-1 && (resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500) {
			time.Sleep(c.backoff(i))
			continue
		}
		if resp.StatusCode == http.StatusNotFound {
			return nil, perr.New(perr.KindNotFound, "notion page not found")
		}
		respSnippet := strings.TrimSpace(string(b))
		if len(respSnippet) > 300 {
			respSnippet = respSnippet[:300]
		}
		return nil, perr.New(
			perr.KindDependencyUnavailable,
			fmt.Sprintf("notion error: %s (%s %s) %s", resp.Status, method, url, respSnippet),
		)
	}
	return nil, perr.New(perr.KindDependencyUnavailable, "notion unavailable")
}

func (c *Client) backoff(attempt int) time.Duration {
	base := time.Duration(c.BackoffMS*(1<<attempt)) * time.Millisecond
	jitter := time.Duration(c.rand.Intn(c.BackoffMS+1)) * time.Millisecond
	return base + jitter
}

func (c *Client) GetInvoice(ctx context.Context, id string) (invoice.InvoiceDraft, error) {
	url := fmt.Sprintf("%s/pages/%s", c.BaseURL, id)
	b, err := c.do(ctx, http.MethodGet, url, nil)
	if err != nil {
		return invoice.InvoiceDraft{}, err
	}
	var page rawPage
	if err := json.Unmarshal(b, &page); err != nil {
		return invoice.InvoiceDraft{}, perr.Wrap(perr.KindDependencyUnavailable, "invalid notion invoice response", err)
	}

	worklogPages, err := c.fetchRelatedPages(ctx, page.Properties, "Worklogs")
	if err != nil {
		return invoice.InvoiceDraft{}, err
	}
	costPages, err := c.fetchRelatedPages(ctx, page.Properties, "Costs")
	if err != nil {
		return invoice.InvoiceDraft{}, err
	}
	clientPages, err := c.fetchRelatedPages(ctx, page.Properties, "Clients")
	if err != nil {
		return invoice.InvoiceDraft{}, err
	}
	var clientPage *rawPage
	if len(clientPages) > 0 {
		clientPage = &clientPages[0]
	}

	draft, err := mapInvoicePage(page, worklogPages, costPages, clientPage)
	if err != nil {
		return invoice.InvoiceDraft{}, err
	}
	return draft, nil
}

func (c *Client) ListInvoices(ctx context.Context, status string, limit int, cursor string) ([]invoice.InvoiceSummary, string, error) {
	if err := c.ensureInvoiceDB(); err != nil {
		return nil, "", err
	}
	payload := map[string]any{"page_size": limit}
	if cursor != "" {
		payload["start_cursor"] = cursor
	}
	if status != "" {
		payload["filter"] = map[string]any{
			"property": "Status",
			"status":   map[string]any{"equals": status},
		}
	}
	body, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s/databases/%s/query", c.BaseURL, c.InvoiceDB)
	b, err := c.do(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, "", err
	}
	var qr struct {
		Results    []rawPage `json:"results"`
		NextCursor string    `json:"next_cursor"`
	}
	if err := json.Unmarshal(b, &qr); err != nil {
		return nil, "", perr.Wrap(perr.KindDependencyUnavailable, "invalid notion list response", err)
	}
	items := make([]invoice.InvoiceSummary, 0, len(qr.Results))
	for _, p := range qr.Results {
		s := mapSummaryPage(p)
		if s.PageID == "" {
			continue
		}
		items = append(items, s)
	}
	return items, qr.NextCursor, nil
}

func (c *Client) UploadInvoicePDF(ctx context.Context, pageID, path string) error {
	if err := c.ensureToken(); err != nil {
		return err
	}
	if strings.TrimSpace(pageID) == "" {
		return perr.New(perr.KindValidation, "missing page id for notion pdf upload")
	}
	fb, err := os.ReadFile(path)
	if err != nil {
		return perr.Wrap(perr.KindValidation, "failed to read pdf for notion upload", err)
	}
	if len(fb) == 0 {
		return perr.New(perr.KindValidation, "pdf file is empty")
	}
	if len(fb) > maxSinglePartBytes {
		return perr.New(perr.KindValidation, "pdf exceeds Notion single-part upload limit (20MB)")
	}

	filename := filepath.Base(path)
	if filename == "" || filename == "." || filename == string(filepath.Separator) {
		filename = "invoice.pdf"
	}
	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(filename)))
	if contentType == "" {
		contentType = "application/pdf"
	}

	createReq := map[string]any{
		"mode":         "single_part",
		"filename":     filename,
		"content_type": contentType,
	}
	createBody, _ := json.Marshal(createReq)
	createResp, err := c.doWithVersion(
		ctx,
		http.MethodPost,
		fmt.Sprintf("%s/file_uploads", c.BaseURL),
		createBody,
		notionVersionUpload,
		"application/json",
	)
	if err != nil {
		return err
	}
	var created struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(createResp, &created); err != nil {
		return perr.Wrap(perr.KindDependencyUnavailable, "invalid notion create file upload response", err)
	}
	if strings.TrimSpace(created.ID) == "" {
		return perr.New(perr.KindDependencyUnavailable, "notion create file upload returned empty id")
	}

	var multipartBody bytes.Buffer
	mp := multipart.NewWriter(&multipartBody)
	partHeaders := make(textproto.MIMEHeader)
	partHeaders.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, filename))
	partHeaders.Set("Content-Type", contentType)
	part, err := mp.CreatePart(partHeaders)
	if err != nil {
		return perr.Wrap(perr.KindInternal, "failed to create upload form file", err)
	}
	if _, err := part.Write(fb); err != nil {
		return perr.Wrap(perr.KindInternal, "failed to write upload form file", err)
	}
	if err := mp.Close(); err != nil {
		return perr.Wrap(perr.KindInternal, "failed to finalize upload form", err)
	}

	sendResp, err := c.doWithVersion(
		ctx,
		http.MethodPost,
		fmt.Sprintf("%s/file_uploads/%s/send", c.BaseURL, created.ID),
		multipartBody.Bytes(),
		notionVersionUpload,
		mp.FormDataContentType(),
	)
	if err != nil {
		return err
	}
	var sent struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(sendResp, &sent); err != nil {
		return perr.Wrap(perr.KindDependencyUnavailable, "invalid notion send file upload response", err)
	}
	if sent.Status != "" && sent.Status != "uploaded" {
		return perr.New(perr.KindDependencyUnavailable, "notion file upload did not complete")
	}

	attachReq := map[string]any{
		"properties": map[string]any{
			"Files & media": map[string]any{
				"files": []any{
					map[string]any{
						"type": "file_upload",
						"name": filename,
						"file_upload": map[string]any{
							"id": created.ID,
						},
					},
				},
			},
		},
	}
	attachBody, _ := json.Marshal(attachReq)
	_, err = c.doWithVersion(
		ctx,
		http.MethodPatch,
		fmt.Sprintf("%s/pages/%s", c.BaseURL, pageID),
		attachBody,
		notionVersionUpload,
		"application/json",
	)
	return err
}

func (c *Client) MarkInvoiceStatus(ctx context.Context, pageID, status string) error {
	payload := map[string]any{"properties": map[string]any{"Status": map[string]any{"status": map[string]any{"name": status}}}}
	body, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s/pages/%s", c.BaseURL, pageID)
	_, err := c.do(ctx, http.MethodPatch, url, body)
	return err
}

func (c *Client) fetchRelatedPages(ctx context.Context, props map[string]any, keys ...string) ([]rawPage, error) {
	ids := make([]string, 0, 8)
	seen := map[string]struct{}{}
	for _, key := range keys {
		for _, id := range relationIDs(props, key) {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	pages := make([]rawPage, 0, len(ids))
	for _, rid := range ids {
		cb, err := c.do(ctx, http.MethodGet, fmt.Sprintf("%s/pages/%s", c.BaseURL, rid), nil)
		if err != nil {
			return nil, err
		}
		var cp rawPage
		if err := json.Unmarshal(cb, &cp); err != nil {
			return nil, perr.Wrap(perr.KindDependencyUnavailable, "invalid notion related page response", err)
		}
		pages = append(pages, cp)
	}
	return pages, nil
}
