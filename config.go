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
	Source        string
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
	TargetEPSG    string
	Nodata        string
	Retention     time.Duration
	StacURL       string
	StacLimit     int
	SmhiURL       string
	DmiURL        string
}

func LoadConfig() (*Config, error) {
	source := envOrDefault("SOURCE", "fmi")
	switch source {
	case "fmi", "metno", "smhi", "dmi":
	default:
		return nil, fmt.Errorf("invalid SOURCE %q: must be fmi, metno, smhi, or dmi", source)
	}

	cfg := &Config{
		Source:        source,
		OutputDir:     envOrDefault("OUTPUT_DIR", "."),
		PollInterval:  60 * time.Second,
		ErrorInterval: 120 * time.Second,
		MaxBackoff:    5 * time.Minute,
		HTTPTimeout:   60 * time.Second,
		MaxRetries:    3,
		LogLevel:      slog.LevelInfo,
		COGEnabled:    true,
		COGCompress:   envOrDefault("COG_COMPRESS", "DEFLATE"),
		TargetEPSG:    os.Getenv("TARGET_EPSG"),
		Nodata:        os.Getenv("NODATA"),
		Retention:     24 * time.Hour,
	}

	switch source {
	case "fmi":
		cfg.StoredQuery = envOrDefault("STORED_QUERY", "fmi::radar::composite::dbz")
		cfg.WFSURL = os.Getenv("WFS_URL")
		if cfg.WFSURL == "" {
			params := url.Values{}
			params.Set("service", "WFS")
			params.Set("version", "2.0.0")
			params.Set("request", "GetFeature")
			params.Set("storedquery_id", cfg.StoredQuery)
			cfg.WFSURL = "https://opendata.fmi.fi/wfs?" + params.Encode()
		}
		cfg.FilePrefix = envOrDefault("FILE_PREFIX", strings.ReplaceAll(cfg.StoredQuery, "::", "_"))

	case "metno":
		cfg.StacURL = envOrDefault("STAC_URL", "https://radar-stacapi.met.no/v1/collections/Mosaic-Norway-v1/items")
		cfg.StacLimit = 10
		if v := os.Getenv("STAC_LIMIT"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("invalid STAC_LIMIT %q: %w", v, err)
			}
			cfg.StacLimit = n
		}
		cfg.FilePrefix = envOrDefault("FILE_PREFIX", "metno_radar")

	case "smhi":
		cfg.SmhiURL = envOrDefault("SMHI_URL", "https://opendata-download-radar.smhi.se/api/version/latest/area/sweden/product/comp")
		cfg.FilePrefix = envOrDefault("FILE_PREFIX", "smhi_radar")

	case "dmi":
		cfg.DmiURL = envOrDefault("DMI_URL", "https://opendataapi.dmi.dk/v1/radardata/collections/composite/items")
		cfg.FilePrefix = envOrDefault("FILE_PREFIX", "dmi_radar")
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

	if v := os.Getenv("RETENTION"); v != "" {
		if v == "0" || v == "none" {
			cfg.Retention = 0
		} else {
			d, err := time.ParseDuration(v)
			if err != nil {
				return nil, fmt.Errorf("invalid RETENTION %q: %w", v, err)
			}
			cfg.Retention = d
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
