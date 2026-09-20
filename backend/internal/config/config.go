package config

import (
	"os"
	"strings"
)

type Config struct {
	Port               string
	DatabaseDriver     string
	MySQLDSN           string
	SQLitePath         string
	ElasticsearchURL   string
	ElasticsearchIndex string
	LLMBaseURL         string
	LLMAPIKey          string
	LLMModel           string
	STTProvider        string
	DemoSeed           bool
}

func Load() Config {
	return Config{
		Port:               value("APP_PORT", "8080"),
		DatabaseDriver:     value("DATABASE_DRIVER", "mysql"),
		MySQLDSN:           value("MYSQL_DSN", "mathwiki:mathwiki@tcp(127.0.0.1:3306)/mathwiki?charset=utf8mb4&parseTime=True&loc=Local"),
		SQLitePath:         value("SQLITE_PATH", "mathwiki-dev.db"),
		ElasticsearchURL:   value("ELASTICSEARCH_URL", "http://127.0.0.1:9200"),
		ElasticsearchIndex: value("ELASTICSEARCH_INDEX", "mathwiki_articles"),
		LLMBaseURL:         value("LLM_BASE_URL", "https://api.deepseek.com"),
		LLMAPIKey:          os.Getenv("LLM_API_KEY"),
		LLMModel:           value("LLM_MODEL", "deepseek-chat"),
		STTProvider:        value("STT_PROVIDER", "mock"),
		DemoSeed:           boolean("DEMO_SEED", true),
	}
}

func boolean(key string, fallback bool) bool {
	current := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if current == "" {
		return fallback
	}
	return current == "1" || current == "true" || current == "yes" || current == "on"
}

func value(key, fallback string) string {
	if current := os.Getenv(key); current != "" {
		return current
	}
	return fallback
}
