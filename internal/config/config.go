package config

import (
	"net"
	"os"
	"strings"

	"github.com/go-sql-driver/mysql"
)

type Config struct {
	// MySQL (TecDoc / dev_ifritah)
	MySQLUser string
	MySQLPass string
	MySQLHost string
	MySQLPort string
	MySQLDB   string

	// Server
	ServerPort string
	BindAddr   string // e.g. "0.0.0.0" or "127.0.0.1"
	DataDir    string // path to data/ folder with SQLite DBs

	// CORS
	CORSOrigins []string // comma-separated allowed origins

	// Elasticsearch
	ElasticURL string

	// NHTSA
	NHTSABaseURL string
}

func Load() *Config {
	origins := envOr("CORS_ORIGINS", "http://localhost:5173,http://localhost:3000")
	return &Config{
		MySQLUser:    envOr("DBUSER", "root"),
		MySQLPass:    envOr("PASSWORD", ""),
		MySQLHost:    envOr("HOST", "127.0.0.1"),
		MySQLPort:    envOr("DBPORT", "3306"),
		MySQLDB:      envOr("DBNAME", "dev_ifritah"),
		ServerPort:   envOr("PORT", "8080"),
		BindAddr:     envOr("BIND_ADDR", "0.0.0.0"),
		DataDir:      envOr("DATA_DIR", ""),
		CORSOrigins:  splitCSV(origins),
		ElasticURL:   envOr("ELASTIC_URL", "http://localhost:9200"),
		NHTSABaseURL: envOr("NHTSA_URL", "https://vpic.nhtsa.dot.gov/api"),
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

func (c *Config) MySQLDSN() string {
	cfg := mysql.NewConfig()
	cfg.User = c.MySQLUser
	cfg.Passwd = c.MySQLPass
	cfg.Net = "tcp"
	cfg.Addr = net.JoinHostPort(c.MySQLHost, c.MySQLPort)
	cfg.DBName = c.MySQLDB
	cfg.ParseTime = true
	cfg.Params = map[string]string{"charset": "utf8mb4"}
	cfg.InterpolateParams = true
	return cfg.FormatDSN()
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
