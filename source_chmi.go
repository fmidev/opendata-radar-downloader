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

var chmiFilePattern = regexp.MustCompile(`href="(T_PANV23_C_OKPR_(\d{14})\.hdf)"`)

// CHMISource fetches radar files from CHMI's open data directory.
// Files are ODIM_H5 v2.4 PCAPPI 2 km composites (DBZH, uint8).
type CHMISource struct {
	URL string // directory URL, e.g. https://opendata.chmi.cz/meteorology/weather/radar/composite/pseudocappi2km/hdf5/
}

func (s *CHMISource) Name() string { return "chmi_radar" }

func (s *CHMISource) FetchFiles(ctx context.Context, client *http.Client) ([]RadarFile, error) {
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

	matches := chmiFilePattern.FindAllSubmatch(body, -1)

	var files []RadarFile
	for _, m := range matches {
		filename := string(m[1])
		tsStr := string(m[2])

		t, err := time.Parse("20060102150405", tsStr)
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
