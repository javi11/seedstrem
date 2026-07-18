// Command seedstrem runs a Stremio addon that searches Prowlarr indexers
// and streams torrents through Deluge while they download.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/javib/seedstrem/internal/admin"
	"github.com/javib/seedstrem/internal/config"
	"github.com/javib/seedstrem/internal/deluge"
	"github.com/javib/seedstrem/internal/meta"
	"github.com/javib/seedstrem/internal/prowlarr"
	"github.com/javib/seedstrem/internal/server"
	"github.com/javib/seedstrem/internal/store"
	"github.com/javib/seedstrem/internal/stream"
	"github.com/javib/seedstrem/internal/stremio"
	"github.com/javib/seedstrem/internal/syncer"
	"github.com/javib/seedstrem/internal/torrents"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "seedstrem:", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", defaultConfigPath(), "path to config.yaml")
	healthcheck := flag.Bool("healthcheck", false, "probe the local /api/health endpoint and exit (for Docker HEALTHCHECK)")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("seedstrem", version)
		return nil
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	if *healthcheck {
		return probeHealth(cfg.Server.Listen)
	}

	logger := newLogger(cfg.Log.Level)
	slog.SetDefault(logger)

	// Generate api token / admin password on first run and persist them.
	changed, err := config.EnsureSecrets(&cfg)
	if err != nil {
		return err
	}
	if changed {
		if err := config.Save(cfg, *configPath); err != nil {
			logger.Warn("could not persist generated secrets; they will change on restart",
				"error", err, "config", *configPath)
		}
		logger.Info("generated admin password (also saved to config)",
			"admin_password", cfg.Server.AdminPassword,
			"config", *configPath)
	}

	db, err := store.Open(cfg.Storage.Database)
	if err != nil {
		return err
	}
	defer db.Close()

	cm := config.NewManager(cfg, *configPath)
	dc := deluge.NewSwappable(deluge.New(cfg.Deluge.Host, cfg.Deluge.Port, cfg.Deluge.Username, cfg.Deluge.Password))

	torrentSvc := torrents.New(db, dc, func() torrents.Settings {
		c := cm.Get()
		return torrents.Settings{
			MetadataTimeout:     c.Meta.MetadataTimeout,
			DeleteFilesOnRemove: c.Storage.DeleteFilesOnRemove,
		}
	}, logger)

	metaClient := meta.New(cfg.Meta.CinemetaURL, cfg.Meta.TMDbAPIKey)
	stremioHandler := stremio.New(torrentSvc, metaClient, func() stremio.Settings {
		c := cm.Get()
		return stremio.Settings{
			ExternalURL: c.Server.ExternalURL,
			Prowlarr: stremio.ProwlarrSettings{
				URL:             c.Prowlarr.URL,
				APIKey:          c.Prowlarr.APIKey,
				MovieCategories: c.Prowlarr.MovieCategories,
				TVCategories:    c.Prowlarr.TVCategories,
				AnimeCategories: c.Prowlarr.AnimeCategories,
				IndexerIDs:      c.Prowlarr.IndexerIDs,
			},
			Addon: stremio.AddonSettings{
				EnableMovies: c.Addon.EnableMovies,
				EnableSeries: c.Addon.EnableSeries,
				EnableAnime:  c.Addon.EnableAnime,
			},
			Filters: prowlarr.Filters{
				MinSeeders:   c.Filters.MinSeeders,
				MinSizeBytes: c.Filters.MinSizeMB << 20,
				MaxSizeBytes: c.Filters.MaxSizeMB << 20,
			},
			MaxResults: c.Filters.MaxResults,
		}
	}, version, logger)

	resolver := stream.NewResolver(dc, func() []config.Mapping { return cm.Get().Paths.Mappings })
	avail := stream.NewAvailability(dc)
	streamHandler := stream.NewHandler(db, dc, resolver, avail, func() stream.Settings {
		c := cm.Get()
		return stream.Settings{
			WaitTimeout: c.Stream.WaitTimeout,
			ReadChunk:   c.Stream.ReadChunk,
		}
	}, logger)

	adminHandler := admin.New(cm, db, dc, version, logger)

	syncCtx, stopSync := context.WithCancel(context.Background())
	defer stopSync()
	go syncer.New(db, dc, logger, 30*time.Second).Run(syncCtx)

	handler := server.New(server.Options{
		Logger:  logger,
		Stremio: stremioHandler.Router(),
		Stream:  streamHandler.Router(),
		Admin:   adminHandler.Router(),
	})

	httpServer := &http.Server{
		Addr:              cfg.Server.Listen,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("seedstrem listening", "addr", cfg.Server.Listen, "version", version)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return err
	case sig := <-quit:
		logger.Info("shutting down", "signal", sig.String())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return httpServer.Shutdown(ctx)
}

func defaultConfigPath() string {
	if _, err := os.Stat("/config"); err == nil {
		return "/config/config.yaml"
	}
	return "config.yaml"
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: lvl}
	if fi, err := os.Stdout.Stat(); err == nil && fi.Mode()&os.ModeCharDevice != 0 {
		return slog.New(slog.NewTextHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, opts))
}

func probeHealth(listen string) error {
	addr := listen
	if addr == "" {
		addr = ":8080"
	}
	if addr[0] == ':' {
		addr = "127.0.0.1" + addr
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://" + addr + "/api/health")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned %d", resp.StatusCode)
	}
	return nil
}
