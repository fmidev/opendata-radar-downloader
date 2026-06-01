package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
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
				"file", rf.outputRelPath(cfg),
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

	removed := 0
	// Walk recursively: some sources (e.g. fmi_s3) write into per-radar
	// subdirectories, and output files may be .tif or raw .h5/.hdf5.
	err := filepath.WalkDir(outputDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		switch filepath.Ext(d.Name()) {
		case ".tif", ".h5", ".hdf5":
		default:
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.ModTime().Before(cutoff) {
			if err := os.Remove(path); err != nil {
				slog.Warn("failed to remove old file", "file", path, "error", err)
			} else {
				removed++
			}
		}
		return nil
	})
	if err != nil {
		slog.Warn("failed to walk output directory for purge", "error", err)
	}

	if removed > 0 {
		slog.Info("purged old files", "count", removed, "retention", retention)
	}
}

func cleanupTempFiles(outputDir string) {
	removed := 0
	// Walk recursively so temp files left in per-radar subdirectories are also
	// cleaned. Only intermediate artifacts are matched (see isTempFile) — raw
	// .h5/.hdf5 output files are deliberately left intact.
	err := filepath.WalkDir(outputDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if isTempFile(d.Name()) {
			if err := os.Remove(path); err != nil {
				slog.Warn("failed to remove temp file", "file", path, "error", err)
			} else {
				removed++
			}
		}
		return nil
	})
	if err != nil {
		slog.Warn("failed to walk output directory for temp cleanup", "error", err)
	}

	if removed > 0 {
		slog.Info("cleaned up stale temp files", "count", removed)
	}
}

// isTempFile reports whether name is an intermediate artifact of the download
// pipeline. The HDF5/raw intermediates are always named "<final>.tif.h5" /
// "<final>.tif.raw", so matching the ".tif." infix avoids deleting raw .h5
// output files (e.g. from the fmi_s3 source).
func isTempFile(name string) bool {
	if strings.HasPrefix(name, ".download-") && strings.HasSuffix(name, ".tmp") {
		return true
	}
	for _, suffix := range []string{".gdal.tmp", ".cog.tmp", ".tif.raw", ".tif.h5", ".tif.hdf5"} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}
