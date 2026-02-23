package config

import "github.com/spf13/viper"

func applyDefaults(v *viper.Viper) {
	v.SetDefault("profile", "dev")
	v.SetDefault("log.format", "console")
	v.SetDefault("log.level", "info")
	v.SetDefault("modules.enabled", []string{"invoice"})

	v.SetDefault("agent_policy.enabled", false)
	v.SetDefault("agent_policy.allowed_commands", []string{"invoice.list", "invoice.preview"})
	v.SetDefault("agent_policy.invoice_id_regex", "^[a-zA-Z0-9-]{8,64}$")
	v.SetDefault("agent_policy.max_list_limit", 50)
	v.SetDefault("agent_policy.records_allowed_collections", []string{})
	v.SetDefault("agent_policy.records_allowed_properties", []string{})

	v.SetDefault("audit.enabled", false)
	v.SetDefault("audit.path", "audit/biz-audit.log")
	v.SetDefault("audit.signing_key", "")
	v.SetDefault("audit.strict", true)
	v.SetDefault("audit.dir_perm", 448)  // 0700
	v.SetDefault("audit.file_perm", 384) // 0600

	v.SetDefault("notion.base_url", "https://api.notion.com/v1")
	v.SetDefault("notion.collections", map[string]string{})
	v.SetDefault("notion.read_timeout", "10s")
	v.SetDefault("notion.retry_count", 3)
	v.SetDefault("notion.retry_backoff_ms", 200)

	v.SetDefault("invoice.output_dir", "invoices")
	v.SetDefault("invoice.output_base_dir", "")
	v.SetDefault("invoice.template_path", "templates/invoice/default.html.tmpl")
	v.SetDefault("invoice.source", "notion")
	v.SetDefault("invoice.fallback_file", "")
	v.SetDefault("invoice.currency_allow_list", []string{"USD", "EUR", "GBP"})
	v.SetDefault("invoice.create_timeout", "60s")
	v.SetDefault("invoice.pdf_timeout", "30s")
	v.SetDefault("invoice.idempotency_store", "invoices/.idempotency.json")
	v.SetDefault("invoice.idempotency_signing_key", "")
	v.SetDefault("invoice.default_status_query", "Ready to Invoice")
	v.SetDefault("invoice.default_list_limit", 20)
	v.SetDefault("invoice.default_preview_format", "pdf")
	v.SetDefault("invoice.default_upload_notion", false)
	v.SetDefault("invoice.allow_notion_mutations", false)
	v.SetDefault("invoice.require_mutation_confirm", true)
	v.SetDefault("invoice.max_render_html_bytes", 1048576)
	v.SetDefault("invoice.renderer_no_sandbox", false)
	v.SetDefault("invoice.renderer_disable_javascript", true)
	v.SetDefault("invoice.renderer_disable_dev_shm_usage", true)
	v.SetDefault("invoice.renderer_chrome_path", "")

	v.SetDefault("tax.default_region", "US")
	v.SetDefault("tax.required", false)
	v.SetDefault("tax.rates", map[string]float64{"US": 0.0})
}
