package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/HopStat/HopStat/internal/agent"
	"github.com/HopStat/HopStat/internal/bgp"
	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/geo"
	"github.com/HopStat/HopStat/internal/server"
	"github.com/HopStat/HopStat/internal/store"
	"github.com/HopStat/HopStat/internal/store/repo"
	"github.com/HopStat/HopStat/web"
)

var version = "dev"

func main() {
	modeFlag := flag.String("mode", "server", "run mode: server or agent")
	configPath := flag.String("config", "config.yaml", "path to config file")
	installService := flag.Bool("install-service", false, "install hopstat as a systemd service and exit")
	versionFlag := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Println("hopstat", version)
		return
	}

	if *installService {
		if err := runInstallService(*configPath, *modeFlag); err != nil {
			fmt.Fprintf(os.Stderr, "install-service failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Auto-generate config if it doesn't exist yet
	if _, err := os.Stat(*configPath); os.IsNotExist(err) {
		fmt.Printf("[hopstat] config not found — generating %s\n", *configPath)
		if err := config.Generate(*configPath, *modeFlag); err != nil {
			fmt.Fprintf(os.Stderr, "failed to generate config: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("[hopstat] generated %s with random secrets\n", *configPath)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	// --mode flag overrides config
	if *modeFlag != "" {
		cfg.Server.Mode = *modeFlag
	}

	slog.Info("starting hopstat", "version", version, "mode", cfg.Server.Mode, "port", cfg.Server.Port)

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

		// LG_ADMIN_PASSWORD env var takes precedence (useful in Docker/CI)
		if pw := os.Getenv("LG_ADMIN_PASSWORD"); pw != "" {
			if err := store.SeedAdminPassword(db, pw); err != nil {
				slog.Error("failed to seed admin password", "error", err)
			} else {
				slog.Info("admin password set from LG_ADMIN_PASSWORD env")
			}
		} else {
			// First-run: generate a random admin password and show it once
			email, pw, generated, err := store.EnsureFirstAdmin(db)
			if err != nil {
				slog.Error("failed to ensure admin user", "error", err)
				os.Exit(1)
			}
			if generated {
				printFirstRunCredentials(email, pw, cfg.Server.Port)
			}
		}

		geoDB := geo.New(cfg.GeoIP.ASNDBPath, cfg.GeoIP.CityDBPath)
		defer geoDB.Close()
		if geoDB.Enabled() {
			slog.Info("geoip enabled", "asn_db", cfg.GeoIP.ASNDBPath, "city_db", cfg.GeoIP.CityDBPath)
		} else {
			slog.Warn("geoip disabled — set geoip.license_key and geoip.account_id to enable")
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

		srv := server.New(cfg, db, geoDB, web.Dist(), bgpMgr, version)
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

// printFirstRunCredentials prints the auto-generated admin credentials prominently.
func printFirstRunCredentials(email, password string, port int) {
	line := strings.Repeat("═", 54)
	fmt.Println()
	fmt.Println("  ╔" + line + "╗")
	fmt.Println("  ║          HOPSTAT — FIRST RUN CREDENTIALS           ║")
	fmt.Println("  ╠" + line + "╣")
	fmt.Printf("  ║  URL      http://localhost:%d/admin                ║\n", port)
	fmt.Printf("  ║  Email    %-40s║\n", email)
	fmt.Printf("  ║  Password %-40s║\n", password)
	fmt.Println("  ╠" + line + "╣")
	fmt.Println("  ║  Change your password in Admin → Users after login. ║")
	fmt.Println("  ╚" + line + "╝")
	fmt.Println()
}

// runInstallService installs hopstat as a systemd service.
func runInstallService(cfgPath, mode string) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("--install-service must be run as root (sudo hopstat --install-service)")
	}

	binDest := "/usr/local/bin/hopstat"
	cfgDest := "/etc/hopstat/config.yaml"
	unitFile := "/etc/systemd/system/hopstat.service"

	// 1. Copy binary
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	self, err = filepath.EvalSymlinks(self)
	if err != nil {
		return fmt.Errorf("eval symlinks: %w", err)
	}
	fmt.Printf("→ installing binary to %s\n", binDest)
	if err := copyFile(self, binDest, 0755); err != nil {
		return fmt.Errorf("copy binary: %w", err)
	}

	// 2. Generate config if missing
	if err := os.MkdirAll("/etc/hopstat", 0755); err != nil {
		return fmt.Errorf("create /etc/hopstat: %w", err)
	}
	if _, err := os.Stat(cfgDest); os.IsNotExist(err) {
		fmt.Printf("→ generating config at %s\n", cfgDest)
		if err := config.Generate(cfgDest, mode); err != nil {
			return fmt.Errorf("generate config: %w", err)
		}
		// Point database to a persistent data dir
		replaceInFile(cfgDest, `path: "./lg.db"`, `path: "/var/lib/hopstat/lg.db"`)
		replaceInFile(cfgDest, `db_dir: "./data/geoip"`, `db_dir: "/var/lib/hopstat/geoip"`)
		if err := os.MkdirAll("/var/lib/hopstat", 0755); err != nil {
			return fmt.Errorf("create data dir: %w", err)
		}
	} else {
		fmt.Printf("→ config already exists at %s — skipping generation\n", cfgDest)
	}

	// Resolve the effective config path for the unit
	effectiveCfg := cfgDest
	if cfgPath != "config.yaml" {
		effectiveCfg = cfgPath
	}

	// 3. Write systemd unit
	fmt.Printf("→ writing unit file %s\n", unitFile)
	unit := fmt.Sprintf(`[Unit]
Description=HopStat Network Looking Glass
Documentation=https://github.com/HopStat/HopStat
After=network.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s --mode=%s --config=%s
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=hopstat
AmbientCapabilities=CAP_NET_RAW CAP_NET_ADMIN

[Install]
WantedBy=multi-user.target
`, binDest, mode, effectiveCfg)

	if err := os.WriteFile(unitFile, []byte(unit), 0644); err != nil {
		return fmt.Errorf("write unit file: %w", err)
	}

	// 4. Enable and start
	for _, args := range [][]string{
		{"systemctl", "daemon-reload"},
		{"systemctl", "enable", "hopstat"},
		{"systemctl", "restart", "hopstat"},
	} {
		fmt.Printf("→ running: %s\n", strings.Join(args, " "))
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s: %w", strings.Join(args, " "), err)
		}
	}

	fmt.Println()
	fmt.Println("✓ hopstat service installed and started")
	fmt.Printf("  config:  %s\n", effectiveCfg)
	fmt.Printf("  logs:    journalctl -u hopstat -f\n")
	fmt.Printf("  status:  systemctl status hopstat\n")
	fmt.Println()
	fmt.Println("  Admin credentials will appear in the service logs on first start:")
	fmt.Println("  journalctl -u hopstat | grep -A 10 'FIRST RUN'")
	return nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, mode)
}

func replaceInFile(path, old, new string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	updated := strings.ReplaceAll(string(data), old, new)
	os.WriteFile(path, []byte(updated), 0600)
}
