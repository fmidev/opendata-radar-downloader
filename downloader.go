package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

func DownloadIfNew(ctx context.Context, client *http.Client, m Member, cfg *Config) error {
	fileName := m.PhenomenonTime.Format("20060102150405") + "_" + cfg.FilePrefix + ".tif"
	filePath := filepath.Join(cfg.OutputDir, fileName)

	if _, err := os.Stat(filePath); err == nil {
		slog.Debug("file exists, skipping", "file", fileName)
		return nil
	}

	var lastErr error
	for attempt := 1; attempt <= cfg.MaxRetries; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		lastErr = downloadFile(ctx, client, m.FileReference, filePath)
		if lastErr == nil {
			return nil
		}

		if isNoSpace(lastErr) {
			slog.Error("disk full, cannot download", "file", fileName, "error", lastErr)
			return lastErr
		}

		if attempt < cfg.MaxRetries {
			backoff := time.Duration(1<<(attempt-1)) * time.Second
			slog.Warn("download failed, retrying",
				"file", fileName,
				"attempt", attempt,
				"max_retries", cfg.MaxRetries,
				"backoff", backoff,
				"error", lastErr,
			)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}
	}

	return fmt.Errorf("download failed after %d attempts: %w", cfg.MaxRetries, lastErr)
}

func downloadFile(ctx context.Context, client *http.Client, url, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	start := time.Now()

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	f, err := os.CreateTemp(filepath.Dir(destPath), ".download-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}

	tmpPath := f.Name()

	n, err := io.Copy(f, resp.Body)
	if closeErr := f.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("writing file: %w", err)
	}

	if err := os.Chmod(tmpPath, 0o644); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("setting file permissions: %w", err)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming temp file: %w", err)
	}

	fileName := filepath.Base(destPath)
	duration := time.Since(start)
	slog.Info("file downloaded",
		"file", fileName,
		"size", n,
		"duration", duration.Round(time.Millisecond),
	)

	return nil
}

func isNoSpace(err error) bool {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return errors.Is(pathErr.Err, syscall.ENOSPC)
	}
	return errors.Is(err, syscall.ENOSPC)
}
