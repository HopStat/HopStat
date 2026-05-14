package server

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourorg/lg-looking-glass/internal/bgp"
	"github.com/yourorg/lg-looking-glass/internal/config"
	"github.com/yourorg/lg-looking-glass/internal/geo"
	"github.com/yourorg/lg-looking-glass/internal/server/handler"
	"github.com/yourorg/lg-looking-glass/internal/server/middleware"
	"github.com/yourorg/lg-looking-glass/internal/updater"
)

type Server struct {
	cfg     *config.Config
	db      *sql.DB
	geoDB   *geo.GeoIPDB
	bgpMgr  *bgp.SessionManager
	router  *gin.Engine
	distFS  fs.FS
	updater *updater.Updater
}

func New(cfg *config.Config, db *sql.DB, geoDB *geo.GeoIPDB, distFS fs.FS, bgpMgr *bgp.SessionManager, version string) *Server {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.Logger())
	router.SetTrustedProxies(nil)
	router.Use(middleware.CORS(nil))
	router.Use(func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "0")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self'; connect-src 'self'; frame-ancestors 'none'")
		c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		c.Next()
	})

	// Limit request body to 2MB
	router.MaxMultipartMemory = 2 << 20

	srv := &Server{
		cfg:    cfg,
		db:     db,
		geoDB:  geoDB,
		bgpMgr: bgpMgr,
		router: router,
		distFS: distFS,
	}

	if cfg.Update.Enabled && cfg.Update.GithubRepo != "" {
		srv.updater = updater.New(cfg.Update.GithubRepo, version)
	}

	srv.setupRoutes()
	return srv
}

