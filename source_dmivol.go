package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// DMIVolumeSource fetches individual-radar ODIM HDF5 volume scans from DMI's
// STAC API (the "volume" collection). Like the fmi_s3 source, each configured
// radar's files are written to a separate subdirectory and stored raw as .h5
// (volume scans are not georeferenced rasters, so the GDAL pipeline is
// bypassed).
//
// STAC item ids are "<radar>_<YYYYMMDDHHMM>.vol.h5", e.g.
// "dkste_202606011945.vol.h5"; the radar is the id prefix. Each radar emits
// both doppler and fullRange scan types (one file per ~5 min slot), all of
// which are downloaded.
type DMIVolumeSource struct {
	URL      string   // STAC items URL
	Radars   []string // radar codes, e.g. dkste, dkrom
	FlatOutput bool   // write all radars to OUTPUT_DIR directly instead of per-radar subdirs
}

func (s *DMIVolumeSource) Name() string { return "dmi_volume" }

func (s *DMIVolumeSource) FetchFiles(ctx context.Context, client *http.Client) ([]RadarFile, error) {
	now := time.Now().UTC()
	cutoff := now.Add(-1 * time.Hour)

	wanted := make(map[string]bool, len(s.Radars))
	for _, r := range s.Radars {
		wanted[r] = true
	}

	// DMI requires a datetime filter to return recent data.
	reqURL := fmt.Sprintf("%s?datetime=%s/%s&limit=1000",
		s.URL,
		cutoff.Format(time.RFC3339),
		now.Format(time.RFC3339),
	)

	var files []RadarFile
	for page := 0; page < 10 && reqURL != ""; page++ {
		fc, next, err := s.fetchPage(ctx, client, reqURL)
		if err != nil {
			return nil, err
		}

		for _, f := range fc.Features {
			code := dmiRadarCode(f.ID)
			if !wanted[code] {
				continue
			}

			t, err := time.Parse(time.RFC3339, f.Properties.Datetime)
			if err != nil {
				continue
			}

			if f.Asset.Data.Href == "" {
				continue
			}

			rf := RadarFile{
				Timestamp:   t,
				DownloadURL: f.Asset.Data.Href,
				IsHDF5:      true,
				Raw:         true,
				Prefix:      code,
			}
			if !s.FlatOutput {
				rf.Subdir = code
			}
			files = append(files, rf)
		}

		reqURL = next
	}

	return files, nil
}

func (s *DMIVolumeSource) fetchPage(ctx context.Context, client *http.Client, reqURL string) (*dmiVolResponse, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("creating request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var fc dmiVolResponse
	if err := json.NewDecoder(resp.Body).Decode(&fc); err != nil {
		return nil, "", fmt.Errorf("decoding JSON: %w", err)
	}

	next := ""
	for _, l := range fc.Links {
		if l.Rel == "next" {
			next = l.Href
			break
		}
	}

	return &fc, next, nil
}

// dmiRadarCode extracts the radar code from a STAC item id such as
// "dkste_202606011945.vol.h5".
func dmiRadarCode(id string) string {
	if i := strings.IndexByte(id, '_'); i >= 0 {
		return id[:i]
	}
	return ""
}

type dmiVolResponse struct {
	Features []dmiVolFeature `json:"features"`
	Links    []dmiVolLink    `json:"links"`
}

type dmiVolFeature struct {
	ID         string           `json:"id"`
	Properties dmiVolProperties `json:"properties"`
	Asset      dmiVolAssetMap   `json:"asset"`
}

type dmiVolProperties struct {
	Datetime string `json:"datetime"`
	ScanType string `json:"scanType"`
}

type dmiVolAssetMap struct {
	Data dmiVolAsset `json:"data"`
}

type dmiVolAsset struct {
	Href string `json:"href"`
	Type string `json:"type"`
}

type dmiVolLink struct {
	Rel  string `json:"rel"`
	Href string `json:"href"`
}
