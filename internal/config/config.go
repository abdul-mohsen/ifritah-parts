package config

import (
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/go-sql-driver/mysql"
)

// Config holds every runtime setting the server reads at startup. Fields are
// grouped by domain (Postgres = owned catalog / sqlc; MySQL = optional TecDoc
// source; server, CORS, external APIs). Every field is env-driven with an
// explicit default in Load() so the zero-value struct never leaves this file.
type Config struct {
	// PostgreSQL — owned catalog, sqlc-generated store, primary runtime DB.
	PostgresURL      string
	PostgresHost     string
	PostgresPort     string
	PostgresUser     string
	PostgresPassword string
	PostgresDB       string
	PostgresSSLMode  string

	// MySQL — optional TecDoc source (the "big data" on the server: articles,
	// articlecrosses, oem_number, articlesvehicletrees, articlecriteria, etc.).
	// When MySQLHost is empty, the TecDoc reader is skipped and the app runs
	// on Postgres + SQLite cache alone. When set, internal/service/tecdoc.go is
	// initialised and the /health endpoint reports tecdoc:true.
	MySQLHost     string
	MySQLPort     string
	MySQLUser     string
	MySQLPassword string
	MySQLDB       string

	// Server
	ServerPort string
	BindAddr   string
	DataDir    string

	// CORS
	CORSOrigins []string

	// External services
	ElasticURL      string
	NHTSABaseURL    string
	NHTSARecallsURL string
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

		// MySQL — the primary source of truth for parts. Every OEM / text /
		// vehicle search must consult MySQL FIRST (the 21.5M-row oem_number
		// table + articlesvehicletrees + articlecrosses + articlecriteria).
		// The local Postgres + SQLite tables are a cache / enrichment layer,
		// not the authority.
		//
		// Canonical env-var names (documented in C:\ssda\chatGPT\parts\test_queries.go):
		//   HOST      → "host:port" or bare host
		//   DBPORT    → 3306 default (ignored if HOST already has :port)
		//   DBUSER    → root default
		//   PASSWORD  → required in production, empty in dev
		//   DBNAME    → dev_ifritah default
		// Aliases (accepted but secondary): MYSQL_HOST, MYSQL_PORT, MYSQL_USER,
		//   MYSQL_PASSWORD, MYSQL_DATABASE.
		//
		// Empty HOST + no MYSQL_HOST + no ALLOW_NO_MYSQL=1 → server exits at
		// startup. See internal/db/mysql.go for the fail-hard contract.
		MySQLHost:     firstNonEmpty(stripPort(os.Getenv("HOST")), os.Getenv("MYSQL_HOST")),
		MySQLPort:     firstNonEmpty(os.Getenv("DBPORT"), portFrom(os.Getenv("HOST")), os.Getenv("MYSQL_PORT"), "3306"),
		MySQLUser:     firstNonEmpty(os.Getenv("DBUSER"), os.Getenv("MYSQL_USER"), "root"),
		MySQLPassword: firstNonEmpty(os.Getenv("PASSWORD"), os.Getenv("MYSQL_PASSWORD")),
		MySQLDB:       firstNonEmpty(os.Getenv("DBNAME"), os.Getenv("MYSQL_DATABASE"), "dev_ifritah"),

		ServerPort:      envOr("PORT", "8080"),
		BindAddr:        envOr("BIND_ADDR", "0.0.0.0"),
		DataDir:         envOr("DATA_DIR", ""),
		CORSOrigins:     splitCSV(origins),
		ElasticURL:      envOr("ELASTIC_URL", "http://localhost:9200"),
		NHTSABaseURL:    envOr("NHTSA_URL", "https://vpic.nhtsa.dot.gov/api"),
		NHTSARecallsURL: envOr("NHTSA_RECALLS_URL", "https://api.nhtsa.gov/recalls"),
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

// PostgresDSN builds the connection string for the primary runtime DB.
// Honours DATABASE_URL when set; otherwise assembles from the discrete PG* vars.
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

// MySQLEnabled reports whether the operator has configured a MySQL host.
// The whole MySQL/TecDoc path is skipped when this is false.
func (c *Config) MySQLEnabled() bool {
	return strings.TrimSpace(c.MySQLHost) != ""
}

// MySQLDSN builds a DSN suitable for the go-sql-driver/mysql driver.
// InterpolateParams is deliberately set to false — we want the driver to
// use server-side prepared statements. Every query in the codebase already
// uses parameterised placeholders, so this is defence-in-depth.
func (c *Config) MySQLDSN() string {
	cfg := mysql.NewConfig()
	cfg.User = c.MySQLUser
	cfg.Passwd = c.MySQLPassword
	cfg.Net = "tcp"
	cfg.Addr = net.JoinHostPort(c.MySQLHost, c.MySQLPort)
	cfg.DBName = c.MySQLDB
	cfg.ParseTime = true
	cfg.Params = map[string]string{"charset": "utf8mb4"}
	// See note above: keep InterpolateParams=false so the driver uses real
	// prepared statements. If throughput becomes an issue on batch loaders,
	// override on the specific caller, not globally.
	cfg.InterpolateParams = false
	return cfg.FormatDSN()
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// firstNonEmpty returns the first non-empty value from the given strings.
// Used to layer legacy env-var names underneath the new MYSQL_* namespace
// (see MySQL fields in Load()).
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// stripPort parses a legacy HOST="host:port" value and returns just the host.
// When the input has no colon, returns it unchanged.
func stripPort(hostPort string) string {
	hostPort = strings.TrimSpace(hostPort)
	if hostPort == "" {
		return ""
	}
	if h, _, err := net.SplitHostPort(hostPort); err == nil {
		return h
	}
	return hostPort
}

// portFrom extracts the port from a legacy HOST="host:port" value.
// Returns empty string when there is no port.
func portFrom(hostPort string) string {
	hostPort = strings.TrimSpace(hostPort)
	if hostPort == "" {
		return ""
	}
	if _, p, err := net.SplitHostPort(hostPort); err == nil {
		return p
	}
	return ""
}
