package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/HopStat/HopStat/internal/agent"
	"github.com/HopStat/HopStat/internal/bgp"
	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/geo"
	"github.com/HopStat/HopStat/internal/server"
	"github.com/HopStat/HopStat/internal/store"
	"github.com/HopStat/HopStat/internal/store/repo"
)

var version = "dev"

func main() {
	modeFlag := flag.String("mode", "server", "run mode: server or agent")
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Override mode from flag if specified
	if *modeFlag != "" {
		cfg.Server.Mode = *modeFlag
	}

	slog.Info("starting hopstat",
		"version", version,
		"mode", cfg.Server.Mode,
		"port", cfg.Server.Port,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sigch := make(chan os.Signal, 1)
		signal.Notify(sigch, syscall.SIGINT, syscall.SIGTERM)
		<-sigch
		slog.Info("shutting down...")
		cancel()
	}()

	switch cfg.Server.Mode {
	case "server":
		db, err := store.Open(cfg.Database.Path)
		if err != nil {
			slog.Error("failed to open database", "error", err)
			os.Exit(1)
		}
		defer db.Close()

		if err := store.Migrate(db); err != nil {
			slog.Error("failed to migrate database", "error", err)
			os.Exit(1)
		}

		if pw := os.Getenv("LG_ADMIN_PASSWORD"); pw != "" {
			if err := store.SeedAdminPassword(db, pw); err != nil {
				slog.Error("failed to seed admin password", "error", err)
			} else {
				slog.Info("admin password set from LG_ADMIN_PASSWORD env")
			}
		}

		geoDB := geo.New(cfg.GeoIP.ASNDBPath, cfg.GeoIP.CityDBPath)
		defer geoDB.Close()
		if geoDB.Enabled() {
			slog.Info("geoip enabled", "asn_db", cfg.GeoIP.ASNDBPath, "city_db", cfg.GeoIP.CityDBPath)
		} else {
			slog.Warn("geoip disabled - set geoip.asn_db_path and geoip.city_db_path to enable")
		}

		var bgpMgr *bgp.SessionManager
		if cfg.BGP.ListenPort > 0 || cfg.BGP.RouterID != "" {
			bgpMgr = bgp.NewSessionManager(cfg.BGP)
			if err := bgpMgr.Start(ctx); err != nil {
				slog.Error("failed to start bgp manager", "error", err)
				os.Exit(1)
			}
			defer bgpMgr.Stop()
			if err := bgpMgr.LoadNeighbors(ctx, repo.NewBGPNeighborRepo(db)); err != nil {
				slog.Warn("failed to load bgp neighbors", "error", err)
			}
		}

		srv := server.New(cfg, db, geoDB, os.DirFS("web/dist"), bgpMgr, version)
		if err := srv.Run(ctx); err != nil {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}

	case "agent":
		agt := agent.New(cfg)
		if err := agt.Run(ctx); err != nil {
			slog.Error("agent error", "error", err)
			os.Exit(1)
		}

	default:
		fmt.Fprintf(os.Stderr, "invalid mode: %s (must be 'server' or 'agent')\n", cfg.Server.Mode)
		os.Exit(1)
	}
}
