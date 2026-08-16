package config

import (
	"net/url"
	"os"
	"strings"
)

type Config struct {
	PostgresURL      string
	PostgresHost     string
	PostgresPort     string
	PostgresUser     string
	PostgresPassword string
	PostgresDB       string
	PostgresSSLMode  string

	ServerPort string
	BindAddr   string
	DataDir    string

	CORSOrigins     []string
	ElasticURL      string
	NHTSABaseURL    string
	NHTSARecallsURL string

	// InternalAPIKey guards /api/internal/* endpoints.
	// Set via INTERNAL_API_KEY env var. When empty the routes are disabled.
	InternalAPIKey string

	// TecDocDSN is the MySQL DSN for the TecDoc catalog database.
	// Example: "user:pass@tcp(host:3306)/tecdoc?parseTime=true"
	// When empty, all TecDoc MySQL features are disabled gracefully.
	TecDocDSN string
}

func Load() *Config {
	origins := envOr("CORS_ORIGINS", "http://localhost:5173,http://localhost:3000")
	return &Config{
		PostgresURL:      os.Getenv("DATABASE_URL"),
		PostgresHost:     envOr("PGHOST", "127.0.0.1"),
		PostgresPort:     envOr("PGPORT", "5432"),
		PostgresUser:     envOr("PGUSER", "postgres"),
		PostgresPassword: os.Getenv("PGPASSWORD"),
		PostgresDB:       envOr("PGDATABASE", "parts_engine"),
		PostgresSSLMode:  envOr("PGSSLMODE", "disable"),
		ServerPort:       envOr("PORT", "8080"),
		BindAddr:         envOr("BIND_ADDR", "0.0.0.0"),
		DataDir:          envOr("DATA_DIR", ""),
		CORSOrigins:      splitCSV(origins),
		ElasticURL:       envOr("ELASTIC_URL", "http://localhost:9200"),
		NHTSABaseURL:     envOr("NHTSA_URL", "https://vpic.nhtsa.dot.gov/api"),
		NHTSARecallsURL:  envOr("NHTSA_RECALLS_URL", "https://api.nhtsa.gov/recalls"),
		InternalAPIKey:   os.Getenv("INTERNAL_API_KEY"),
		TecDocDSN:        os.Getenv("TECDOC_DSN"),
	}
}

func splitCSV(s string) []string {
	var out []string
	for _, v := range strings.Split(s, ",") {
		v = strings.TrimSpace(v)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func (c *Config) PostgresDSN() string {
	if c.PostgresURL != "" {
		return c.PostgresURL
	}
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.PostgresUser, c.PostgresPassword),
		Host:   c.PostgresHost + ":" + c.PostgresPort,
		Path:   c.PostgresDB,
	}
	q := u.Query()
	q.Set("sslmode", c.PostgresSSLMode)
	u.RawQuery = q.Encode()
	return u.String()
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