func (s *Server) setupRoutes() {
	r := s.router

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "mode": "server"})
	})

	// Public API routes
	public := r.Group("/api/v1")
	public.Use(middleware.RateLimit(s.cfg.Security.RateLimitPerMin))
	{
		public.GET("/nodes", handler.ListNodes(s.db))
		public.GET("/nodes/:id", handler.GetNode(s.db))
		public.POST("/query", handler.SubmitQuery(s.db, s.cfg, s.geoDB))
		public.GET("/query/:id", handler.GetResult(s.db))
		public.GET("/query/:id/stream", handler.StreamResult(s.db))
		public.GET("/myip", handler.MyIP(s.geoDB))
			public.GET("/settings", handler.GetPublicSettings(s.db))
	}

	// Auth routes
	bruteForceGuard := middleware.NewBruteForceGuard(
		s.cfg.Security.BruteForceMax,
		s.cfg.Security.BruteForceBanMin,
	)
	api := r.Group("/api/v1")
	{
		api.POST("/auth/login", bruteForceGuard.Middleware(), handler.Login(s.db, s.cfg))
		api.POST("/auth/logout", middleware.Auth(s.cfg), handler.Logout())
	}

	// Admin routes (protected)
	admin := r.Group("/api/v1/admin")
	admin.Use(middleware.Auth(s.cfg), middleware.RequireAdmin(), middleware.RateLimit(s.cfg.Security.RateLimitPerMin))
	{
		admin.GET("/nodes", handler.ListAllNodes(s.db))
		admin.POST("/nodes", handler.CreateNode(s.db))
		admin.GET("/nodes/:id", handler.GetNode(s.db))
		admin.PUT("/nodes/:id", handler.UpdateNode(s.db))
		admin.DELETE("/nodes/:id", handler.DeleteNode(s.db))
		admin.POST("/nodes/:id/test", handler.TestNode(s.db))

		admin.GET("/audit", handler.ListAudit(s.db))
		admin.GET("/audit/export", handler.ExportAudit(s.db))

		admin.GET("/users", handler.ListUsers(s.db))
		admin.POST("/users", handler.CreateUser(s.db))
		admin.DELETE("/users/:id", handler.DeleteUser(s.db))

		admin.GET("/community-rules", handler.ListCommunityRules(s.db))
		admin.POST("/community-rules", handler.CreateCommunityRule(s.db))
		admin.PUT("/community-rules/:id", handler.UpdateCommunityRule(s.db))
		admin.DELETE("/community-rules/:id", handler.DeleteCommunityRule(s.db))
		admin.PATCH("/community-rules/:id/toggle", handler.ToggleCommunityRule(s.db))

		admin.GET("/bgp-neighbors", handler.ListBGPNeighbors(s.db, s.bgpMgr))
		admin.POST("/bgp-neighbors", handler.CreateBGPNeighbor(s.db, s.bgpMgr))
		admin.PUT("/bgp-neighbors/:id", handler.UpdateBGPNeighbor(s.db, s.bgpMgr))
		admin.DELETE("/bgp-neighbors/:id", handler.DeleteBGPNeighbor(s.db, s.bgpMgr))
		admin.GET("/bgp-neighbors/statuses", handler.GetBGPNeighborStatuses(s.bgpMgr))

			admin.GET("/settings", handler.GetAdminSettings(s.db))
			admin.PUT("/settings", handler.UpdateSettings(s.db))
			admin.POST("/settings/logo", handler.UploadLogo(s.db))

		if s.updater != nil {
			admin.GET("/update/status", handler.UpdateStatus(s.updater))
			admin.POST("/update/apply", handler.UpdateApply(s.updater))
		}
	}

	// Serve uploaded logo files
	r.GET("/logo.png", func(c *gin.Context) {
		http.ServeFile(c.Writer, c.Request, "data/uploads/logo.png")
	})
	r.GET("/logo.jpg", func(c *gin.Context) {
		http.ServeFile(c.Writer, c.Request, "data/uploads/logo.jpg")
	})
	r.GET("/logo.svg", func(c *gin.Context) {
		http.ServeFile(c.Writer, c.Request, "data/uploads/logo.svg")
	})
	r.GET("/logo.webp", func(c *gin.Context) {
		http.ServeFile(c.Writer, c.Request, "data/uploads/logo.webp")
	})

	// Serve SPA assets with long cache
	r.GET("/assets/*filepath", func(c *gin.Context) {
		filepath := c.Param("filepath")
		f, err := s.distFS.Open("assets" + filepath)
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		defer f.Close()

		stat, err := f.Stat()
		if err != nil || stat.IsDir() {
			c.Status(http.StatusNotFound)
			return
		}

		ext := strings.ToLower(filepath[strings.LastIndex(filepath, "."):])
		mime := "application/octet-stream"
		switch ext {
		case ".css":
			mime = "text/css; charset=utf-8"
		case ".js":
			mime = "application/javascript; charset=utf-8"
		case ".woff", ".woff2":
			mime = "font/woff2"
		case ".ttf":
			mime = "font/ttf"
		case ".svg":
			mime = "image/svg+xml"
		case ".png":
			mime = "image/png"
		case ".jpg", ".jpeg":
			mime = "image/jpeg"
		case ".ico":
			mime = "image/x-icon"
		case ".json":
			mime = "application/json"
		case ".html":
			mime = "text/html; charset=utf-8"
		}

		c.Header("Content-Type", mime)
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
		http.ServeContent(c.Writer, c.Request, stat.Name(), stat.ModTime(), f.(io.ReadSeeker))
	})

	// SPA fallback: serve index.html for all non-API, non-assets routes
	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		data, err := fs.ReadFile(s.distFS, "index.html")
		if err != nil {
			c.String(http.StatusInternalServerError, "failed to load app")
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", data)
	})
}

func (s *Server) Run(ctx context.Context) error {
	addr := fmt.Sprintf("%s:%d", s.cfg.Server.Host, s.cfg.Server.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: s.router,
	}

	slog.Info("server starting", "addr", addr)

	go func() {
		var err error
		if s.cfg.Server.TLSCert != "" && s.cfg.Server.TLSKey != "" {
			err = srv.ListenAndServeTLS(s.cfg.Server.TLSCert, s.cfg.Server.TLSKey)
		} else {
			err = srv.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
