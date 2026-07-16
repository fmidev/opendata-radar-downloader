package main

import (
	"context"
	"net/http"
	"time"
)

// RadarFile is the common representation of a downloadable radar file.
type RadarFile struct {
	Timestamp   time.Time
	DownloadURL string
	Checksum    string // optional, e.g. "multihash-sha256:abc..." from MET Norway
	IsHDF5      bool   // true when the source knows the file is HDF5 (used when URL has no extension)
	Subdir      string // optional: write under OutputDir/Subdir (e.g. per-radar directory)
	Prefix      string // optional: override the filename prefix (defaults to cfg.FilePrefix)
	Raw         bool   // optional: store the file as downloaded, skipping the GDAL pipeline
}

// Source fetches the list of currently available radar files from a provider.
type Source interface {
	// Name returns a short identifier used in file naming and logs.
	Name() string
	// FetchFiles returns the radar files currently available from this source.
	FetchFiles(ctx context.Context, client *http.Client) ([]RadarFile, error)
}

func newSource(cfg *Config) Source {
	switch cfg.Source {
	case "metno":
		return &MetNoSource{URL: cfg.StacURL, Limit: cfg.StacLimit}
	case "smhi":
		return &SMHISource{BaseURL: cfg.SmhiURL}
	case "smhi_volume":
		return &SMHIVolumeSource{BaseURL: cfg.SmhiVolumeURL, Area: cfg.SmhiArea}
	case "dmi":
		return &DMISource{URL: cfg.DmiURL}
	case "ee":
		return &EESource{
			URL:         cfg.EeURL,
			RadarObject: cfg.RadarObject,
			RadarNode:   cfg.RadarNode,
		}
	case "imgw":
		return &IMGWSource{ListURL: cfg.ImgwListURL, ListPath: cfg.ImgwListPath, DownloadURL: cfg.ImgwDownloadURL}
	case "dwd":
		return &DWDSource{URL: cfg.DwdURL}
	case "chmi":
		return &CHMISource{URL: cfg.ChmiURL}
	case "fmi_s3":
		return &FMIS3Source{URL: cfg.FmiS3URL, Radars: cfg.FmiRadars}
	case "dmi_volume":
		return &DMIVolumeSource{URL: cfg.DmiVolumeURL, Radars: cfg.DmiRadars, FlatOutput: cfg.DmiFlatOutput}
	default:
		return &FMISource{URL: cfg.WFSURL, Prefix: cfg.FilePrefix}
	}
}
