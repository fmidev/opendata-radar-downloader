package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func DownloadIfNew(ctx context.Context, client *http.Client, rf RadarFile, cfg *Config) error {
	fileName := rf.Timestamp.Format("20060102150405") + "_" + cfg.FilePrefix + ".tif"
	filePath := filepath.Join(cfg.OutputDir, fileName)

	if _, err := os.Stat(filePath); err == nil {
		slog.Debug("file exists, skipping", "file", fileName)
		return nil
	}

	// Determine if we need GDAL processing
	isHDF5 := strings.HasSuffix(rf.DownloadURL, ".h5") || strings.HasSuffix(rf.DownloadURL, ".hdf5")
	needsProcessing := cfg.COGEnabled || cfg.TargetEPSG != "" || isHDF5

	downloadPath := filePath
	if needsProcessing {
		ext := ".raw"
		if isHDF5 {
			ext = ".h5"
		}
		downloadPath = filePath + ext
	}

	var lastErr error
	for attempt := 1; attempt <= cfg.MaxRetries; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		lastErr = downloadFile(ctx, client, rf.DownloadURL, downloadPath)
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

	if rf.Checksum != "" {
		if err := verifyChecksum(downloadPath, rf.Checksum); err != nil {
			os.Remove(downloadPath)
			return fmt.Errorf("checksum verification: %w", err)
		}
	}

	if needsProcessing {
		if err := processGDAL(ctx, downloadPath, filePath, cfg); err != nil {
			slog.Error("GDAL processing failed, keeping raw file",
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

func verifyChecksum(filePath, expected string) error {
	// MET Norway uses "multihash-sha256:{hex}" format
	expectedHex := expected
	if after, ok := strings.CutPrefix(expected, "multihash-sha256:"); ok {
		expectedHex = after
	} else if after, ok := strings.CutPrefix(expected, "sha256:"); ok {
		expectedHex = after
	} else {
		slog.Debug("unknown checksum format, skipping verification", "checksum", expected)
		return nil
	}

	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("opening file: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("reading file: %w", err)
	}

	actual := hex.EncodeToString(h.Sum(nil))
	if actual != expectedHex {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedHex, actual)
	}

	slog.Debug("checksum verified", "file", filepath.Base(filePath))
	return nil
}

func processGDAL(ctx context.Context, srcPath, destPath string, cfg *Config) error {
	start := time.Now()
	tmpPath := destPath + ".gdal.tmp"

	// Use gdalwarp when reprojecting (can also output COG directly),
	// otherwise use gdal_translate for COG/format conversion.
	gdalSrc := srcPath
	var cmd *exec.Cmd
	if cfg.TargetEPSG != "" {
		args := []string{"-t_srs", "EPSG:" + cfg.TargetEPSG}
		if cfg.Nodata != "" {
			args = append(args, "-srcnodata", cfg.Nodata, "-dstnodata", cfg.Nodata)
		}
		if cfg.COGEnabled {
			args = append(args,
				"-of", "COG",
				"-co", "COMPRESS="+cfg.COGCompress,
				"-co", "PREDICTOR=YES",
				"-co", "BLOCKSIZE=256",
				"-co", "OVERVIEW_RESAMPLING=NEAREST",
				"-co", "OVERVIEWS=AUTO",
			)
		}
		args = append(args, gdalSrc, tmpPath)
		cmd = exec.CommandContext(ctx, "gdalwarp", args...)
	} else if cfg.COGEnabled {
		args := []string{
			"-of", "COG",
			"-co", "COMPRESS=" + cfg.COGCompress,
			"-co", "PREDICTOR=YES",
			"-co", "BLOCKSIZE=256",
			"-co", "OVERVIEW_RESAMPLING=NEAREST",
			"-co", "OVERVIEWS=AUTO",
		}
		if cfg.Nodata != "" {
			args = append(args, "-a_nodata", cfg.Nodata)
		}
		args = append(args, gdalSrc, tmpPath)
		cmd = exec.CommandContext(ctx, "gdal_translate", args...)
	} else {
		args := []string{}
		if cfg.Nodata != "" {
			args = append(args, "-a_nodata", cfg.Nodata)
		}
		args = append(args, gdalSrc, tmpPath)
		// Format conversion only (e.g., HDF5 to GeoTIFF)
		cmd = exec.CommandContext(ctx, "gdal_translate", args...)
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("gdal processing: %w", err)
	}

	if err := os.Chmod(tmpPath, 0o644); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("setting file permissions: %w", err)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming output file: %w", err)
	}

	duration := time.Since(start)
	attrs := []any{
		"file", filepath.Base(destPath),
		"duration", duration.Round(time.Millisecond),
	}
	if info, err := os.Stat(destPath); err == nil {
		attrs = append(attrs, "size", info.Size())
	}
	slog.Info("GDAL processing complete", attrs...)

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
