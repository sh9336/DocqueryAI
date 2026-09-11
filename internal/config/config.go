package config

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL    string
	CohereAPIKey   string
	Port           string
	AllowedOrigins string // comma-separated list of allowed CORS origins
	Env            string // "development" or "production"

	// Session lease limits
	MaxSessions             int
	MaxSessionsPerIP        int
	SessionHeartbeatTimeout time.Duration
	SessionMaxLifetime      time.Duration

	// Per-session resource limits
	MaxDocsPerSession      int
	MaxFileSizeMB          int
	MaxPagesPerDoc         int
	MaxChunksPerSession    int
	MaxQuestionsPerSession int
	MaxQuestionLength      int

	// Shared Cohere budget protection
	CohereEmbedRPM int
	CohereChatRPM  int

	// Debug endpoints are only registered when this is non-empty
	DebugToken string
}

func LoadConfig() *Config {
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found, relying on environment variables")
	}

	return &Config{
		DatabaseURL:    getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/rag_scheduler?sslmode=disable"),
		CohereAPIKey:   getEnv("COHERE_API_KEY", ""),
		Port:           getEnv("PORT", "8080"),
		AllowedOrigins: getEnv("ALLOWED_ORIGINS", "http://localhost:3000"),
		Env:            getEnv("ENV", "development"),

		MaxSessions:             getEnvInt("MAX_SESSIONS", 5),
		MaxSessionsPerIP:        getEnvInt("MAX_SESSIONS_PER_IP", 2),
		SessionHeartbeatTimeout: getEnvDuration("SESSION_HEARTBEAT_TIMEOUT", 2*time.Minute),
		SessionMaxLifetime:      getEnvDuration("SESSION_MAX_LIFETIME", 2*time.Hour),

		MaxDocsPerSession:      getEnvInt("MAX_DOCS_PER_SESSION", 3),
		MaxFileSizeMB:          getEnvInt("MAX_FILE_SIZE_MB", 10),
		MaxPagesPerDoc:         getEnvInt("MAX_PAGES_PER_DOC", 50),
		MaxChunksPerSession:    getEnvInt("MAX_CHUNKS_PER_SESSION", 2000),
		MaxQuestionsPerSession: getEnvInt("MAX_QUESTIONS_PER_SESSION", 30),
		MaxQuestionLength:      getEnvInt("MAX_QUESTION_LENGTH", 2000),

		CohereEmbedRPM: getEnvInt("COHERE_EMBED_RPM", 5),
		CohereChatRPM:  getEnvInt("COHERE_CHAT_RPM", 10),

		DebugToken: getEnv("DEBUG_TOKEN", ""),
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if value, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(value); err == nil {
			return n
		}
		log.Printf("Invalid int for %s=%q, using default %d", key, value, fallback)
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if value, ok := os.LookupEnv(key); ok {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
		log.Printf("Invalid duration for %s=%q, using default %s", key, value, fallback)
	}
	return fallback
}
