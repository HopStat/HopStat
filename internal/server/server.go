package server

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/HopStat/HopStat/internal/bgp"
	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/geo"
	"github.com/HopStat/HopStat/internal/server/handler"
	"github.com/HopStat/HopStat/internal/server/middleware"
	"github.com/HopStat/HopStat/internal/sitecache"
	"github.com/HopStat/HopStat/internal/updater"
	"github.com/gin-gonic/gin"
)

type Server struct {
	cfg          *config.Config
	db           *sql.DB
	geoDB        *geo.GeoIPDB
	bgpMgr       *bgp.SessionManager
	queryHandler *handler.Handler
	router       *gin.Engine
	distFS       fs.FS
	updater      *updater.Updater
	version      string
}

func New(cfg *config.Config, db *sql.DB, geoDB *geo.GeoIPDB, distFS fs.FS, bgpMgr *bgp.SessionManager, version string) *Server {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.Logger())
	ConfigureClientIP(router, cfg.Server)
	router.Use(middleware.CORS(nil))
	router.Use(func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "0")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob: https://flagcdn.com; font-src 'self'; connect-src 'self'; frame-ancestors 'none'")
		c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		c.Next()
	})

	// Limit request body to 2MB
	router.MaxMultipartMemory = 2 << 20

	srv := &Server{
		cfg:          cfg,
		db:           db,
		geoDB:        geoDB,
		bgpMgr:       bgpMgr,
		queryHandler: handler.New(db, cfg, geoDB, bgpMgr),
		router:       router,
		distFS:       distFS,
		version:      version,
	}
	srv.updater = updater.New("HopStat/HopStat", version, cfg.Update.Enabled)
	// Read per call, so switching self-update off in the admin panel takes effect at once.
	srv.updater.SetEnabledSource(selfUpdateSettingSource(db))

	srv.setupRoutes()
	return srv
}

