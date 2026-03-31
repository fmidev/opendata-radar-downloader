package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	cfg, err := LoadConfig()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	}))
	slog.SetDefault(logger)

	src := newSource(cfg)

	slog.Info("starting fmi-radar-downloader",
		"source", cfg.Source,
		"output_dir", cfg.OutputDir,
		"poll_interval", cfg.PollInterval,
	)

	cleanupTempFiles(cfg.OutputDir)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	client := &http.Client{
		Timeout: cfg.HTTPTimeout,
	}

	consecutiveErrors := 0

	// Run first poll immediately, then wait between polls
	for {
		nextDelay := poll(ctx, client, cfg, src, &consecutiveErrors)

		select {
		case <-ctx.Done():
			slog.Info("shutting down")
			return
		case <-time.After(nextDelay):
		}
	}
}

func poll(ctx context.Context, client *http.Client, cfg *Config, src Source, consecutiveErrors *int) time.Duration {
	if ctx.Err() != nil {
		return 0
	}

	slog.Debug("fetching files", "source", src.Name())

	files, err := src.FetchFiles(ctx, client)
	if err != nil {
		*consecutiveErrors++
		backoff := errorBackoff(cfg.ErrorInterval, cfg.MaxBackoff, *consecutiveErrors)
		slog.Error("fetch failed",
			"source", src.Name(),
			"error", err,
			"consecutive_errors", *consecutiveErrors,
			"backoff", backoff,
		)
		return backoff
	}

	slog.Debug("found files", "count", len(files))

	downloadErrors := 0
	for _, rf := range files {
		if ctx.Err() != nil {
			return 0
		}
		if err := DownloadIfNew(ctx, client, rf, cfg); err != nil {
			slog.Error("download failed",
				"file", rf.Timestamp.Format("20060102150405")+"_"+cfg.FilePrefix+".tif",
				"error", err,
			)
			downloadErrors++
		}
	}

	if downloadErrors == 0 {
		*consecutiveErrors = 0
		writeHealthFile(cfg.OutputDir)
		if cfg.Retention > 0 {
			purgeOldFiles(cfg.OutputDir, cfg.Retention)
		}
	} else {
		*consecutiveErrors++
	}

	return cfg.PollInterval
}

func errorBackoff(base, max time.Duration, consecutive int) time.Duration {
	backoff := base
	for i := 1; i < consecutive; i++ {
		backoff *= 2
		if backoff >= max {
			return max
		}
	}
	return backoff
}

func writeHealthFile(outputDir string) {
	healthPath := filepath.Join(outputDir, ".last_successful_poll")
	ts := time.Now().UTC().Format(time.RFC3339)
	os.WriteFile(healthPath, []byte(ts+"\n"), 0o644)
}

func purgeOldFiles(outputDir string, retention time.Duration) {
	cutoff := time.Now().Add(-retention)

	entries, err := os.ReadDir(outputDir)
	if err != nil {
		slog.Warn("failed to read output directory for purge", "error", err)
		return
	}

	removed := 0
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".tif" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			path := filepath.Join(outputDir, e.Name())
			if err := os.Remove(path); err != nil {
				slog.Warn("failed to remove old file", "file", e.Name(), "error", err)
			} else {
				removed++
			}
		}
	}

	if removed > 0 {
		slog.Info("purged old files", "count", removed, "retention", retention)
	}
}

func cleanupTempFiles(outputDir string) {
	patterns := []string{
		filepath.Join(outputDir, ".download-*.tmp"),
		filepath.Join(outputDir, "*.cog.tmp"),
		filepath.Join(outputDir, "*.raw"),
	}

	removed := 0
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}
		for _, f := range matches {
			if err := os.Remove(f); err != nil {
				slog.Warn("failed to remove temp file", "file", f, "error", err)
			} else {
				removed++
			}
		}
	}

	if removed > 0 {
		slog.Info("cleaned up stale temp files", "count", removed)
	}
}
