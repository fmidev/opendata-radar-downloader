package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// DMISource fetches radar files from DMI's STAC API.
type DMISource struct {
	URL string
}

func (s *DMISource) Name() string { return "dmi_radar" }

func (s *DMISource) FetchFiles(ctx context.Context, client *http.Client) ([]RadarFile, error) {
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

	var fc dmiFeatureCollection
	if err := json.NewDecoder(resp.Body).Decode(&fc); err != nil {
		return nil, fmt.Errorf("decoding JSON: %w", err)
	}

	cutoff := time.Now().Add(-1 * time.Hour)

	var files []RadarFile
	for _, f := range fc.Features {
		t, err := time.Parse(time.RFC3339, f.Properties.Datetime)
		if err != nil {
			continue
		}

		if t.Before(cutoff) {
			continue
		}

		if f.Asset.Data.Href == "" {
			continue
		}

		files = append(files, RadarFile{
			Timestamp:   t,
			DownloadURL: f.Asset.Data.Href,
		})
	}

	return files, nil
}

type dmiFeatureCollection struct {
	Features []dmiFeature `json:"features"`
}

type dmiFeature struct {
	Properties dmiProperties `json:"properties"`
	Asset      dmiAssetMap   `json:"asset"`
}

type dmiProperties struct {
	Datetime string `json:"datetime"`
	ScanType string `json:"scanType"`
}

type dmiAssetMap struct {
	Data dmiAsset `json:"data"`
}

type dmiAsset struct {
	Href string `json:"href"`
	Type string `json:"type"`
}
