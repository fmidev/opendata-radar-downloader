package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// MetNoSource fetches radar files from MET Norway's STAC API.
type MetNoSource struct {
	URL   string
	Limit int
}

func (s *MetNoSource) Name() string { return "metno_radar" }

func (s *MetNoSource) FetchFiles(ctx context.Context, client *http.Client) ([]RadarFile, error) {
	files, _, err := s.fetchPage(ctx, client, s.URL)
	if err != nil {
		return nil, err
	}

	// Only return files from the last hour
	cutoff := time.Now().Add(-1 * time.Hour)
	var recent []RadarFile
	for _, rf := range files {
		if rf.Timestamp.After(cutoff) {
			recent = append(recent, rf)
		}
	}

	return recent, nil
}

func (s *MetNoSource) fetchPage(ctx context.Context, client *http.Client, url string) ([]RadarFile, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "opendata-radar-downloader/1.0 (https://github.com/fmidev/opendata-radar-downloader)")

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var fc stacFeatureCollection
	if err := json.NewDecoder(resp.Body).Decode(&fc); err != nil {
		return nil, "", fmt.Errorf("decoding JSON: %w", err)
	}

	var files []RadarFile
	for _, f := range fc.Features {
		t, err := time.Parse(time.RFC3339, f.Properties.Datetime)
		if err != nil {
			return nil, "", fmt.Errorf("parsing datetime %q: %w", f.Properties.Datetime, err)
		}

		data, ok := f.Assets["data"]
		if !ok {
			continue
		}

		rf := RadarFile{
			Timestamp:   t,
			DownloadURL: data.Href,
		}
		if data.Checksum != "" {
			rf.Checksum = data.Checksum
		}

		files = append(files, rf)
	}

	// Find "next" pagination link
	var nextURL string
	for _, link := range fc.Links {
		if link.Rel == "next" {
			nextURL = link.Href
			break
		}
	}

	return files, nextURL, nil
}

type stacFeatureCollection struct {
	Features []stacFeature `json:"features"`
	Links    []stacLink    `json:"links"`
}

type stacFeature struct {
	Properties stacProperties       `json:"properties"`
	Assets     map[string]stacAsset `json:"assets"`
}

type stacProperties struct {
	Datetime string `json:"datetime"`
}

type stacAsset struct {
	Href     string `json:"href"`
	Checksum string `json:"file:checksum"`
}

type stacLink struct {
	Rel  string `json:"rel"`
	Href string `json:"href"`
}
