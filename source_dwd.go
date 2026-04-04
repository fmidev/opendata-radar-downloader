package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var dwdFilePattern = regexp.MustCompile(`href="(composite_hx_(\d{8})_(\d{4})-hd5)"`)

// DWDSource fetches radar files from DWD's open data directory.
type DWDSource struct {
	URL string // directory URL, e.g. https://opendata.dwd.de/weather/radar/composite/hx/
}

func (s *DWDSource) Name() string { return "dwd_radar" }

func (s *DWDSource) FetchFiles(ctx context.Context, client *http.Client) ([]RadarFile, error) {
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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	cutoff := time.Now().UTC().Add(-1 * time.Hour)
	baseURL := strings.TrimRight(s.URL, "/") + "/"

	matches := dwdFilePattern.FindAllSubmatch(body, -1)

	var files []RadarFile
	for _, m := range matches {
		filename := string(m[1])
		dateStr := string(m[2])
		timeStr := string(m[3])

		t, err := time.Parse("20060102 1504", dateStr+" "+timeStr)
		if err != nil {
			continue
		}

		if t.Before(cutoff) {
			continue
		}

		files = append(files, RadarFile{
			Timestamp:   t,
			DownloadURL: baseURL + filename,
			IsHDF5:      true,
		})
	}

	return files, nil
}
