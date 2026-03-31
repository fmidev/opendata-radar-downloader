package main

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"time"
)

// FMISource fetches radar files from the FMI WFS endpoint.
type FMISource struct {
	URL    string
	Prefix string
}

func (s *FMISource) Name() string { return s.Prefix }

func (s *FMISource) FetchFiles(ctx context.Context, client *http.Client) ([]RadarFile, error) {
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

	var files []RadarFile
	decoder := xml.NewDecoder(resp.Body)

	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("parsing XML: %w", err)
		}
		if token == nil {
			break
		}

		se, ok := token.(xml.StartElement)
		if !ok {
			continue
		}

		switch se.Name.Local {
		case "ExceptionReport":
			var exc owsException
			if err := decoder.DecodeElement(&exc, &se); err != nil {
				return nil, fmt.Errorf("decoding OWS exception: %w", err)
			}
			return nil, fmt.Errorf("WFS exception: %s", exc.ExceptionText)

		case "member":
			var xm xmlMember
			if err := decoder.DecodeElement(&xm, &se); err != nil {
				return nil, fmt.Errorf("decoding member: %w", err)
			}

			t, err := time.Parse(time.RFC3339, xm.Observation.PhenomenonTime.TimeInstant.TimePosition)
			if err != nil {
				return nil, fmt.Errorf("parsing time %q: %w", xm.Observation.PhenomenonTime.TimeInstant.TimePosition, err)
			}

			files = append(files, RadarFile{
				Timestamp:   t,
				DownloadURL: xm.Observation.Result.GridCoverage.RangeSet.File.FileReference,
			})
		}
	}

	return files, nil
}

type xmlMember struct {
	Observation struct {
		PhenomenonTime struct {
			TimeInstant struct {
				TimePosition string `xml:"timePosition"`
			} `xml:"TimeInstant"`
		} `xml:"phenomenonTime"`
		Result struct {
			GridCoverage struct {
				RangeSet struct {
					File struct {
						FileReference string `xml:"fileReference"`
					} `xml:"File"`
				} `xml:"rangeSet"`
			} `xml:"RectifiedGridCoverage"`
		} `xml:"result"`
	} `xml:"GridSeriesObservation"`
}

type owsException struct {
	ExceptionText string `xml:"Exception>ExceptionText"`
}
