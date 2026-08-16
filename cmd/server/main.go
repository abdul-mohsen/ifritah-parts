package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
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

	pg := db.NewPostgres(cfg)
	var dataDBAvailable bool
	if pg != nil {
		dataDBAvailable = true
		defer pg.Close()
	} else {
		log.Println("⚠ Running without PostgreSQL application data — only NHTSA decode + recalls will work")
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

	vinCache := service.NewVINCache(1 * time.Hour)

	vinH := handler.NewVINHandler(vinDecoder, partsLookup, platform, recalls, vinCache)
	partsH := handler.NewPartsHandler(partsLookup, oemLookup)
	partsH.SetCrossRef(crossRef)
	partsH.SetAlternatives(alternatives)
	partsH.SetCategoryTree(categoryTree)
	partsH.SetPlacementAdvisor(placementAdvisor)
	partsH.SetReplacementAdvisor(replacementAdvisor)
	oemH := handler.NewOEMHandler(oemLookup)
	oemH.SetCrossRef(crossRef)
	oemH.SetPartsLookup(partsLookup)
	superH := handler.NewSupersessionHandler(supersession)
	recallsH := handler.NewRecallsHandler(recalls)
	searchH := handler.NewSearchHandler(smartSearch)
	catalogH := handler.NewCatalogHandler(partsLookup, crossRef)
	platformH := handler.NewPlatformHandler(platform, partsLookup)
	workerH := handler.NewWorkerHandler(workerStore)
	commonsMediaH := handler.NewCommonsMediaHandler(commonsMediaStore)

	r := gin.Default()
	r.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.CORSOrigins,
		AllowMethods:     []string{"GET", "POST", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept"},
		AllowCredentials: true,
	}))

	// internalAuth guards /api/internal/* routes with a static bearer token.
	// Set INTERNAL_API_KEY in the environment. When the key is empty the
	// middleware disables all internal routes entirely (503).
	internalAuth := func(c *gin.Context) {
		if cfg.InternalAPIKey == "" {
			c.AbortWithStatusJSON(503, gin.H{"error": "internal API not configured"})
			return
		}
		auth := c.GetHeader("Authorization")
		if auth != "Bearer "+cfg.InternalAPIKey {
			c.AbortWithStatusJSON(401, gin.H{"error": "unauthorized"})
			return
		}
		c.Next()
	}

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
		api.GET("/part/:id/detail", partsH.Detail)
		api.GET("/part/:id/vehicles", partsH.ReverseByArticle)
		api.GET("/part/:id/crossref", searchH.CrossRef)
		api.GET("/part/:id/alternatives", partsH.Alternatives)
		api.GET("/recalls", recallsH.ByVIN)
		api.GET("/search", searchH.Search)

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
		c.JSON(200, gin.H{"status": "ok", "mode": mode, "tecdoc": false})
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
			c.File(filepath.Join(frontendDir, "index.html"))
		})
		log.Printf("✓ Serving frontend from %s", frontendDir)
	}

	addr := fmt.Sprintf("%s:%s", cfg.BindAddr, cfg.ServerPort)
	log.Printf("Parts Engine starting on %s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
