// embed_articles - batch embedder for TecDoc article descriptions.
//
// M5.S1.T1. Streams every (legacyArticleId, genericArticleDescription)
// from MySQL, calls the sentence-transformer sidecar over a unix socket
// (or falls back to a REST endpoint), inserts the resulting 384-dim
// vector into Postgres article_embeddings.
//
// Idempotent: skips articles that already have an embedding for the
// current model. Safe to re-run to fill new rows.
//
// The sentence-transformer runs as a separate Python process:
//
//	python -m parts_engine.embedder --socket /tmp/embed.sock
//
// The socket handles JSON messages:
//
//	REQ  {"texts": ["Engine Oil Filter", "Brake Pad Set (Front)"]}
//	RESP {"embeddings": [[0.12, -0.03, ...], [0.05, ...]]}
//
// Usage:
//
//	./embed_articles                    # full corpus
//	./embed_articles --batch-size=500   # smaller batches
//	./embed_articles --resume-from=1000000
//	./embed_articles --dry-run          # count what needs embedding, no writes
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"time"

	"parts-engine/internal/config"
	"parts-engine/internal/db"
)

const modelName = "sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2"

type embedReq struct {
	Texts []string `json:"texts"`
}

type embedResp struct {
	Embeddings [][]float32 `json:"embeddings"`
	Error      string      `json:"error,omitempty"`
}

