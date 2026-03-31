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

	slog.Info("starting fmi-radar-downloader",
		"output_dir", cfg.OutputDir,
		"poll_interval", cfg.PollInterval,
		"wfs_url", cfg.WFSURL,
	)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	client := &http.Client{
		Timeout: cfg.HTTPTimeout,
	}

	consecutiveErrors := 0

	// Run first poll immediately, then wait between polls
	for {
		nextDelay := poll(ctx, client, cfg, &consecutiveErrors)

		select {
		case <-ctx.Done():
			slog.Info("shutting down")
			return
		case <-time.After(nextDelay):
		}
	}
}

func poll(ctx context.Context, client *http.Client, cfg *Config, consecutiveErrors *int) time.Duration {
	if ctx.Err() != nil {
		return 0
	}

	slog.Debug("fetching WFS", "url", cfg.WFSURL)

	members, err := FetchWFS(ctx, client, cfg.WFSURL)
	if err != nil {
		*consecutiveErrors++
		backoff := errorBackoff(cfg.ErrorInterval, cfg.MaxBackoff, *consecutiveErrors)
		slog.Error("wfs fetch failed",
			"error", err,
			"consecutive_errors", *consecutiveErrors,
			"backoff", backoff,
		)
		return backoff
	}

	slog.Debug("found members", "count", len(members))

	downloadErrors := 0
	for _, m := range members {
		if ctx.Err() != nil {
			return 0
		}
		if err := DownloadIfNew(ctx, client, m, cfg); err != nil {
			slog.Error("download failed",
				"file", m.PhenomenonTime.Format("20060102150405")+"_"+cfg.FilePrefix+".tif",
				"error", err,
			)
			downloadErrors++
		}
	}

	if downloadErrors == 0 {
		*consecutiveErrors = 0
		writeHealthFile(cfg.OutputDir)
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
