package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// SMHISource fetches radar files from SMHI's open data API.
type SMHISource struct {
	BaseURL string // e.g. https://opendata-download-radar.smhi.se/api/version/latest/area/sweden/product/comp
}

func (s *SMHISource) Name() string { return "smhi_radar" }

func (s *SMHISource) FetchFiles(ctx context.Context, client *http.Client) ([]RadarFile, error) {
	now := time.Now().UTC()
	cutoff := now.Add(-1 * time.Hour)

	// Fetch today's files
	files, err := s.fetchDay(ctx, client, now)
	if err != nil {
		return nil, err
	}

	// If cutoff is in the previous day, also fetch yesterday's files
	if cutoff.Day() != now.Day() {
		yesterday, err := s.fetchDay(ctx, client, now.AddDate(0, 0, -1))
		if err != nil {
			return nil, err
		}
		files = append(yesterday, files...)
	}

	// Filter to last hour
	var recent []RadarFile
	for _, rf := range files {
		if rf.Timestamp.After(cutoff) {
			recent = append(recent, rf)
		}
	}

	return recent, nil
}

func (s *SMHISource) fetchDay(ctx context.Context, client *http.Client, day time.Time) ([]RadarFile, error) {
	url := fmt.Sprintf("%s/%d/%02d/%02d.json?format=tif",
		strings.TrimRight(s.BaseURL, "/"),
		day.Year(), day.Month(), day.Day())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // no data for this day yet
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var dayResp smhiDayResponse
	if err := json.NewDecoder(resp.Body).Decode(&dayResp); err != nil {
		return nil, fmt.Errorf("decoding JSON: %w", err)
	}

	var files []RadarFile
	for _, f := range dayResp.Files {
		t, err := time.Parse("2006-01-02 15:04", f.Valid)
		if err != nil {
			continue
		}

		for _, format := range f.Formats {
			if format.Key == "tif" && format.Link != "" {
				files = append(files, RadarFile{
					Timestamp:   t,
					DownloadURL: format.Link,
				})
			}
		}
	}

	return files, nil
}

type smhiDayResponse struct {
	Files []smhiFile `json:"files"`
}

type smhiFile struct {
	Key     string       `json:"key"`
	Valid   string       `json:"valid"`
	Updated string       `json:"updated"`
	Formats []smhiFormat `json:"formats"`
}

type smhiFormat struct {
	Key     string `json:"key"`
	Updated string `json:"updated"`
	Link    string `json:"link"`
}
