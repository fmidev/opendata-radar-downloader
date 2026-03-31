package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
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

	// When COG is enabled, download to a raw file first, then convert
	downloadPath := filePath
	if cfg.COGEnabled {
		downloadPath = filePath + ".raw"
	}

	var lastErr error
	for attempt := 1; attempt <= cfg.MaxRetries; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		lastErr = downloadFile(ctx, client, m.FileReference, downloadPath)
		if lastErr == nil {
			break
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

	if lastErr != nil {
		return fmt.Errorf("download failed after %d attempts: %w", cfg.MaxRetries, lastErr)
	}

	if cfg.COGEnabled {
		if err := convertToCOG(ctx, downloadPath, filePath, cfg.COGCompress); err != nil {
			slog.Error("COG conversion failed, keeping raw file",
				"file", fileName,
				"raw", downloadPath,
				"error", err,
			)
			return err
		}
		os.Remove(downloadPath)
	}

	return nil
}

func convertToCOG(ctx context.Context, srcPath, destPath, compress string) error {
	start := time.Now()

	tmpPath := destPath + ".cog.tmp"
	cmd := exec.CommandContext(ctx, "gdal_translate",
		"-of", "COG",
		"-co", "COMPRESS="+compress,
		"-co", "PREDICTOR=YES",
		"-co", "BLOCKSIZE=256",
		"-co", "OVERVIEW_RESAMPLING=AVERAGE",
		"-co", "OVERVIEWS=AUTO",
		srcPath, tmpPath,
	)
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("gdal_translate: %w", err)
	}

	if err := os.Chmod(tmpPath, 0o644); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("setting COG file permissions: %w", err)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming COG file: %w", err)
	}

	duration := time.Since(start)
	attrs := []any{
		"file", filepath.Base(destPath),
		"duration", duration.Round(time.Millisecond),
	}
	if info, err := os.Stat(destPath); err == nil {
		attrs = append(attrs, "size", info.Size())
	}
	slog.Info("COG conversion complete", attrs...)

	return nil
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
