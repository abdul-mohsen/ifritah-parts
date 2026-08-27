package main

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"

	"parts-engine/db/migrations"
	"parts-engine/internal/config"
	"parts-engine/internal/db"
	dbg "parts-engine/internal/debug"
	"parts-engine/internal/enrich"
	"parts-engine/internal/handler"
	"parts-engine/internal/middleware"
	"parts-engine/internal/nhtsa"
	"parts-engine/internal/service"
)

func main() {
	// Auto-load .env from the current working directory (and its parents up to
	// the module root) BEFORE config.Load reads env vars. Missing .env is not
	// an error — the app boots off pure OS environment variables in that case.
	// This matches how the docs describe the run recipe: "edit .env, run
	// ./server" — without godotenv the Go binary would ignore .env entirely
	// (unlike Node/Python which auto-load).
	for _, candidate := range []string{".env", "../.env", "../../.env"} {
		if _, err := os.Stat(candidate); err == nil {
			if err := godotenv.Load(candidate); err == nil {
				log.Printf("✓ Loaded env from %s", candidate)
				break
			}
		}
	}

	cfg := config.Load()

	// Debug logger — tees every log.Printf into a ring buffer + SSE broadcast.
	// Only active when DEBUG_LOGS=1. In production this is nil and the normal
	// log.SetOutput(os.Stderr) default applies.
	var debugLogger *dbg.Logger
	if cfg.DebugLogs {
		debugLogger = dbg.New(os.Stderr, 500)
		log.SetOutput(debugLogger)
		log.SetFlags(log.LstdFlags)
		log.Println("⚡ DEBUG_LOGS=1 — /api/debug/logs SSE endpoint is active")
	}

	pg := db.NewPostgres(cfg)
	var dataDBAvailable bool
	if pg != nil {
		dataDBAvailable = true
		defer pg.Close()

		// App-managed migrations — replaces the postgres initdb hook so that
		// migrations added after the initial deploy still get applied on the
		// next container start. Idempotent, advisory-locked, safe across
		// multi-replica deployments. Every migration file must be idempotent
		// (all existing files use CREATE TABLE IF NOT EXISTS + ON CONFLICT
		// DO NOTHING, which is enforced by the tests in internal/db/*_test.go).
		if err := db.RunMigrations(pg, migrations.FS, "."); err != nil {
			log.Fatalf("✗ migrations failed — refusing to start with a stale schema: %v", err)
		}
	} else {
		log.Println("⚠ Running without PostgreSQL application data — only NHTSA decode + recalls will work")
	}

	// Optional MySQL/TecDoc connection. Skipped when MYSQL_HOST is empty (the
	// default for local dev + CI). When enabled, the TecDoc service adapter is
	// initialised and the SmartSearch cascade gains its 21.5M-row cross-reference
	// index. See internal/service/tecdoc.go for the actual query surface.
	mysql := db.NewMySQL(cfg)
	if mysql != nil {
		defer mysql.Close()
	}

	var dataDir string
	if cfg.DataDir != "" {
		dataDir = cfg.DataDir
	} else {
		exe, _ := os.Executable()
		dataDir = filepath.Join(filepath.Dir(exe), "data")
	}

	if pg != nil {
		externalStore := service.NewExternalSourceStore(pg)
		if externalStore != nil {
			if err := externalStore.SeedCatalog(service.DefaultExternalSourceCatalog(), service.DefaultExternalSourceAssessments()); err != nil {
				log.Printf("⚠ External source registry seed error (non-fatal): %v", err)
			} else if counts, err := externalStore.CountSourcesByRecommendation(); err != nil {
				log.Printf("⚠ External source registry count error (non-fatal): %v", err)
			} else {
				log.Printf("✓ External source registry ready: backend=%d research=%d rejected=%d",
					counts["backend_enrichment"], counts["research_only"], counts["rejected"])
			}
		}
	}

	var nhtsaDec *nhtsa.Decoder
	vpicPath := filepath.Join(dataDir, "vpic.lite.db")
	if _, err := os.Stat(vpicPath); err == nil {
		nhtsaDec, err = nhtsa.NewDecoder(vpicPath)
		if err != nil {
			log.Printf("⚠ NHTSA vPIC DB failed to load: %v (falling back to WMI)", err)
		} else {
			log.Printf("✓ NHTSA vPIC DB loaded from %s", vpicPath)
			defer nhtsaDec.Close()
		}
	} else {
		log.Println("⚠ NHTSA vPIC DB not found at", vpicPath, "(falling back to WMI)")
	}

	vehicleEnricher := enrich.New(dataDir)
	defer vehicleEnricher.Close()

	workerStore, err := service.NewWorkerStore(dataDir)
	if err != nil {
		log.Printf("⚠ Worker contribution store failed to load: %v", err)
	} else {
		defer workerStore.Close()
		log.Printf("✓ Worker contribution store ready: %s", filepath.Join(dataDir, "worker_contributions.db"))
	}

	vinDecoder := service.NewVINDecoder(cfg.NHTSABaseURL, nhtsaDec, vehicleEnricher)
	partsLookup := service.NewPartsLookup(pg, false)
	oemLookup := service.NewOEMLookup(pg)
	supersession := service.NewSupersession(pg)
	relatedParts := service.NewRelatedParts(pg)
	feedback := service.NewFeedbackService(pg)
	communityContrib := service.NewCommunityContribService(pg)

	// Semantic search (optional) — only wire when EMBEDDER_SOCKET is set.
	// When empty the /api/search/semantic endpoint returns 503 gracefully.
	var semanticSvc *service.SemanticSearch
	if sock := os.Getenv("EMBEDDER_SOCKET"); sock != "" {
		semanticSvc = service.NewSemanticSearch(pg, service.NewUnixSocketEmbedder(sock))
	}
	platform := service.NewPlatform(pg)
	recalls := service.NewRecallsClient(cfg.NHTSARecallsURL)
	crossRef := service.NewCrossRef(pg, false)
	categoryTree := service.NewCategoryTree(pg, false)
	alternatives := service.NewAlternatives(pg, false)
	placementAdvisor := service.NewPlacementAdvisor(pg)
	replacementAdvisor := service.NewReplacementAdvisor(crossRef, partsLookup, alternatives)
	commonsMediaStore := service.NewCommonsMediaStore(pg)

	var partsCache *service.PartsCache
	var onlineLookup *service.PartsOuqService
	onlineLookup = service.NewPartsOuqService(partsCache)
	smartSearch := service.NewSmartSearch(pg, partsLookup, crossRef, oemLookup, platform, onlineLookup, false)

	dealerLookup := service.NewDealerLookup(partsCache)
	smartSearch.SetDealerLookup(dealerLookup)

	// Phase 1 (2026-08-17): prefix + chassis-code inference. Requires the
	// Postgres tables from migration 000011 (hk_oem_prefix_map,
	// hk_chassis_code_map, hk_variant_suffix_map). Seeded with hand-curated
	// baseline; optionally enriched by `go run ./scripts/derive_hk_maps`
	// which clusters TecDoc data. Zero external calls at query time.
	if pg != nil {
		prefixInf := service.NewPrefixInference(pg)
		smartSearch.SetPrefixInference(prefixInf)
		log.Printf("✓ Prefix-inference strategy wired (Postgres migration 000011)")
	}

	// Phase 2 (2026-08-17): persistent OEM resolution cache. Requires
	// Postgres migration 000012 (oem_resolution_cache). Every successful
	// resolution — from TecDoc, dealer_lookup, partsouq, prefix inference,
	// or any future source — writes here. Second-visit reads hit in <100 ms.
	if pg != nil {
		oemCache := service.NewOEMCache(pg)
		smartSearch.SetOEMCache(oemCache)
		log.Printf("✓ Persistent OEM cache wired (Postgres migration 000012)")
	}

	// Auto-start the TecDoc-derive background worker. Runs immediately (with
	// a 3s jitter) if the 30-day cadence has elapsed since the last successful
	// derive, then monthly thereafter. Uses a Postgres advisory lock so a
	// multi-replica deploy still runs derive at most once per cadence window.
	//
	// When MySQL is unreachable, the worker no-ops silently — the hand-curated
	// seed baseline from migration 000011 stays in place and the app keeps
	// working. This is why we never call log.Fatal from here.
	//
	// Operators can still force an immediate derive from a CLI via
	// `go run ./scripts/derive_hk_maps --force` — that CLI acquires the same
	// advisory lock so it's mutually exclusive with the background worker.
	if pg != nil {
		deriveWorker := service.NewDeriveWorker(pg, mysql)
		deriveWorker.Start(context.Background())
		log.Printf("✓ TecDoc derive worker started (monthly cadence, advisory-locked)")
	}

	// When MySQL is connected, wire the full TecDoc reader into SmartSearch.
	// The SmartSearch cascade uses TecDoc as an early-hit strategy for OEM
	// searches that miss the local Postgres cache — the source-of-truth for
	// the 21.5M-row oem_number and 651M-row articlesvehicletrees data.
	var tecdocEnabled bool
	var tecdoc *service.TecDoc
	if mysql != nil {
		tecdoc = service.NewTecDoc(mysql)
		if tecdoc != nil {
			// ─── M8: wire the online-search dispatcher onto TecDoc ─────
			// FindAftermarketForOEM will fan out to the online sources
			// alongside its 3 TecDoc paths (articlecrosses / oem_number /
			// oem_search_index). Cache-first read via Postgres
			// aftermarket_online_cache; on miss the dispatcher parallel-
			// queries every enabled source.
			//
			// Every source is ENABLED by default. To disable a specific
			// source without a redeploy set ONLINE_<NAME>_ENABLED=false.
			// Global kill switch: ONLINE_SEARCH_ENABLED=false disables
			// the whole subsystem in one env var.
			cacheRepo := service.NewAftermarketOnlineCacheRepo(pg)
			robots := service.NewRobotsGuard(nil)
			httpClient := &http.Client{Timeout: 8 * time.Second}
			sources := append(
				[]service.OnlineSource{service.NewEbayFinder(nil)},
				service.AllG5Adapters(httpClient, robots)...,
			)
			online := service.NewOnlineSearch(cacheRepo, sources...)
			tecdoc = tecdoc.WithOnlineSearch(online)
			log.Printf("✓ Online-search dispatcher wired: %d sources (eBay + G5), all enabled by default; disable any via ONLINE_<SOURCE>_ENABLED=false", len(sources))

			smartSearch.SetTecDoc(tecdoc)
			// S2: articlecrosses cross-reference (30M rows)
			smartSearch.SetTecDocCrossRef(service.NewTecDocCrossRef(mysql))
			// S3: enrichment pipeline services
			smartSearch.SetTecDocSpecifications(service.NewTecDocSpecifications(mysql))
			smartSearch.SetTecDocDocuments(service.NewTecDocDocuments(mysql))
			smartSearch.SetTecDocSupersession(service.NewTecDocSupersession(mysql))
			smartSearch.SetTecDocFunctional(service.NewTecDocFunctional(mysql))
			smartSearch.SetTecDocVehicle(service.NewTecDocVehicle(mysql))
			tecdocEnabled = true
			log.Println("✓ TecDoc reader attached to SmartSearch (OEM + crossref + specs + docs + supersession + functional + vehicle)")
			tecdoc.LogStats()
		}
	}

	vinCache := service.NewVINCache(1 * time.Hour)

	vinH := handler.NewVINHandler(vinDecoder, partsLookup, platform, recalls, vinCache)
	partsH := handler.NewPartsHandler(partsLookup, oemLookup)
	partsH.SetCrossRef(crossRef)
	partsH.SetAlternatives(alternatives)
	partsH.SetCategoryTree(categoryTree)
	partsH.SetPlacementAdvisor(placementAdvisor)
	partsH.SetReplacementAdvisor(replacementAdvisor)
	// TecDoc-first for /api/vehicle/:id/parts + /api/part/:id/detail when MySQL is connected.
	if tecdoc != nil {
		partsH.SetTecDoc(tecdoc)
	}
	oemH := handler.NewOEMHandler(oemLookup)
	oemH.SetCrossRef(crossRef)
	oemH.SetPartsLookup(partsLookup)
	superH := handler.NewSupersessionHandler(supersession)
	relatedPartsH := handler.NewRelatedPartsHandler(relatedParts)
	vinPartsH := handler.NewVINPartsHandler(vinDecoder, tecdoc)
	feedbackH := handler.NewFeedbackHandler(feedback)
	// Admin gate for /api/feedback/weekly + /api/feedback/disputed uses the
	// same ADMIN_AUTH_TOKEN env var already consumed by CommunityContribHandler.
	// Empty token = dev mode (no gate); set in prod so aggregate reports
	// aren't world-readable.
	adminTok := os.Getenv("ADMIN_AUTH_TOKEN")
	if adminTok != "" {
		feedbackH.SetAdminAuthToken(adminTok)
	}
	contribH := handler.NewCommunityContribHandler(communityContrib)
	if adminTok != "" {
		contribH.SetAdminAuthToken(adminTok)
	}
	semanticH := handler.NewSemanticSearchHandler(semanticSvc)
	recallsH := handler.NewRecallsHandler(recalls)
	searchH := handler.NewSearchHandler(smartSearch)
	catalogH := handler.NewCatalogHandler(partsLookup, crossRef)
	platformH := handler.NewPlatformHandler(platform, partsLookup)
	workerH := handler.NewWorkerHandler(workerStore)
	commonsMediaH := handler.NewCommonsMediaHandler(commonsMediaStore)

	r := gin.Default()

	// CORS — allow configured origins. A wildcard origin combined with
	// AllowCredentials is spec-violating and browser-rejected; refuse to
	// enable credentials in that case and log the misconfiguration.
	corsAllowCredentials := true
	for _, o := range cfg.CORSOrigins {
		if o == "*" {
			corsAllowCredentials = false
			log.Printf("⚠ CORS_ORIGINS contains '*' — refusing to enable AllowCredentials (unsafe). Set explicit origins in prod.")
			break
		}
	}
	r.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.CORSOrigins,
		AllowMethods:     []string{"GET", "POST", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept"},
		AllowCredentials: corsAllowCredentials,
	}))

	// internalAuth guards /api/internal/* routes with a static bearer token.
	// Set INTERNAL_API_KEY in the environment. When the key is empty the
	// middleware disables all internal routes entirely (503).
	//
	// Token comparison uses crypto/subtle.ConstantTimeCompare so per-byte
	// timing does not leak the secret to an attacker measuring latency.
	expectedAuth := []byte("Bearer " + cfg.InternalAPIKey)
	internalAuth := func(c *gin.Context) {
		if cfg.InternalAPIKey == "" {
			c.AbortWithStatusJSON(503, gin.H{"error": "internal API not configured"})
			return
		}
		provided := []byte(c.GetHeader("Authorization"))
		if subtle.ConstantTimeCompare(provided, expectedAuth) != 1 {
			c.AbortWithStatusJSON(401, gin.H{"error": "unauthorized"})
			return
		}
		c.Next()
	}

	api := r.Group("/api")
	{
		api.POST("/vin/decode", vinH.Decode)
		api.GET("/vin/:vin/parts", vinPartsH.Get)
		api.GET("/vehicle/:id/parts", partsH.ByVehicle)
		api.GET("/vehicle/:id/categories", searchH.Categories)
		api.GET("/vehicle/:id/engine", partsH.Engine)
		api.GET("/vehicle/:id/tree", partsH.CategoryTree)
		api.GET("/vehicle/:id/platform", platformH.Siblings)
		api.GET("/oem/:number", oemH.Lookup)
		api.GET("/part/:id/chain", superH.GetChain)
		api.GET("/part/:id/detail", partsH.Detail)
		api.GET("/part/:id/vehicles", partsH.ReverseByArticle)
		api.GET("/part/:id/crossref", searchH.CrossRef)
		api.GET("/part/:id/alternatives", partsH.Alternatives)
		api.GET("/parts/related", relatedPartsH.Get)
		// M6.S2.T1 — privacy-preserving feedback API. Routes live under
		// /api/feedback (no /search/ prefix). The Submit endpoint is
		// public + rate-limited (60 rpm / IP, burst 10, enforced inside
		// the handler); the two aggregate endpoints are admin-gated via
		// ADMIN_AUTH_TOKEN. Frontend widget is delivered as a follow-up
		// task; NO other route registers a feedback handler.
		api.POST("/feedback", feedbackH.Submit)
		api.GET("/feedback/weekly", feedbackH.Weekly)
		api.GET("/feedback/disputed", feedbackH.Disputed)
		api.POST("/aftermarket/contribute", contribH.Submit)
		api.GET("/admin/moderation/pending", contribH.ListPending)
		api.POST("/admin/moderation/:id/review", contribH.Review)
		api.GET("/search/semantic", semanticH.Get)
		api.GET("/recalls", recallsH.ByVIN)
		// Rate-limited: 100 requests/min sustained, burst 20 per client IP.
		// Applied only to /api/search (not /search/modes which is cheap).
		searchRL := middleware.NewRateLimiter(100, 20)
		api.GET("/search", searchRL.Middleware(), searchH.Search)
		api.GET("/search/stream", searchRL.Middleware(), searchH.SearchStream)
		api.GET("/search/modes", searchH.Modes)

		// /api/debug/logs — SSE log stream. Only registered when DEBUG_LOGS=1.
		// No auth gate: in dev you want logs immediately without setup friction.
		if cfg.DebugLogs && debugLogger != nil {
			debugH := handler.NewDebugHandler(debugLogger)
			api.GET("/debug/logs", debugH.LogStream)
			log.Printf("✓ Debug log stream active at GET /api/debug/logs")
		}

		api.GET("/catalog/models", catalogH.Models)
		api.GET("/catalog/vehicles", catalogH.Vehicles)
		api.GET("/catalog/groups", catalogH.Groups)
		api.GET("/catalog/parts", catalogH.GroupParts)

		internal := api.Group("/internal/worker", internalAuth)
		{
			internal.POST("/replacements", workerH.SubmitReplacement)
			internal.GET("/replacements", workerH.ListReplacements)
			internal.POST("/replacements/:id/review", workerH.ReviewReplacement)
		}
		media := api.Group("/internal/media/commons", internalAuth)
		{
			media.POST("", commonsMediaH.Submit)
			media.GET("", commonsMediaH.List)
			media.POST("/:id/review", commonsMediaH.Review)
		}
	}

	r.GET("/health", func(c *gin.Context) {
		mode := "postgres"
		if !dataDBAvailable {
			mode = "no_database"
		}
		c.JSON(200, gin.H{"status": "ok", "mode": mode, "tecdoc": tecdocEnabled})
	})

	frontendDir := os.Getenv("FRONTEND_DIR")
	if frontendDir == "" {
		frontendDir = filepath.Join(dataDir, "..", "frontend", "dist")
	}
	if _, err := os.Stat(frontendDir); err == nil {
		r.Static("/assets", filepath.Join(frontendDir, "assets"))
		r.StaticFile("/favicon.svg", filepath.Join(frontendDir, "favicon.svg"))
		r.StaticFile("/icons.svg", filepath.Join(frontendDir, "icons.svg"))
		r.NoRoute(func(c *gin.Context) {
			// Never let /api/* fall through to the SPA — a JSON client
			// expects a JSON 404, not an HTML dump. Only serve index.html
			// for non-API routes so React Router can pick them up.
			if strings.HasPrefix(c.Request.URL.Path, "/api/") {
				c.JSON(404, gin.H{
					"error":  "not_found",
					"path":   c.Request.URL.Path,
					"method": c.Request.Method,
				})
				return
			}
			c.File(filepath.Join(frontendDir, "index.html"))
		})
		log.Printf("✓ Serving frontend from %s", frontendDir)
	} else {
		// No frontend built — still make /api/* return JSON 404s.
		r.NoRoute(func(c *gin.Context) {
			c.JSON(404, gin.H{
				"error":  "not_found",
				"path":   c.Request.URL.Path,
				"method": c.Request.Method,
			})
		})
	}

	addr := fmt.Sprintf("%s:%s", cfg.BindAddr, cfg.ServerPort)
	log.Printf("Parts Engine starting on %s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
