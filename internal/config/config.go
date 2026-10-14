package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the whole runtime configuration. Everything is externalized through
// environment variables; the process refuses to start when a security-relevant
// value is missing.
type Config struct {
	AppPort string

	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string

	JWTSecret    string
	JWTTTL       time.Duration
	SeedPassword string

	UploadDir      string
	MaxUploadBytes int64

	QdrantURL        string
	QdrantCollection string
	VectorMinScore   float64
	SearchMaxResults int
	RRFK             int

	ChunkMaxChars     int
	ChunkOverlapChars int

	ChatBaseURL      string
	ChatAPIKey       string
	ChatModel        string
	EmbeddingBaseURL string
	EmbeddingAPIKey  string
	EmbeddingModel   string
	EmbeddingDim     int

	MCPAuthToken string
	MaxToolCalls int

	LogLevel string
}

func Load() (*Config, error) {
	c := &Config{
		AppPort: env("APP_PORT", "8080"),

		DBHost:     env("DB_HOST", "127.0.0.1"),
		DBPort:     env("DB_PORT", "3306"),
		DBUser:     env("DB_USER", "campusclaw"),
		DBPassword: os.Getenv("DB_PASSWORD"),
		DBName:     env("DB_NAME", "campusclaw"),

		JWTSecret:    os.Getenv("JWT_SECRET"),
		JWTTTL:       time.Duration(envInt("JWT_TTL_SECONDS", 86400)) * time.Second,
		SeedPassword: env("SEED_PASSWORD", "Campus@2026"),

		UploadDir:      env("UPLOAD_DIR", "./uploads"),
		MaxUploadBytes: int64(envInt("MAX_UPLOAD_BYTES", 10*1024*1024)),

		QdrantURL:        env("QDRANT_URL", "http://127.0.0.1:6333"),
		QdrantCollection: env("QDRANT_COLLECTION", "campusclaw_chunks"),
		VectorMinScore:   envFloat("SEARCH_VECTOR_MIN", 0.35),
		SearchMaxResults: envInt("SEARCH_MAX_RESULTS", 20),
		RRFK:             envInt("RRF_K", 60),

		ChunkMaxChars:     envInt("CHUNK_MAX_CHARS", 800),
		ChunkOverlapChars: envInt("CHUNK_OVERLAP_CHARS", 80),

		ChatBaseURL:      strings.TrimRight(env("CHAT_BASE_URL", "https://ai-gateway.devops.hello1023.com"), "/"),
		ChatAPIKey:       os.Getenv("CHAT_API_KEY"),
		ChatModel:        env("CHAT_MODEL", "campus-chat"),
		EmbeddingBaseURL: strings.TrimRight(env("EMBEDDING_BASE_URL", env("CHAT_BASE_URL", "https://ai-gateway.devops.hello1023.com")), "/"),
		EmbeddingAPIKey:  os.Getenv("EMBEDDING_API_KEY"),
		EmbeddingModel:   env("EMBEDDING_MODEL", "campus-embedding"),
		EmbeddingDim:     envInt("EMBEDDING_DIM", 256),

		MCPAuthToken: os.Getenv("MCP_AUTH_TOKEN"),
		MaxToolCalls: envInt("MAX_TOOL_CALLS_PER_REQUEST", 8),
		LogLevel:     env("LOG_LEVEL", "info"),
	}
	if c.EmbeddingAPIKey == "" {
		c.EmbeddingAPIKey = c.ChatAPIKey
	}
	if err := c.validate(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Config) validate() error {
	var missing []string
	if len(c.JWTSecret) < 16 {
		missing = append(missing, "JWT_SECRET (at least 16 characters)")
	}
	if c.DBPassword == "" {
		missing = append(missing, "DB_PASSWORD")
	}
	if c.MCPAuthToken == "" {
		missing = append(missing, "MCP_AUTH_TOKEN")
	}
	if c.EmbeddingDim < 8 || c.EmbeddingDim > 4096 {
		return fmt.Errorf("EMBEDDING_DIM must be between 8 and 4096")
	}
	if c.ChunkMaxChars < 100 || c.ChunkOverlapChars < 0 || c.ChunkOverlapChars >= c.ChunkMaxChars {
		return fmt.Errorf("invalid chunk window: max=%d overlap=%d", c.ChunkMaxChars, c.ChunkOverlapChars)
	}
	if len(missing) > 0 {
		return errors.New("missing required environment variables: " + strings.Join(missing, ", "))
	}
	return nil
}

// DSN builds the MySQL DSN. parseTime is required so DATETIME columns scan into time.Time.
func (c *Config) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci&multiStatements=true",
		c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName)
}

func (c *Config) OfflineAI() bool { return c.ChatAPIKey == "" }

func env(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envFloat(k string, def float64) float64 {
	if v := os.Getenv(k); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}
