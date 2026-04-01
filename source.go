package main

import (
	"context"
	"net/http"
	"time"
)

// RadarFile is the common representation of a downloadable radar file.
type RadarFile struct {
	Timestamp   time.Time
	DownloadURL string
	Checksum    string // optional, e.g. "multihash-sha256:abc..." from MET Norway
}

// Source fetches the list of currently available radar files from a provider.
type Source interface {
	// Name returns a short identifier used in file naming and logs.
	Name() string
	// FetchFiles returns the radar files currently available from this source.
	FetchFiles(ctx context.Context, client *http.Client) ([]RadarFile, error)
}

func newSource(cfg *Config) Source {
	switch cfg.Source {
	case "metno":
		return &MetNoSource{URL: cfg.StacURL, Limit: cfg.StacLimit}
	case "smhi":
		return &SMHISource{BaseURL: cfg.SmhiURL}
	default:
		return &FMISource{URL: cfg.WFSURL, Prefix: cfg.FilePrefix}
	}
}
