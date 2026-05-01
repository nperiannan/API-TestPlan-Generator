package web

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

//go:embed static/*
var staticFS embed.FS

// Server holds the web server state
type Server struct {
	router  *gin.Engine
	store   *MinIOStore
	config  *DashboardConfig
	version *VersionManager
}

// DashboardConfig holds server configuration
type DashboardConfig struct {
	Port           int
	MinIOEndpoint  string
	MinIOAccessKey string
	MinIOSecretKey string
	MinIOBucket    string
	MinIOUseSSL    bool
	TestPlansDir   string
	ReportsDir     string
	SourcesDir     string
}

// DefaultConfig returns a default configuration
func DefaultConfig() *DashboardConfig {
	return &DashboardConfig{
		Port:           8080,
		MinIOEndpoint:  "localhost:9000",
		MinIOAccessKey: "minioadmin",
		MinIOSecretKey: "minioadmin123",
		MinIOBucket:    "testplans",
		MinIOUseSSL:    false,
		TestPlansDir:   "./Testplans",
		ReportsDir:     "./reports",
		SourcesDir:     "./sources",
	}
}

// NewServer creates and initializes the web server
func NewServer(cfg *DashboardConfig) (*Server, error) {
	store, err := NewMinIOStore(cfg.MinIOEndpoint, cfg.MinIOAccessKey, cfg.MinIOSecretKey, cfg.MinIOBucket, cfg.MinIOUseSSL)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize MinIO store: %w", err)
	}

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(gin.Logger())
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept"},
		ExposeHeaders:    []string{"Content-Length", "Content-Disposition"},
		AllowCredentials: false,
		MaxAge:           12 * time.Hour,
	}))

	s := &Server{
		router:  router,
		store:   store,
		config:  cfg,
		version: NewVersionManager(store),
	}

	s.registerRoutes()
	return s, nil
}

func (s *Server) registerRoutes() {
	// Serve embedded static files
	staticSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatalf("failed to get static sub FS: %v", err)
	}

	// Parse embedded templates
	tmpl, err := template.ParseFS(staticFS, "static/index.html")
	if err != nil {
		log.Fatalf("failed to parse templates: %v", err)
	}
	s.router.SetHTMLTemplate(tmpl)

	// Static assets
	s.router.StaticFS("/static", http.FS(staticSub))

	// Dashboard page
	s.router.GET("/", s.handleDashboard)

	// API routes
	api := s.router.Group("/api")
	{
		// Versions
		api.GET("/versions", s.handleListVersions)
		api.POST("/versions", s.handleCreateVersion)
		api.GET("/versions/:id", s.handleGetVersion)
		api.GET("/versions/:id/diff/:otherId", s.handleDiffVersions)

		// Test plans
		api.GET("/testplans", s.handleListTestPlans)
		api.GET("/testplans/:category/:feature", s.handleGetTestPlan)
		api.GET("/testplans/:category/:feature/download", s.handleDownloadTestPlan)

		// Summary
		api.GET("/summary", s.handleGetSummary)
		api.GET("/summary/report", s.handleGetSummaryReport)

		// Source changes
		api.GET("/sources/changes", s.handleSourceChanges)

		// Generation
		api.POST("/generate", s.handleGenerate)
	}
}

// Run starts the web server
func (s *Server) Run() error {
	addr := fmt.Sprintf(":%d", s.config.Port)
	log.Printf("Dashboard server starting on http://0.0.0.0%s", addr)
	return s.router.Run(addr)
}