func (s *Server) setupRoutes() {
	r := s.router
	credKey := s.cfg.Security.CredentialKey
	denyList := middleware.NewJTIDenyList()
	uploadsDir := handler.ResolveUploadsDir(s.cfg.Database.Path)
	handler.SetLogoUploadsDir(uploadsDir)
	sitecache.SetLogoUploadsDir(uploadsDir)

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "mode": "server", "version": s.version})
	})

	// Remote agent API for standalone nodes (token auth per node)
	agentAPI := r.Group("")
	agentAPI.Use(middleware.NodeAgentAuth(s.db, credKey))
	handler.MountAgentAPI(agentAPI, s.cfg, s.bgpMgr, s.geoDB)

	// Public API routes
	public := r.Group("/api/v1")
	fc := s.cfg.FloodControl
	public.Use(middleware.OptionalRateLimit(fc.Enabled, fc.HTTPRateLimitPerMin))
	{
		public.GET("/nodes", handler.ListNodes(s.db, credKey, s.bgpMgr))
		public.GET("/nodes/:id", handler.GetNode(s.db, credKey))
		public.POST("/query", s.queryHandler.SubmitQuery())
		public.GET("/query/:id", handler.GetResult(s.db))
		public.GET("/query/:id/stream", handler.StreamResult(s.db))
		public.GET("/myip", handler.MyIP(s.geoDB))
		public.GET("/settings", handler.GetPublicSettings(s.db, s.cfg.BGP))
		public.GET("/communities", handler.ListPublicCommunities(s.db))
		public.GET("/quick-queries", handler.ListPublicQuickQueries())
	}

	// Auth routes
	var loginMW gin.HandlerFunc
	if fc.Enabled && fc.BruteForceMax > 0 {
		bruteForceGuard := middleware.NewBruteForceGuard(fc.BruteForceMax, fc.BruteForceBanMin)
		loginMW = bruteForceGuard.Middleware()
	} else {
		loginMW = func(c *gin.Context) { c.Next() }
	}
	api := r.Group("/api/v1")
	{
		api.POST("/auth/login", loginMW, handler.Login(s.db, s.cfg))
		api.GET("/auth/session", middleware.Auth(s.cfg, denyList), handler.Session())
		api.POST("/auth/logout", handler.Logout(s.cfg, denyList))
	}

	// Admin routes (protected)
	admin := r.Group("/api/v1/admin")
	admin.Use(middleware.Auth(s.cfg, denyList), middleware.RequireAdmin(), middleware.OptionalRateLimit(fc.Enabled, fc.HTTPRateLimitPerMin))
	{
		admin.GET("/nodes", handler.ListAllNodes(s.db, credKey))
		admin.POST("/nodes", handler.CreateNode(s.db, credKey))
		admin.GET("/nodes/:id", handler.GetNode(s.db, credKey))
		admin.PUT("/nodes/:id", handler.UpdateNode(s.db, credKey))
		admin.PATCH("/nodes/:id/default", handler.SetDefaultNode(s.db, credKey))
		admin.DELETE("/nodes/:id", handler.DeleteNode(s.db, credKey, s.bgpMgr))
		admin.POST("/nodes/:id/test", handler.TestNode(s.db, credKey, s.cfg))

		admin.GET("/audit", handler.ListAudit(s.db))
		admin.GET("/audit/export", handler.ExportAudit(s.db))

		admin.GET("/account", handler.GetAccount(s.db))
		admin.PUT("/account", handler.UpdateAccount(s.db))

		admin.GET("/community-rules", handler.ListCommunityRules(s.db))
		admin.POST("/community-rules", handler.CreateCommunityRule(s.db))
		admin.PUT("/community-rules/:id", handler.UpdateCommunityRule(s.db))
		admin.DELETE("/community-rules/:id", handler.DeleteCommunityRule(s.db))
		admin.PATCH("/community-rules/:id/toggle", handler.ToggleCommunityRule(s.db))

		admin.GET("/bgp-neighbors", handler.ListBGPNeighbors(s.db, s.bgpMgr, s.cfg.BGP))
		admin.GET("/bgp/config", handler.GetBGPConfig(s.cfg.BGP, s.bgpMgr))
		admin.POST("/bgp-neighbors", handler.CreateBGPNeighbor(s.db, s.bgpMgr, s.cfg.BGP))
		admin.PUT("/bgp-neighbors/:id", handler.UpdateBGPNeighbor(s.db, s.bgpMgr, s.cfg.BGP))
		admin.DELETE("/bgp-neighbors/:id", handler.DeleteBGPNeighbor(s.db, s.bgpMgr))
		admin.GET("/bgp-neighbors/statuses", handler.GetBGPNeighborStatuses(s.bgpMgr))
		admin.POST("/bgp-neighbors/:id/stop", handler.StopBGPNeighbor(s.bgpMgr))
		admin.POST("/bgp-neighbors/:id/restart", handler.RestartBGPNeighbor(s.bgpMgr))
		admin.GET("/bgp-neighbors/:id/logs", handler.GetBGPNeighborLogs(s.bgpMgr))
		admin.GET("/bgp/paths", handler.LookupBGPPaths(s.bgpMgr))

		admin.GET("/settings", handler.GetAdminSettings(s.db))
		admin.PUT("/settings", handler.UpdateSettings(s.db))
		admin.POST("/settings/logo", handler.UploadLogo(s.db))

		admin.GET("/quick-queries", handler.ListQuickQueries(s.db))
		admin.POST("/quick-queries", handler.CreateQuickQuery(s.db))
		admin.PUT("/quick-queries/:id", handler.UpdateQuickQuery(s.db))
		admin.DELETE("/quick-queries/:id", handler.DeleteQuickQuery(s.db))
		admin.PATCH("/quick-queries/:id/toggle", handler.ToggleQuickQuery(s.db))

		admin.GET("/geoip/status", handler.GeoIPStatus(s.db, s.cfg, s.geoDB))
		admin.PUT("/geoip/config", handler.UpdateGeoIPConfig(s.db, s.cfg, s.geoDB))
		admin.GET("/geoip/lookup", handler.GeoIPLookup(s.geoDB))
		admin.GET("/system/status", handler.SystemStatus())
		admin.GET("/system/addresses", handler.SystemAddresses())

		admin.GET("/update/status", handler.UpdateStatus(s.updater))
		admin.POST("/update/apply", handler.UpdateApply(s.updater))
	}

	// Serve uploaded logo files
	serveLogo := func(ext string) gin.HandlerFunc {
		return func(c *gin.Context) {
			c.Header("Cache-Control", "no-cache, must-revalidate")
			http.ServeFile(c.Writer, c.Request, filepath.Join(uploadsDir, "logo"+ext))
		}
	}
	r.GET("/logo.png", serveLogo(".png"))
	r.GET("/logo.jpg", serveLogo(".jpg"))
	r.GET("/logo.svg", serveLogo(".svg"))
	r.GET("/logo.webp", serveLogo(".webp"))

	r.GET("/appearance-boot.js", func(c *gin.Context) {
		data, err := fs.ReadFile(s.distFS, "appearance-boot.js")
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		c.Header("Content-Type", "application/javascript; charset=utf-8")
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
		c.Data(http.StatusOK, "application/javascript; charset=utf-8", data)
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

		dotIdx := strings.LastIndex(filepath, ".")
		ext := ""
		if dotIdx >= 0 {
			ext = strings.ToLower(filepath[dotIdx:])
		}
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
		rs, ok := f.(io.ReadSeeker)
		if !ok {
			c.Status(http.StatusInternalServerError)
			return
		}
		http.ServeContent(c.Writer, c.Request, stat.Name(), stat.ModTime(), rs)
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
		c.Data(http.StatusOK, "text/html; charset=utf-8", injectIndexHTML(data))
	})
}

func (s *Server) Run(ctx context.Context) error {
	addr := fmt.Sprintf("%s:%d", s.cfg.Server.Host, s.cfg.Server.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.router,
		ReadHeaderTimeout: 10 * time.Second,
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
