package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// eeNodeToRadar maps OPERA node codes to the Estonian API's radar names.
var eeNodeToRadar = map[string]string{
	"eehar": "Harku radar (HAR)",
	"eesur": "Sürgavere radar (SUR)",
}

// EESource fetches radar files from the Estonian Environment Agency (KAIA) API.
type EESource struct {
	URL         string
	RadarObject string // COMP or SCAN
	RadarNode   string // OPERA node code, e.g. eehar
}

func (s *EESource) Name() string {
	if s.RadarNode != "" {
		return "ee_radar_" + s.RadarNode
	}
	return "ee_radar"
}

func (s *EESource) FetchFiles(ctx context.Context, client *http.Client) ([]RadarFile, error) {
	cutoff := time.Now().UTC().Add(-1 * time.Hour)

	filter := s.buildFilter(cutoff)

	query := eeQuery{
		Filter:              filter,
		PageSize:            20,
		IncludeFileMetadata: true,
		Fields:              []string{"Timestamp", "Radar", "Phenomenon"},
	}

	var allFiles []RadarFile

	for page := 0; page < 5; page++ {
		files, nextBookmark, err := s.fetchPage(ctx, client, query)
		if err != nil {
			return nil, err
		}
		allFiles = append(allFiles, files...)

		if nextBookmark == "" {
			break
		}
		query.Bookmark = nextBookmark
	}

	return allFiles, nil
}

func (s *EESource) buildFilter(cutoff time.Time) map[string]any {
	children := []map[string]any{
		{"isEqual": map[string]string{"field": "$contentType", "value": "0102FB01"}},
	}

	if s.RadarObject == "COMP" || s.RadarObject == "" {
		// Composite precipitation
		children = append(children, map[string]any{
			"isEqual": map[string]string{"field": "Phenomenon", "value": "COMP"},
		})
	} else {
		// Individual radar CAPPI
		children = append(children, map[string]any{
			"isEqual": map[string]string{"field": "Phenomenon", "value": "CAP"},
		})
		if radarName, ok := eeNodeToRadar[s.RadarNode]; ok {
			children = append(children, map[string]any{
				"isEqual": map[string]string{"field": "Radar", "value": radarName},
			})
		}
	}

	children = append(children, map[string]any{
		"greaterThan": map[string]string{"field": "Timestamp", "value": cutoff.Format(time.RFC3339)},
	})

	return map[string]any{
		"and": map[string]any{
			"children": children,
		},
	}
}

func (s *EESource) fetchPage(ctx context.Context, client *http.Client, query eeQuery) ([]RadarFile, string, error) {
	body, err := json.Marshal(query)
	if err != nil {
		return nil, "", fmt.Errorf("marshaling query: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.URL, bytes.NewReader(body))
	if err != nil {
		return nil, "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result eeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, "", fmt.Errorf("decoding JSON: %w", err)
	}

	baseURL := strings.TrimSuffix(strings.TrimRight(s.URL, "/"), "/query")

	var files []RadarFile
	for _, doc := range result.Documents {
		ts, err := time.Parse(time.RFC3339, doc.Metadata.Timestamp)
		if err != nil {
			continue
		}

		if len(doc.FileMetadata) == 0 {
			continue
		}

		fm := doc.FileMetadata[0]
		downloadURL := fmt.Sprintf("%s/%d/files/%d", baseURL, doc.ID, fm.ID)

		files = append(files, RadarFile{
			Timestamp:   ts,
			DownloadURL: downloadURL,
			IsHDF5:      true,
		})
	}

	return files, result.NextBookmark, nil
}

// Query and response types for the Estonian API.

type eeQuery struct {
	Filter              map[string]any `json:"filter"`
	PageSize            int            `json:"pageSize"`
	IncludeFileMetadata bool           `json:"includeFileMetadata"`
	Fields              []string       `json:"fields"`
	Bookmark            string         `json:"bookmark,omitempty"`
}

type eeResponse struct {
	NumFound     int          `json:"numFound"`
	NextBookmark string       `json:"nextBookmark"`
	Documents    []eeDocument `json:"documents"`
}

type eeDocument struct {
	ID           int          `json:"id"`
	Metadata     eeMetadata   `json:"metadata"`
	FileMetadata []eeFileMeta `json:"fileMetadata"`
}

type eeMetadata struct {
	Timestamp  string `json:"Timestamp"`
	Radar      string `json:"Radar"`
	Phenomenon string `json:"Phenomenon"`
}

type eeFileMeta struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Size int64  `json:"size"`
}
