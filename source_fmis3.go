package main

import (
	"context"
	"encoding/xml"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// FMIS3Source fetches ODIM HDF5 polar volume (PVOL) files for individual FMI
// radars directly from the public AWS S3 bucket
// (s3://fmi-opendata-radar-volume-hdf5/). The bucket is anonymously readable
// over HTTP, so it is listed via the S3 ListObjectsV2 REST API (XML) and
// objects are fetched with plain GETs — no AWS SDK required.
//
// Objects are keyed as: YYYY/MM/DD/<site>/YYYYMMDDHHMM_<site>_PVOL.h5
//
// Each configured radar's files are written to a separate subdirectory and
// stored as raw .h5 (polar volumes are not georeferenced rasters, so the GDAL
// COG/reproject pipeline is bypassed).
type FMIS3Source struct {
	URL    string   // bucket base URL, e.g. https://fmi-opendata-radar-volume-hdf5.s3.amazonaws.com/
	Radars []string // site codes, e.g. fivih, fikor
}

func (s *FMIS3Source) Name() string { return "fmi_s3" }

func (s *FMIS3Source) FetchFiles(ctx context.Context, client *http.Client) ([]RadarFile, error) {
	now := time.Now().UTC()
	cutoff := now.Add(-1 * time.Hour)
	days := dayPrefixes(cutoff, now)

	var files []RadarFile
	var firstErr error
	attempts, failures := 0, 0

	for _, site := range s.Radars {
		for _, day := range days {
			attempts++
			f, err := s.listDay(ctx, client, site, day, cutoff)
			if err != nil {
				failures++
				if firstErr == nil {
					firstErr = err
				}
				slog.Warn("listing radar day failed",
					"source", s.Name(),
					"radar", site,
					"day", day.Format("2006-01-02"),
					"error", err,
				)
				continue
			}
			files = append(files, f...)
		}
	}

	// Only fail the whole poll (triggering backoff) when every listing failed,
	// e.g. S3 is unreachable. Otherwise return what we have so one bad radar or
	// day does not stall the others.
	if attempts > 0 && failures == attempts {
		return nil, firstErr
	}

	return files, nil
}

// listDay lists one radar's objects for a single UTC day, keeping only those at
// or after cutoff.
func (s *FMIS3Source) listDay(ctx context.Context, client *http.Client, site string, day, cutoff time.Time) ([]RadarFile, error) {
	prefix := day.Format("2006/01/02/") + site + "/"

	// Skip objects older than cutoff cheaply: keys sort lexicographically by
	// timestamp, so start-after the earliest wanted minute on this day.
	earliest := cutoff
	if startOfDay := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC); earliest.Before(startOfDay) {
		earliest = startOfDay
	}
	startAfter := prefix + earliest.Format("200601021504")

	baseURL := strings.TrimRight(s.URL, "/") + "/"

	var files []RadarFile
	token := ""
	for page := 0; page < 5; page++ {
		result, err := s.listObjects(ctx, client, prefix, startAfter, token)
		if err != nil {
			return nil, err
		}

		for _, obj := range result.Contents {
			ts, ok := parseS3KeyTimestamp(obj.Key)
			if !ok || ts.Before(cutoff) {
				continue
			}
			files = append(files, RadarFile{
				Timestamp:   ts,
				DownloadURL: baseURL + obj.Key,
				IsHDF5:      true,
				Raw:         true,
				Subdir:      site,
				Prefix:      site,
			})
		}

		if !result.IsTruncated || result.NextContinuationToken == "" {
			break
		}
		token = result.NextContinuationToken
	}

	return files, nil
}

func (s *FMIS3Source) listObjects(ctx context.Context, client *http.Client, prefix, startAfter, token string) (*s3ListResult, error) {
	params := url.Values{}
	params.Set("list-type", "2")
	params.Set("prefix", prefix)
	if token != "" {
		// start-after is ignored once a continuation token is supplied.
		params.Set("continuation-token", token)
	} else if startAfter != "" {
		params.Set("start-after", startAfter)
	}

	reqURL := strings.TrimRight(s.URL, "/") + "/?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
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

	var result s3ListResult
	if err := xml.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding S3 listing: %w", err)
	}

	return &result, nil
}

// dayPrefixes returns the UTC day(s) spanned by [cutoff, now]. Since the two
// are at most an hour apart this is one day, or two when the window crosses
// midnight UTC.
func dayPrefixes(cutoff, now time.Time) []time.Time {
	startDay := time.Date(cutoff.Year(), cutoff.Month(), cutoff.Day(), 0, 0, 0, 0, time.UTC)
	endDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	days := []time.Time{startDay}
	if !endDay.Equal(startDay) {
		days = append(days, endDay)
	}
	return days
}

// parseS3KeyTimestamp extracts the observation time from a key whose basename
// begins with a 12-digit YYYYMMDDHHMM timestamp, e.g.
// "2026/05/31/fikor/202605311200_fikor_PVOL.h5".
func parseS3KeyTimestamp(key string) (time.Time, bool) {
	base := key
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	if len(base) < 13 || base[12] != '_' {
		return time.Time{}, false
	}
	t, err := time.Parse("200601021504", base[:12])
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// s3ListResult is the subset of the S3 ListObjectsV2 XML response we use.
type s3ListResult struct {
	IsTruncated           bool       `xml:"IsTruncated"`
	NextContinuationToken string     `xml:"NextContinuationToken"`
	Contents              []s3Object `xml:"Contents"`
}

type s3Object struct {
	Key  string `xml:"Key"`
	Size int64  `xml:"Size"`
}
