package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"parts-engine/internal/config"
	"parts-engine/internal/db"
	"parts-engine/internal/enrich"
	"parts-engine/internal/handler"
	"parts-engine/internal/nhtsa"
	"parts-engine/internal/service"
)

func main() {
	cfg := config.Load()

	// Connect to MySQL (dev_ifritah) — non-fatal if unavailable
	mysql := db.NewMySQL(cfg)

	// Determine data directory: env override or next to executable
	var dataDir string
	if cfg.DataDir != "" {
		dataDir = cfg.DataDir
	} else {
		exe, _ := os.Executable()
		dataDir = filepath.Join(filepath.Dir(exe), "data")
	}
	// Determine active database: MySQL preferred, SQLite fallback for TecDoc queries
	var activeDB *sql.DB
	var offline bool
	if mysql != nil {
		activeDB = mysql
		defer mysql.Close()
	}

	// Always open SQLite for local data (aftermarket_crossref, parts_cache, dealer_parts_index)
	sqlitePath := filepath.Join(dataDir, "hk_parts.db")
	sqliteDB := db.NewSQLite(sqlitePath)
	if sqliteDB != nil {
		defer sqliteDB.Close()
		if activeDB == nil {
			// No MySQL — use SQLite for everything
			activeDB = sqliteDB
			offline = true
			log.Println("✓ Running in OFFLINE mode (SQLite)")
		} else {
			log.Println("✓ SQLite local DB loaded for aftermarket/cache data")
		}
	} else if activeDB == nil {
		log.Println("⚠ Running without any parts database — only NHTSA decode + recalls will work")
	}

	// Init NHTSA vPIC SQLite decoder (optional — falls back to WMI if unavailable)
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

	// Init vehicle enricher (EPA, OpenVehicleDB, Arthurkao, CarQuery, FuelEconomy APIs)
	vehicleEnricher := enrich.New(dataDir)
	defer vehicleEnricher.Close()

	// Init services
	vinDecoder := service.NewVINDecoder(cfg.NHTSABaseURL, nhtsaDec, vehicleEnricher)
	partsLookup := service.NewPartsLookup(activeDB, offline)
	oemLookup := service.NewOEMLookup(activeDB)
	supersession := service.NewSupersession(activeDB)
	platform := service.NewPlatform(activeDB)
	recalls := service.NewRecallsClient()
	crossRef := service.NewCrossRef(activeDB, offline)
	if sqliteDB != nil {
		crossRef.SetLocalDB(sqliteDB)
	}

	// Engine resolver: motorCode resolution from vehicle linkage (display only)
	var engineResolver *service.EngineResolver
	if mysql != nil {
		engineResolver = service.NewEngineResolver(mysql)
		log.Println("✓ Engine resolver initialized (display only)")
	}

	// Category tree: hierarchical part catalog navigation
	categoryTree := service.NewCategoryTree(activeDB, offline)

	// Alternatives: functional equivalents via same genericArticleDesc
	alternatives := service.NewAlternatives(activeDB, offline)

	// Online lookup: PartsOuq scraper with SQLite cache
	var partsCache *service.PartsCache
	var onlineLookup *service.PartsOuqService
	if sqliteDB != nil {
		partsCache = service.NewPartsCache(sqliteDB)
	}
	onlineLookup = service.NewPartsOuqService(partsCache)

	smartSearch := service.NewSmartSearch(activeDB, partsLookup, crossRef, oemLookup, platform, onlineLookup, offline)

	// Dealer site lookup: fallback for parts not in TecDoc or PartsOuq
	dealerLookup := service.NewDealerLookup(partsCache)
	smartSearch.SetDealerLookup(dealerLookup)

	// TecDoc full MySQL access (only when MySQL is connected)
	var tecdocH *handler.TecDocHandler
	var tecdoc *service.TecDoc
	if mysql != nil {
		tecdoc = service.NewTecDoc(mysql)
		if tecdoc != nil {
			smartSearch.SetTecDoc(tecdoc)
			tecdocH = handler.NewTecDocHandler(tecdoc)
			log.Println("✓ TecDoc full database connected — direct queries enabled")
			tecdoc.LogStats()
		}
	}

	vinCache := service.NewVINCache(1 * time.Hour)

	// Init handlers
	vinH := handler.NewVINHandler(vinDecoder, partsLookup, platform, recalls, vinCache, engineResolver)
	partsH := handler.NewPartsHandler(partsLookup, oemLookup)
	if engineResolver != nil {
		partsH.SetEngineResolver(engineResolver)
	}
	partsH.SetAlternatives(alternatives)
	partsH.SetCategoryTree(categoryTree)
	if tecdoc != nil {
		partsH.SetTecDoc(tecdoc)
	}
	oemH := handler.NewOEMHandler(oemLookup)
	oemH.SetCrossRef(crossRef)
	oemH.SetPartsLookup(partsLookup)
	superH := handler.NewSupersessionHandler(supersession)
	recallsH := handler.NewRecallsHandler(recalls)
	searchH := handler.NewSearchHandler(smartSearch)
	catalogH := handler.NewCatalogHandler(partsLookup, crossRef)
	platformH := handler.NewPlatformHandler(platform, partsLookup)

	// Router
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

	// API routes
	api := r.Group("/api")
	{
		api.POST("/vin/decode", vinH.Decode)
		api.GET("/vehicle/:id/parts", partsH.ByVehicle)
		api.GET("/vehicle/:id/categories", searchH.Categories)
		api.GET("/vehicle/:id/engine", partsH.Engine)
		api.GET("/vehicle/:id/tree", partsH.CategoryTree)
		api.GET("/vehicle/:id/platform", platformH.Siblings)
		api.GET("/oem/:number", oemH.Lookup)
		api.GET("/part/:id/chain", superH.GetChain)
		api.GET("/part/:id/vehicles", partsH.ReverseByArticle)
		api.GET("/part/:id/crossref", searchH.CrossRef)
		api.GET("/part/:id/alternatives", partsH.Alternatives)
		api.GET("/recalls", recallsH.ByVIN)
		api.GET("/search", searchH.Search)

		// Catalog browsing
		api.GET("/catalog/models", catalogH.Models)
		api.GET("/catalog/vehicles", catalogH.Vehicles)
		api.GET("/catalog/groups", catalogH.Groups)
		api.GET("/catalog/parts", catalogH.GroupParts)

		// TecDoc direct queries (only available when MySQL is connected)
		if tecdocH != nil {
			td := api.Group("/tecdoc")
			td.GET("/specs/:id", tecdocH.Specs)
			td.GET("/fitment", tecdocH.Fitment)
			td.GET("/groups", tecdocH.Groups)
			td.GET("/replacements/:id", tecdocH.Replacements)
			td.GET("/vehicle/:id/parts", tecdocH.VehicleParts)
			td.GET("/vehicle/:id/groups", tecdocH.VehicleGroups)
		}
	}

	// Health check
	r.GET("/health", func(c *gin.Context) {
		mode := "mysql"
		if offline {
			mode = "sqlite_offline"
		} else if activeDB == nil {
			mode = "no_database"
		}
		hasTecDoc := tecdocH != nil
		c.JSON(200, gin.H{"status": "ok", "mode": mode, "tecdoc": hasTecDoc})
	})

	// Serve frontend SPA (built dist/) if present
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
