package config

import (
	"github.com/spf13/viper"
	"log"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Redis    RedisConfig
	JWT      JWTConfig
	Log      LogConfig
	Crawler  CrawlerConfig `mapstructure:"crawler"`
	Tagger   TaggerConfig  `mapstructure:"tagger"`
	RAG      RAGConfig     `mapstructure:"rag"`
}

// RAGConfig 智能对话混合检索（由独立 Python 服务写入 Milvus Lite + MySQL 增量同步）。
type RAGConfig struct {
	Enabled bool `mapstructure:"enabled"`
	// 本机 RAG/embedding 服务地址，例如 http://127.0.0.1:5055
	EmbeddingServiceURL string `mapstructure:"embedding_service_url"`
	// Managed 为 true 时由 Go 后端在本机拉起/重启 RAG 子进程（仅本地开发，Docker 请 false）。
	Managed bool `mapstructure:"managed"`
	// AutoStart 在 Go 启动且 health 不可达时自动拉起 RAG（需 managed: true）。
	AutoStart bool `mapstructure:"auto_start"`
	Root      string `mapstructure:"root"`          // 相对 backend 工作目录，默认 ../rag
	Python    string `mapstructure:"python"`        // 空则使用 root/.venv
	ServerScript string `mapstructure:"server_script"` // 相对 root，默认 server.py
}

// CrawlerConfig controls on-demand runs from the API (local subprocess).
// In Docker, the backend image has no Python: set enabled: false unless you mount crawler + venv.
type CrawlerConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Root    string `mapstructure:"root"`    // relative to process working directory (run backend from backend/)
	Python  string `mapstructure:"python"`  // optional absolute path; empty = .venv in Root
}

// TaggerConfig 控制后台 AI 自动打标任务。
// LLM 相关字段（Key/BaseURL/Model）不再从 YAML 读取，以数据库 system_settings 为准。
type TaggerConfig struct {
	Enabled         bool   `mapstructure:"enabled"`
	LLMApiKey       string `mapstructure:"llmApiKey"`
	LLMBaseURL      string `mapstructure:"llmBaseUrl"`
	LLMModel        string `mapstructure:"llmModel"`
	IntervalSeconds int    `mapstructure:"intervalSeconds"`
	BatchSize       int    `mapstructure:"batchSize"`
	MaxPerTick      int    `mapstructure:"maxPerTick"`
}

type ServerConfig struct {
	Port string
	Mode string
}

type DatabaseConfig struct {
	DSN         string
	MaxOpenConn int
	MaxIdleConn int
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

type JWTConfig struct {
	Secret     string
	ExpireHour int
}

type LogConfig struct {
	Level    string
	Filename string
}

var Cfg *Config

func Load(path string) {
	viper.SetDefault("crawler.enabled", true)
	viper.SetDefault("crawler.root", "../crawler")
	viper.SetDefault("tagger.enabled", true)
	viper.SetDefault("tagger.llmBaseUrl", "https://api.deepseek.com")
	viper.SetDefault("tagger.llmModel", "deepseek-chat")
	viper.SetDefault("tagger.intervalSeconds", 120)
	viper.SetDefault("tagger.batchSize", 20)
	viper.SetDefault("tagger.maxPerTick", 200)
	viper.SetDefault("rag.enabled", false)
	viper.SetDefault("rag.embedding_service_url", "http://127.0.0.1:5055")
	viper.SetDefault("rag.managed", false)
	viper.SetDefault("rag.auto_start", true)
	viper.SetDefault("rag.root", "../rag")
	viper.SetDefault("rag.server_script", "server.py")
	viper.SetConfigFile(path)
	viper.AutomaticEnv()
	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("failed to read config: %v", err)
	}
	Cfg = &Config{}
	if err := viper.Unmarshal(Cfg); err != nil {
		log.Fatalf("failed to unmarshal config: %v", err)
	}
}
