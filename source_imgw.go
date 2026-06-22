package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// IMGWSource fetches radar composite (CMAX) files from IMGW-PIB's open data API.
type IMGWSource struct {
	URL         string // API listing URL
	DownloadURL string // base URL for file downloads (different path from API URLs)
}

func (s *IMGWSource) Name() string { return "imgw_radar" }

func (s *IMGWSource) FetchFiles(ctx context.Context, client *http.Client) ([]RadarFile, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var entries []imgwEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("decoding JSON: %w", err)
	}

	cutoff := time.Now().UTC().Add(-1 * time.Hour)

	var files []RadarFile
	for _, e := range entries {
		if !strings.HasSuffix(e.File, ".cmax.h5") {
			continue
		}
		// Filename format: "2026062204300000dBZ.cmax.h5"
		// First 14 chars are YYYYMMDDHHmmss; next two are trailing zeros.
		if len(e.File) < 14 {
			continue
		}
		t, err := time.Parse("20060102150405", e.File[:14])
		if err != nil {
			continue
		}
		if !t.After(cutoff) {
			continue
		}
		files = append(files, RadarFile{
			Timestamp:   t,
			DownloadURL: strings.TrimRight(s.DownloadURL, "/") + "/" + e.File,
			IsHDF5:      true,
		})
	}

	return files, nil
}

type imgwEntry struct {
	File string `json:"file"`
	URL  string `json:"url"`
}
