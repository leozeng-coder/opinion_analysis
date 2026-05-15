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
}

// CrawlerConfig controls on-demand runs from the API (local subprocess).
// In Docker, the backend image has no Python: set enabled: false unless you mount crawler + venv.
type CrawlerConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Root    string `mapstructure:"root"`    // relative to process working directory (run backend from backend/)
	Python  string `mapstructure:"python"`  // optional absolute path; empty = .venv in Root
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
