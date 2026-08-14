package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"parts-engine/internal/config"
	pedb "parts-engine/internal/db"
)

const batchSize = 2000

type partDoc struct {
	LinkageTargetID int    `json:"linkage_target_id"`
	LegacyArticleID int    `json:"legacy_article_id"`
	ArticleNumber   string `json:"article_number"`
	BrandName       string `json:"brand_name"`
	Description     string `json:"description"`
	Category        string `json:"category"`
	AssemblyGroupID int    `json:"assembly_group_id"`
	ModelSeriesName string `json:"model_series_name"`
	ManufacturerID  int    `json:"manufacturer_id"`
}

type oemDoc struct {
	RawNumber     string `json:"raw_number"`
	Normalized    string `json:"normalized"`
	Source        string `json:"source"`
	ArticleID     int    `json:"article_id"`
	ArticleNumber string `json:"article_number"`
	BrandName     string `json:"brand_name"`
	Description   string `json:"description"`
}

func main() {
	cfg := config.Load()
	pool := pedb.NewMySQL(cfg)
	defer pool.Close()

	esURL := cfg.ElasticURL
	if esURL == "" {
		esURL = "http://localhost:9200"
	}

	if len(os.Args) < 2 {
		fmt.Println("Usage: indexer <parts|oem|all>")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "parts":
		indexParts(pool, esURL)
	case "oem":
		indexOEM(pool, esURL)
	case "all":
		indexParts(pool, esURL)
		indexOEM(pool, esURL)
	default:
		fmt.Println("Usage: indexer <parts|oem|all>")
		os.Exit(1)
	}
}

func createIndex(esURL, name, mapping string) {
	req, _ := http.NewRequest("PUT", esURL+"/"+name, bytes.NewBufferString(mapping))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatalf("create index %s: %v", name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 400 {
		log.Printf("index %s already exists, continuing", name)
		return
	}
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		log.Fatalf("create index %s: %d %s", name, resp.StatusCode, body)
	}
	log.Printf("created index %s", name)
}

func bulkSend(esURL, index string, buf *bytes.Buffer) int {
	if buf.Len() == 0 {
		return 0
	}
	req, _ := http.NewRequest("POST", esURL+"/_bulk", buf)
	req.Header.Set("Content-Type", "application/x-ndjson")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatalf("bulk: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		log.Fatalf("bulk %d: %s", resp.StatusCode, body)
	}
	return 1
}

func indexParts(pool *sql.DB, esURL string) {
	mapping := `{
		"settings": {"number_of_shards": 1, "number_of_replicas": 0},
		"mappings": {
			"properties": {
				"article_number": {"type": "keyword"},
				"brand_name":     {"type": "keyword"},
				"description":    {"type": "text", "analyzer": "standard"},
				"category":       {"type": "text"},
				"model_series_name": {"type": "keyword"},
				"manufacturer_id":   {"type": "integer"},
				"linkage_target_id": {"type": "integer"},
				"legacy_article_id": {"type": "integer"},
				"assembly_group_id": {"type": "integer"}
			}
		}
	}`
	createIndex(esURL, "parts", mapping)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	rows, err := pool.QueryContext(ctx, `
		SELECT linkageTargetId, legacyArticleId, articleNumber,
		       brandName, description, category,
		       assemblyGroupNodeId, modelSeriesName, manuId
		FROM hk_parts_cache
	`)
	if err != nil {
		log.Fatalf("query parts: %v", err)
	}
	defer rows.Close()

	var buf bytes.Buffer
	total, batch := 0, 0

	for rows.Next() {
		var d partDoc
		if err := rows.Scan(&d.LinkageTargetID, &d.LegacyArticleID, &d.ArticleNumber,
			&d.BrandName, &d.Description, &d.Category,
			&d.AssemblyGroupID, &d.ModelSeriesName, &d.ManufacturerID); err != nil {
			log.Fatalf("scan: %v", err)
		}

		meta := fmt.Sprintf(`{"index":{"_index":"parts","_id":"%d_%d"}}`, d.LinkageTargetID, d.LegacyArticleID)
		doc, _ := json.Marshal(d)
		buf.WriteString(meta)
		buf.WriteByte('\n')
		buf.Write(doc)
		buf.WriteByte('\n')

		batch++
		total++
		if batch >= batchSize {
			bulkSend(esURL, "parts", &buf)
			buf.Reset()
			batch = 0
			if total%50000 == 0 {
				log.Printf("parts: indexed %d docs", total)
			}
		}
	}
	bulkSend(esURL, "parts", &buf)
	log.Printf("parts: done — %d docs indexed", total)
}

func indexOEM(pool *sql.DB, esURL string) {
	mapping := `{
		"settings": {"number_of_shards": 1, "number_of_replicas": 0},
		"mappings": {
			"properties": {
				"raw_number":     {"type": "keyword"},
				"normalized":     {"type": "keyword"},
				"source":         {"type": "keyword"},
				"article_id":     {"type": "integer"},
				"article_number": {"type": "keyword"},
				"brand_name":     {"type": "keyword"},
				"description":    {"type": "text"}
			}
		}
	}`
	createIndex(esURL, "oem", mapping)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	rows, err := pool.QueryContext(ctx, `
		SELECT raw_number, normalized, source,
		       article_id, article_number, brand_name, description
		FROM oem_search_index LIMIT 5000000
	`)
	if err != nil {
		log.Fatalf("query oem: %v", err)
	}
	defer rows.Close()

	var buf bytes.Buffer
	total, batch := 0, 0

	for rows.Next() {
		var d oemDoc
		if err := rows.Scan(&d.RawNumber, &d.Normalized, &d.Source,
			&d.ArticleID, &d.ArticleNumber, &d.BrandName, &d.Description); err != nil {
			log.Fatalf("scan: %v", err)
		}

		meta := fmt.Sprintf(`{"index":{"_index":"oem","_id":"%s_%d"}}`, d.Normalized, d.ArticleID)
		doc, _ := json.Marshal(d)
		buf.WriteString(meta)
		buf.WriteByte('\n')
		buf.Write(doc)
		buf.WriteByte('\n')

		batch++
		total++
		if batch >= batchSize {
			bulkSend(esURL, "oem", &buf)
			buf.Reset()
			batch = 0
			if total%50000 == 0 {
				log.Printf("oem: indexed %d docs", total)
			}
		}
	}
	bulkSend(esURL, "oem", &buf)
	log.Printf("oem: done — %d docs indexed", total)
}
