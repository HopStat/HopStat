package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/HopStat/HopStat/internal/agent"
	"github.com/HopStat/HopStat/internal/bgp"
	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/geo"
	"github.com/HopStat/HopStat/internal/server"
	"github.com/HopStat/HopStat/internal/sitecache"
	"github.com/HopStat/HopStat/internal/store"
	"github.com/HopStat/HopStat/internal/store/queries"
	"github.com/HopStat/HopStat/internal/store/repo"
	"github.com/HopStat/HopStat/web"
)

var version = "dev"

func adminPasswordForce(configGenerated bool) bool {
	if configGenerated {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("LG_FORCE_ADMIN_PASSWORD"))) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

func main() {
	modeFlag := flag.String("mode", "server", "run mode: server or agent")
	configPath := flag.String("config", "config.yaml", "path to config file")
	installService := flag.Bool("install-service", false, "install hopstat as a systemd service and exit")
	bootstrap := flag.Bool("bootstrap", false, "initialize config, database and admin user in the data volume, then exit")
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

	if *bootstrap {
		if err := runBootstrap(*configPath); err != nil {
			fmt.Fprintf(os.Stderr, "bootstrap failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Auto-generate config if it doesn't exist yet
	configGenerated := false
	if _, err := os.Stat(*configPath); os.IsNotExist(err) {
		fmt.Printf("[hopstat] config not found — generating %s\n", *configPath)
		if err := config.Generate(*configPath, *modeFlag); err != nil {
			fmt.Fprintf(os.Stderr, "failed to generate config: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("[hopstat] generated %s with random secrets\n", *configPath)
		configGenerated = true
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

		if err := sitecache.Load(db, cfg.Security.CredentialKey, cfg.BGP.LocalAS); err != nil {
			slog.Warn("failed to load site cache", "error", err)
		}

		// LG_ADMIN_PASSWORD env var takes precedence (useful in Docker/CI)
		if pw := os.Getenv("LG_ADMIN_PASSWORD"); pw != "" {
			os.Unsetenv("LG_ADMIN_PASSWORD")
			if applied, err := store.ApplyAdminPassword(db, pw, adminPasswordForce(configGenerated)); err != nil {
				slog.Error("failed to seed admin password", "error", err)
			} else if applied {
				if configGenerated {
					slog.Info("admin password set from LG_ADMIN_PASSWORD (config auto-generated)")
				} else {
					slog.Info("admin password set from LG_ADMIN_PASSWORD env")
				}
				printFirstRunCredentials(store.DefaultAdminEmail, pw, cfg.Server.Port)
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

		asnPath, cityPath := geo.ResolvePaths(cfg.GeoIP)
		geoDB := geo.New(asnPath, cityPath)
		defer geoDB.Close()
		if geoDB.Enabled() {
			slog.Info("geoip enabled", "asn_db", asnPath, "city_db", cityPath)
		} else {
			slog.Warn("geoip disabled — set geoip.license_key and geoip.account_id to enable")
		}

		q := queries.New(db)
		if err := geo.SyncSettings(q, cfg.GeoIP); err != nil {
			slog.Warn("failed to sync geoip settings", "error", err)
		}

		settings, _ := q.GetSettings()
		if settings[geo.SettingLicenseKey] != "" && settings[geo.SettingAccountID] != "" {
			geoUpdater := geo.NewUpdater(cfg.GeoIP, geoDB)
			geoUpdater.SetLastDownload(func(edition string) time.Time {
				current, err := q.GetSettings()
				if err != nil {
					return time.Time{}
				}
				return geo.LastDownloadFromSettings(current, edition)
			})
			geoUpdater.SetUpdateInterval(func() time.Duration {
				current, err := q.GetSettings()
				if err != nil {
					return geo.ParseUpdateInterval(cfg.GeoIP.UpdateInterval, 72*time.Hour)
				}
				return geo.ResolveUpdateInterval(current, cfg.GeoIP)
			})
			geoUpdater.SetCredentials(func() (string, string) {
				current, err := q.GetSettings()
				if err != nil {
					return cfg.GeoIP.LicenseKey, cfg.GeoIP.AccountID
				}
				return current[geo.SettingLicenseKey], current[geo.SettingAccountID]
			})
			geoUpdater.SetOnDownload(func(edition string, downloadedAt time.Time) {
				key := geo.SettingASNLastDownload
				if edition == "GeoLite2-City" {
					key = geo.SettingCityLastDownload
				}
				if err := q.SetSetting(key, downloadedAt.Format(time.RFC3339)); err != nil {
					slog.Warn("failed to save geoip download time", "edition", edition, "error", err)
				}
			})
			go geoUpdater.Run(ctx)
		}

		var bgpMgr *bgp.SessionManager
		bgpMgr = bgp.NewSessionManager(cfg.BGP)
		if err := bgpMgr.Start(ctx); err != nil {
			slog.Warn("bgp manager unavailable — neighbors will be stored but sessions will not start", "error", err)
			bgpMgr = nil
		} else {
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
	fmt.Println("  ║  Change your password in Admin → Settings after login. ║")
	fmt.Println("  ╚" + line + "╝")
	fmt.Println()
}

func runBootstrap(cfgPath string) error {
	configGenerated := false
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		fmt.Printf("[hopstat] config not found — generating %s\n", cfgPath)
		if err := config.Generate(cfgPath, "server"); err != nil {
			return fmt.Errorf("generate config: %w", err)
		}
		fmt.Printf("[hopstat] generated %s with random secrets\n", cfgPath)
		configGenerated = true
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	db, err := store.Open(cfg.Database.Path)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	if err := store.Migrate(db); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}

	email := store.DefaultAdminEmail
	password := os.Getenv("LG_ADMIN_PASSWORD")
	var passwordSet bool

	if password != "" {
		applied, err := store.ApplyAdminPassword(db, password, adminPasswordForce(configGenerated))
		if err != nil {
			return fmt.Errorf("seed admin password: %w", err)
		}
		if applied {
			passwordSet = true
			if configGenerated {
				fmt.Println("[hopstat] admin password set from LG_ADMIN_PASSWORD (config auto-generated)")
			} else {
				fmt.Println("[hopstat] admin password set from LG_ADMIN_PASSWORD")
			}
		} else {
			fmt.Println("[hopstat] admin password already configured — leaving unchanged")
		}
	} else {
		var generated bool
		var err error
		email, password, generated, err = store.EnsureFirstAdmin(db)
		if err != nil {
			return fmt.Errorf("ensure admin user: %w", err)
		}
		if generated {
			passwordSet = true
			fmt.Println("[hopstat] generated random admin password")
		} else if email != "" {
			fmt.Println("[hopstat] admin password already configured — leaving unchanged")
		} else {
			return fmt.Errorf("no admin user found — run migrations first")
		}
	}

	fmt.Printf("[hopstat] bootstrap complete\n")
	fmt.Printf("  admin email: %s\n", email)
	if passwordSet {
		fmt.Printf("  admin password: %s\n", password)
	}
	return nil
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
		if err := replaceInFile(cfgDest, `path: "./lg.db"`, `path: "/var/lib/hopstat/lg.db"`); err != nil {
			slog.Warn("failed to update database path in config", "err", err)
		}
		if err := replaceInFile(cfgDest, `db_dir: "./data/geoip"`, `db_dir: "/var/lib/hopstat/geoip"`); err != nil {
			slog.Warn("failed to update geoip path in config", "err", err)
		}
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
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

func replaceInFile(path, old, new string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	updated := strings.ReplaceAll(string(data), old, new)
	return os.WriteFile(path, []byte(updated), 0600)
}
