package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// smhiVolumeAreas lists all individual SMHI radar station area keys.
var smhiVolumeAreas = []string{
	"angelholm",
	"atvidaberg",
	"balsta",
	"hemse",
	"hudiksvall",
	"karlskrona",
	"kiruna",
	"leksand",
	"lulea",
	"ornskoldsvik",
	"ostersund",
	"vara",
}

// SMHIVolumeSource fetches radar volume (qcvol) HDF5 files from SMHI's open data API.
type SMHIVolumeSource struct {
	BaseURL string // e.g. https://opendata-download-radar.smhi.se/api/version/latest
	Area    string // specific area key, or empty to fetch all areas
}

func (s *SMHIVolumeSource) Name() string {
	if s.Area != "" {
		return "smhi_radar_" + s.Area
	}
	return "smhi_radar_volume"
}

func (s *SMHIVolumeSource) FetchFiles(ctx context.Context, client *http.Client) ([]RadarFile, error) {
	areas := smhiVolumeAreas
	if s.Area != "" {
		areas = []string{s.Area}
	}

	now := time.Now().UTC()
	cutoff := now.Add(-1 * time.Hour)

	var allFiles []RadarFile
	for _, area := range areas {
		files, err := s.fetchArea(ctx, client, area, now, cutoff)
		if err != nil {
			return nil, fmt.Errorf("smhi area %s: %w", area, err)
		}
		allFiles = append(allFiles, files...)
	}
	return allFiles, nil
}

func (s *SMHIVolumeSource) fetchArea(ctx context.Context, client *http.Client, area string, now, cutoff time.Time) ([]RadarFile, error) {
	files, err := s.fetchDay(ctx, client, area, now)
	if err != nil {
		return nil, err
	}
	if cutoff.Day() != now.Day() {
		yesterday, err := s.fetchDay(ctx, client, area, now.AddDate(0, 0, -1))
		if err != nil {
			return nil, err
		}
		files = append(yesterday, files...)
	}

	var recent []RadarFile
	for _, rf := range files {
		if rf.Timestamp.After(cutoff) {
			recent = append(recent, rf)
		}
	}
	return recent, nil
}

func (s *SMHIVolumeSource) fetchDay(ctx context.Context, client *http.Client, area string, day time.Time) ([]RadarFile, error) {
	url := fmt.Sprintf("%s/area/%s/product/qcvol/%d/%02d/%02d.json",
		strings.TrimRight(s.BaseURL, "/"),
		area, day.Year(), day.Month(), day.Day())

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
		return nil, nil
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
			if format.Key == "h5" && format.Link != "" {
				files = append(files, RadarFile{
					Timestamp:   t,
					DownloadURL: format.Link,
					Raw:         true,
					Prefix:      "smhi_radar_" + area,
				})
			}
		}
	}
	return files, nil
}