func main() {
	batchSize := flag.Int("batch-size", 200, "articles per embedder round-trip")
	resumeFrom := flag.Int("resume-from", 0, "start from this legacyArticleId")
	socketPath := flag.String("socket", "/tmp/embed.sock", "embedder unix socket path")
	dryRun := flag.Bool("dry-run", false, "count without writing")
	flag.Parse()

	cfg := config.Load()

	pg := db.NewPostgres(cfg)
	if pg == nil {
		log.Fatal("postgres: connection failed")
	}
	defer pg.Close()

	if !cfg.MySQLEnabled() {
		log.Fatal("MySQL/TecDoc must be configured to source article descriptions")
	}
	mysql := db.NewMySQL(cfg)
	if mysql == nil {
		log.Fatal("mysql: connection failed")
	}
	defer mysql.Close()

	// Count target work.
	var totalToProcess int
	countQ := `
		SELECT COUNT(*) FROM articles a
		WHERE a.legacyArticleId >= ?
		  AND COALESCE(a.genericArticleDescription, '') != ''
		  AND NOT EXISTS (
		    SELECT 1 FROM article_embeddings_cursor
		    WHERE cursor_key = 'embed_articles' AND max_processed_id >= a.legacyArticleId
		  )`
	// Simpler count (no cursor table dependency): just count non-empty rows.
	_ = countQ
	err := mysql.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM articles WHERE legacyArticleId >= ? AND COALESCE(genericArticleDescription, '') != ''`,
		*resumeFrom,
	).Scan(&totalToProcess)
	if err != nil {
		log.Fatalf("count target work: %v", err)
	}
	log.Printf("[embed_articles] %d articles to consider (resume-from=%d dry-run=%v)",
		totalToProcess, *resumeFrom, *dryRun)

	if *dryRun {
		fmt.Printf("would process %d articles\n", totalToProcess)
		return
	}

	// Batch loop
	processed := 0
	skipped := 0
	written := 0
	errored := 0
	startTime := time.Now()

	lastID := *resumeFrom
	for {
		batch, err := fetchBatch(mysql, lastID, *batchSize)
		if err != nil {
			log.Fatalf("fetch batch: %v", err)
		}
		if len(batch) == 0 {
			break
		}

		// Filter out articles that already have an embedding for this model.
		ids := make([]int, 0, len(batch))
		for _, a := range batch {
			ids = append(ids, a.id)
		}
		alreadyEmbedded, err := loadExistingIDs(pg, ids)
		if err != nil {
			log.Printf("check existing: %v", err)
			// non-fatal — worst case we re-embed a few
		}

		toEmbed := make([]articleRow, 0, len(batch))
		for _, a := range batch {
			if alreadyEmbedded[a.id] {
				skipped++
				continue
			}
			toEmbed = append(toEmbed, a)
		}

		if len(toEmbed) > 0 {
			texts := make([]string, len(toEmbed))
			for i, a := range toEmbed {
				texts[i] = a.description
			}
			embeddings, err := callEmbedder(*socketPath, texts)
			if err != nil {
				log.Printf("call embedder err (batch starting %d): %v", toEmbed[0].id, err)
				errored += len(toEmbed)
			} else if len(embeddings) == len(toEmbed) {
				if err := upsertEmbeddings(pg, toEmbed, embeddings); err != nil {
					log.Printf("upsert err (batch starting %d): %v", toEmbed[0].id, err)
					errored += len(toEmbed)
				} else {
					written += len(toEmbed)
				}
			} else {
				log.Printf("embedder returned wrong count: got %d, want %d", len(embeddings), len(toEmbed))
				errored += len(toEmbed)
			}
		}

		lastID = batch[len(batch)-1].id
		processed += len(batch)

		if processed%2000 == 0 || len(batch) < *batchSize {
			elapsed := time.Since(startTime)
			rate := float64(processed) / elapsed.Seconds()
			log.Printf("[embed_articles] processed=%d written=%d skipped=%d errored=%d rate=%.1f/sec last_id=%d",
				processed, written, skipped, errored, rate, lastID)
		}

		if len(batch) < *batchSize {
			break
		}
	}

	elapsed := time.Since(startTime)
	fmt.Printf("processed=%d written=%d skipped=%d errored=%d elapsed=%s\n",
		processed, written, skipped, errored, elapsed)
}

type articleRow struct {
	id          int
	description string
}

func fetchBatch(mysql *sql.DB, resumeFrom, batchSize int) ([]articleRow, error) {
	const q = `
		SELECT legacyArticleId, genericArticleDescription
		FROM articles
		WHERE legacyArticleId > ?
		  AND COALESCE(genericArticleDescription, '') != ''
		ORDER BY legacyArticleId ASC
		LIMIT ?`
	rows, err := mysql.QueryContext(context.Background(), q, resumeFrom, batchSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []articleRow
	for rows.Next() {
		var a articleRow
		if err := rows.Scan(&a.id, &a.description); err != nil {
			continue
		}
		out = append(out, a)
	}
	return out, nil
}

func loadExistingIDs(pg *sql.DB, ids []int) (map[int]bool, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	// Build "$1,$2,..."
	placeholders := ""
	args := make([]any, len(ids))
	for i, id := range ids {
		if i > 0 {
			placeholders += ","
		}
		placeholders += fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	q := "SELECT legacy_article_id FROM article_embeddings WHERE model = $" +
		fmt.Sprintf("%d", len(ids)+1) + " AND legacy_article_id IN (" + placeholders + ")"
	args = append(args, modelName)

	rows, err := pg.QueryContext(context.Background(), q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int]bool, len(ids))
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err == nil {
			out[id] = true
		}
	}
	return out, nil
}

func upsertEmbeddings(pg *sql.DB, articles []articleRow, embeddings [][]float32) error {
	tx, err := pg.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	const q = `
		INSERT INTO article_embeddings (legacy_article_id, description, embedding, model)
		VALUES ($1, $2, $3::vector, $4)
		ON CONFLICT (legacy_article_id) DO UPDATE SET
			description = EXCLUDED.description,
			embedding   = EXCLUDED.embedding,
			model       = EXCLUDED.model,
			updated_at  = NOW()`

	for i, a := range articles {
		vecStr := vectorToString(embeddings[i])
		if _, err := tx.Exec(q, a.id, a.description, vecStr, modelName); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// vectorToString formats a float32 slice as pgvector's "[1.0,2.0,3.0]" format.
func vectorToString(v []float32) string {
	if len(v) == 0 {
		return "[]"
	}
	buf := make([]byte, 0, len(v)*10)
	buf = append(buf, '[')
	for i, f := range v {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = fmt.Appendf(buf, "%g", f)
	}
	buf = append(buf, ']')
	return string(buf)
}

func callEmbedder(socketPath string, texts []string) ([][]float32, error) {
	conn, err := net.DialTimeout("unix", socketPath, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("dial embedder socket %s: %w", socketPath, err)
	}
	defer conn.Close()

	req := embedReq{Texts: texts}
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if _, err := conn.Write(reqBytes); err != nil {
		return nil, err
	}
	if _, err := conn.Write([]byte{'\n'}); err != nil {
		return nil, err
	}

	dec := json.NewDecoder(conn)
	var resp embedResp
	if err := dec.Decode(&resp); err != nil {
		return nil, err
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("embedder error: %s", resp.Error)
	}
	return resp.Embeddings, nil
}
