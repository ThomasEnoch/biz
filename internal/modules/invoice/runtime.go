package invoice

import (
	"biz/internal/app"
	"biz/internal/invoice"
	"biz/internal/invoice/datasource"
	"biz/internal/invoice/notion"
	"biz/internal/invoice/pdf"
	"biz/internal/invoice/render"
	"biz/internal/platform/clock"
	perr "biz/internal/platform/errors"
)

func (Module) InitRuntime(rt *app.Runtime) error {
	cfg := rt.Config
	notionClient := notion.New(
		cfg.Notion.Token,
		cfg.Notion.BaseURL,
		cfg.Notion.InvoiceDBID,
		cfg.Notion.ReadTimeout,
		cfg.Notion.RetryCount,
		cfg.Notion.RetryBackoffMS,
	)
	if rt.Tax == nil {
		return perr.New(perr.KindInternal, "tax module not initialized")
	}
	rt.Invoice = &invoice.Service{
		Clock:    clock.RealClock{},
		Notion:   notionClient,
		Tax:      rt.Tax,
		Template: render.NewTemplate(cfg.Invoice.TemplatePath),
		PDF: render.ChromePDF{
			NoSandbox:          cfg.Invoice.RendererNoSandbox,
			DisableDevShmUsage: cfg.Invoice.RendererDisableDevShm,
			DisableJavaScript:  cfg.Invoice.RendererDisableJS,
			ExecPath:           cfg.Invoice.RendererChromePath,
		},
		LocalPDF: pdf.LocalStore{
			OutputBaseDir: cfg.Invoice.OutputBaseDir,
			DirPerm:       0o700,
			FilePerm:      0o600,
		},
		NotionPDF:              pdf.NotionStore{Client: notionClient},
		LocalData:              datasource.LocalJSON{},
		CurrencyAllowList:      cfg.Invoice.CurrencyAllowList,
		DefaultSource:          cfg.Invoice.Source,
		FallbackFile:           cfg.Invoice.FallbackFile,
		TemplateVersion:        "v1",
		IdempotencyPath:        cfg.Invoice.IdempotencyStore,
		IdempotencySigningKey:  cfg.Invoice.IdempotencySigningKey,
		CreateTimeout:          cfg.Invoice.CreateTimeout,
		PDFTimeout:             cfg.Invoice.PDFTimeout,
		MaxRenderHTMLBytes:     cfg.Invoice.MaxRenderHTMLBytes,
		AllowMutations:         cfg.Invoice.AllowNotionMutations,
		RequireMutationConfirm: cfg.Invoice.RequireMutationConfirm,
	}
	return nil
}
