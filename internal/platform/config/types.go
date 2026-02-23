package config

import "time"

type Config struct {
	Profile     string       `mapstructure:"profile"`
	Log         LogConfig    `mapstructure:"log"`
	Modules     Modules      `mapstructure:"modules"`
	AgentPolicy AgentPolicy  `mapstructure:"agent_policy"`
	Audit       AuditConfig  `mapstructure:"audit"`
	Notion      NotionConfig `mapstructure:"notion"`
	Invoice     Invoice      `mapstructure:"invoice"`
	Tax         TaxConfig    `mapstructure:"tax"`
}

type Modules struct {
	Enabled []string `mapstructure:"enabled"`
}

type LogConfig struct {
	Format string `mapstructure:"format"`
	Level  string `mapstructure:"level"`
}

type NotionConfig struct {
	Token          string            `mapstructure:"token"`
	BaseURL        string            `mapstructure:"base_url"`
	InvoiceDBID    string            `mapstructure:"invoice_db_id"`
	Collections    map[string]string `mapstructure:"collections"`
	ReadTimeout    time.Duration     `mapstructure:"read_timeout"`
	RetryCount     int               `mapstructure:"retry_count"`
	RetryBackoffMS int               `mapstructure:"retry_backoff_ms"`
}

type Invoice struct {
	OutputDir              string        `mapstructure:"output_dir"`
	OutputBaseDir          string        `mapstructure:"output_base_dir"`
	TemplatePath           string        `mapstructure:"template_path"`
	Source                 string        `mapstructure:"source"`
	FallbackFile           string        `mapstructure:"fallback_file"`
	CurrencyAllowList      []string      `mapstructure:"currency_allow_list"`
	CreateTimeout          time.Duration `mapstructure:"create_timeout"`
	PDFTimeout             time.Duration `mapstructure:"pdf_timeout"`
	IdempotencyStore       string        `mapstructure:"idempotency_store"`
	IdempotencySigningKey  string        `mapstructure:"idempotency_signing_key"`
	DefaultStatusQuery     string        `mapstructure:"default_status_query"`
	DefaultListLimit       int           `mapstructure:"default_list_limit"`
	DefaultPreviewFormat   string        `mapstructure:"default_preview_format"`
	DefaultUploadNotion    bool          `mapstructure:"default_upload_notion"`
	AllowNotionMutations   bool          `mapstructure:"allow_notion_mutations"`
	RequireMutationConfirm bool          `mapstructure:"require_mutation_confirm"`
	MaxRenderHTMLBytes     int           `mapstructure:"max_render_html_bytes"`
	RendererNoSandbox      bool          `mapstructure:"renderer_no_sandbox"`
	RendererDisableJS      bool          `mapstructure:"renderer_disable_javascript"`
	RendererDisableDevShm  bool          `mapstructure:"renderer_disable_dev_shm_usage"`
	RendererChromePath     string        `mapstructure:"renderer_chrome_path"`
}

type TaxConfig struct {
	DefaultRegion string             `mapstructure:"default_region"`
	Required      bool               `mapstructure:"required"`
	Rates         map[string]float64 `mapstructure:"rates"`
}

type AgentPolicy struct {
	Enabled                   bool     `mapstructure:"enabled"`
	AllowedCommands           []string `mapstructure:"allowed_commands"`
	InvoiceIDRegex            string   `mapstructure:"invoice_id_regex"`
	MaxListLimit              int      `mapstructure:"max_list_limit"`
	RecordsAllowedCollections []string `mapstructure:"records_allowed_collections"`
	RecordsAllowedProperties  []string `mapstructure:"records_allowed_properties"`
}

type AuditConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	Path       string `mapstructure:"path"`
	SigningKey string `mapstructure:"signing_key"`
	Strict     bool   `mapstructure:"strict"`
	DirPerm    uint32 `mapstructure:"dir_perm"`
	FilePerm   uint32 `mapstructure:"file_perm"`
}
