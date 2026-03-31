package main

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	OutputDir     string
	PollInterval  time.Duration
	ErrorInterval time.Duration
	MaxBackoff    time.Duration
	StoredQuery   string
	WFSURL        string
	FilePrefix    string
	HTTPTimeout   time.Duration
	MaxRetries    int
	LogLevel      slog.Level
	COGEnabled    bool
	COGCompress   string
}

func LoadConfig() (*Config, error) {
	storedQuery := envOrDefault("STORED_QUERY", "fmi::radar::composite::dbz")

	wfsURL := os.Getenv("WFS_URL")
	if wfsURL == "" {
		params := url.Values{}
		params.Set("service", "WFS")
		params.Set("version", "2.0.0")
		params.Set("request", "GetFeature")
		params.Set("storedquery_id", storedQuery)
		wfsURL = "https://opendata.fmi.fi/wfs?" + params.Encode()
	}

	cfg := &Config{
		OutputDir:     envOrDefault("OUTPUT_DIR", "."),
		StoredQuery:   storedQuery,
		WFSURL:        wfsURL,
		FilePrefix:    strings.ReplaceAll(storedQuery, "::", "_"),
		PollInterval:  60 * time.Second,
		ErrorInterval: 120 * time.Second,
		MaxBackoff:    5 * time.Minute,
		HTTPTimeout:   60 * time.Second,
		MaxRetries:    3,
		LogLevel:      slog.LevelInfo,
		COGEnabled:    true,
		COGCompress:   envOrDefault("COG_COMPRESS", "DEFLATE"),
	}

	if v := os.Getenv("COG_ENABLED"); v != "" {
		switch v {
		case "true", "1":
			cfg.COGEnabled = true
		case "false", "0":
			cfg.COGEnabled = false
		default:
			return nil, fmt.Errorf("invalid COG_ENABLED %q: must be true or false", v)
		}
	}

	if v := os.Getenv("POLL_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("invalid POLL_INTERVAL %q: %w", v, err)
		}
		cfg.PollInterval = d
	}

	if v := os.Getenv("ERROR_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("invalid ERROR_INTERVAL %q: %w", v, err)
		}
		cfg.ErrorInterval = d
	}

	if v := os.Getenv("MAX_BACKOFF"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("invalid MAX_BACKOFF %q: %w", v, err)
		}
		cfg.MaxBackoff = d
	}

	if v := os.Getenv("HTTP_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("invalid HTTP_TIMEOUT %q: %w", v, err)
		}
		cfg.HTTPTimeout = d
	}

	if v := os.Getenv("MAX_RETRIES"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid MAX_RETRIES %q: %w", v, err)
		}
		cfg.MaxRetries = n
	}

	if v := os.Getenv("LOG_LEVEL"); v != "" {
		switch v {
		case "debug":
			cfg.LogLevel = slog.LevelDebug
		case "info":
			cfg.LogLevel = slog.LevelInfo
		case "warn":
			cfg.LogLevel = slog.LevelWarn
		case "error":
			cfg.LogLevel = slog.LevelError
		default:
			return nil, fmt.Errorf("invalid LOG_LEVEL %q: must be debug, info, warn, or error", v)
		}
	}

	if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
		return nil, fmt.Errorf("cannot create output directory %q: %w", cfg.OutputDir, err)
	}

	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
