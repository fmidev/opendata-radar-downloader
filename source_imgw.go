package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var imgwFileRe = regexp.MustCompile(`\d{16}dBZ\.cmax\.h5`)

// IMGWSource fetches radar CMAX composite HDF5 files from IMGW-PIB.
// It uses the datastore getFilesList endpoint (returns live HTML listing)
// rather than the static product API (which can lag many hours).
type IMGWSource struct {
	ListURL     string // POST endpoint for file listing
	ListPath    string // path parameter for getFilesList
	DownloadURL string // base URL for file downloads
}

func (s *IMGWSource) Name() string { return "imgw_radar" }

func (s *IMGWSource) FetchFiles(ctx context.Context, client *http.Client) ([]RadarFile, error) {
	body := url.Values{
		"productType": {imgwProductType(s.ListPath)},
		"path":        {s.ListPath},
	}.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.ListURL, strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	html, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	cutoff := time.Now().UTC().Add(-1 * time.Hour)
	seen := make(map[string]bool)
	base := strings.TrimRight(s.DownloadURL, "/")

	var files []RadarFile
	for _, name := range imgwFileRe.FindAllString(string(html), -1) {
		if seen[name] {
			continue
		}
		seen[name] = true

		// Filename: "2026071612100000dBZ.cmax.h5" — first 14 chars are YYYYMMDDHHmmss.
		t, err := time.Parse("20060102150405", name[:14])
		if err != nil {
			continue
		}
		if !t.After(cutoff) {
			continue
		}
		files = append(files, RadarFile{
			Timestamp:   t,
			DownloadURL: base + "/" + name,
			IsHDF5:      true,
		})
	}

	return files, nil
}

// imgwProductType extracts the product type from the listing path.
// e.g. "Oper/Polrad/Produkty/HVD/HVD_COMPO_CMAX_250.comp.cmax" → "HVD_COMPO_CMAX_250.comp.cmax"
func imgwProductType(path string) string {
	parts := strings.Split(strings.TrimRight(path, "/"), "/")
	return parts[len(parts)-1]
}
