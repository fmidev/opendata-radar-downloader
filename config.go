package main

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Source        string
	OutputDir     string
	PollInterval  time.Duration
	ErrorInterval time.Duration
	MaxBackoff    time.Duration
	StoredQuery   string
	WFSURL        string
	FilePrefix    string
	HTTPTimeout   time.Duration
	MaxRetries    int
	LogLevel      slog.Level
	COGEnabled    bool
	COGCompress   string
	TargetEPSG    string
	Nodata        string
	Retention     time.Duration
	StacURL       string
	StacLimit     int
	SmhiURL       string
	DmiURL        string
	EeURL         string
	RadarObject   string
	RadarNode     string
	DwdURL        string
	ChmiURL       string
	FmiS3URL      string
	FmiRadars     []string
	DmiVolumeURL    string
	DmiRadars       []string
	DmiFlatOutput   bool
	SmhiVolumeURL   string
	SmhiArea        string
	ImgwURL         string
	ImgwDownloadURL string
}

func LoadConfig() (*Config, error) {
	source := envOrDefault("SOURCE", "fmi")
	switch source {
	case "fmi", "fmi_s3", "metno", "smhi", "smhi_volume", "dmi", "dmi_volume", "ee", "dwd", "chmi", "imgw":
	default:
		return nil, fmt.Errorf("invalid SOURCE %q: must be fmi, fmi_s3, metno, smhi, smhi_volume, dmi, dmi_volume, ee, dwd, chmi, or imgw", source)
	}

	cfg := &Config{
		Source:        source,
		OutputDir:     envOrDefault("OUTPUT_DIR", "."),
		PollInterval:  60 * time.Second,
		ErrorInterval: 120 * time.Second,
		MaxBackoff:    5 * time.Minute,
		HTTPTimeout:   60 * time.Second,
		MaxRetries:    3,
		LogLevel:      slog.LevelInfo,
		COGEnabled:    true,
		COGCompress:   envOrDefault("COG_COMPRESS", "DEFLATE"),
		TargetEPSG:    os.Getenv("TARGET_EPSG"),
		Nodata:        os.Getenv("NODATA"),
		Retention:     24 * time.Hour,
	}

	switch source {
	case "fmi":
		cfg.StoredQuery = envOrDefault("STORED_QUERY", "fmi::radar::composite::dbz")
		cfg.WFSURL = os.Getenv("WFS_URL")
		if cfg.WFSURL == "" {
			params := url.Values{}
			params.Set("service", "WFS")
			params.Set("version", "2.0.0")
			params.Set("request", "GetFeature")
			params.Set("storedquery_id", cfg.StoredQuery)
			cfg.WFSURL = "https://opendata.fmi.fi/wfs?" + params.Encode()
		}
		cfg.FilePrefix = envOrDefault("FILE_PREFIX", strings.ReplaceAll(cfg.StoredQuery, "::", "_"))

	case "metno":
		cfg.StacURL = envOrDefault("STAC_URL", "https://radar-stacapi.met.no/v1/collections/Mosaic-Norway-v1/items")
		cfg.StacLimit = 10
		if v := os.Getenv("STAC_LIMIT"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("invalid STAC_LIMIT %q: %w", v, err)
			}
			cfg.StacLimit = n
		}
		cfg.FilePrefix = envOrDefault("FILE_PREFIX", "metno_radar")

	case "smhi":
		cfg.SmhiURL = envOrDefault("SMHI_URL", "https://opendata-download-radar.smhi.se/api/version/latest/area/sweden/product/comp")
		cfg.FilePrefix = envOrDefault("FILE_PREFIX", "smhi_radar")

	case "smhi_volume":
		cfg.SmhiVolumeURL = envOrDefault("SMHI_VOLUME_URL", "https://opendata-download-radar.smhi.se/api/version/latest")
		cfg.SmhiArea = os.Getenv("SMHI_AREA")
		if cfg.SmhiArea != "" {
			valid := false
			for _, a := range smhiVolumeAreas {
				if a == cfg.SmhiArea {
					valid = true
					break
				}
			}
			if !valid {
				return nil, fmt.Errorf("unknown SMHI_AREA %q; known areas: %s", cfg.SmhiArea, strings.Join(smhiVolumeAreas, ", "))
			}
			cfg.FilePrefix = envOrDefault("FILE_PREFIX", "smhi_radar_"+cfg.SmhiArea)
		} else {
			cfg.FilePrefix = envOrDefault("FILE_PREFIX", "smhi_radar_volume")
		}

	case "dmi":
		cfg.DmiURL = envOrDefault("DMI_URL", "https://opendataapi.dmi.dk/v1/radardata/collections/composite/items")
		cfg.FilePrefix = envOrDefault("FILE_PREFIX", "dmi_radar")

	case "ee":
		cfg.EeURL = envOrDefault("EE_URL", "https://avaandmed.keskkonnaportaal.ee/api/lists/active/items/query")
		cfg.RadarObject = envOrDefault("RADAR_OBJECT", "COMP")
		cfg.RadarNode = os.Getenv("RADAR_NODE")

		switch cfg.RadarObject {
		case "COMP", "SCAN", "VOL":
		default:
			return nil, fmt.Errorf("invalid RADAR_OBJECT %q: must be COMP, SCAN, or VOL", cfg.RadarObject)
		}

		if cfg.RadarNode != "" {
			if _, ok := eeNodeToRadar[cfg.RadarNode]; !ok {
				known := make([]string, 0, len(eeNodeToRadar))
				for k := range eeNodeToRadar {
					known = append(known, k)
				}
				return nil, fmt.Errorf("unknown RADAR_NODE %q for Estonian source; known nodes: %s", cfg.RadarNode, strings.Join(known, ", "))
			}
		}

		prefix := "ee_radar"
		if cfg.RadarNode != "" {
			prefix = "ee_radar_" + cfg.RadarNode
		}
		cfg.FilePrefix = envOrDefault("FILE_PREFIX", prefix)

	case "imgw":
		cfg.ImgwURL = envOrDefault("IMGW_URL", "https://danepubliczne.imgw.pl/api/data/product/id/COMPO_CMAX_250.comp.cmax")
		cfg.ImgwDownloadURL = envOrDefault("IMGW_DOWNLOAD_URL", "https://danepubliczne.imgw.pl/en/datastore/getfiledown/Oper/Polrad/Produkty/HVD/HVD_COMPO_CMAX_250.comp.cmax")
		cfg.FilePrefix = envOrDefault("FILE_PREFIX", "imgw_radar")
		if cfg.Nodata == "" {
			cfg.Nodata = "255"
		}

	case "dwd":
		cfg.DwdURL = envOrDefault("DWD_URL", "https://opendata.dwd.de/weather/radar/composite/hx/")
		cfg.FilePrefix = envOrDefault("FILE_PREFIX", "dwd_radar")
		if cfg.Nodata == "" {
			cfg.Nodata = "65535"
		}

	case "chmi":
		cfg.ChmiURL = envOrDefault("CHMI_URL", "https://opendata.chmi.cz/meteorology/weather/radar/composite/pseudocappi2km/hdf5/")
		cfg.FilePrefix = envOrDefault("FILE_PREFIX", "chmi_radar")
		if cfg.Nodata == "" {
			cfg.Nodata = "255"
		}

	case "fmi_s3":
		cfg.FmiS3URL = envOrDefault("FMI_S3_URL", "https://fmi-opendata-radar-volume-hdf5.s3.amazonaws.com/")

		raw := os.Getenv("FMI_RADARS")
		if strings.TrimSpace(raw) == "" {
			return nil, fmt.Errorf("FMI_RADARS is required for SOURCE=fmi_s3 (comma-separated site codes, e.g. fivih,fikor)")
		}
		for _, part := range strings.Split(raw, ",") {
			code := strings.ToLower(strings.TrimSpace(part))
			if code == "" {
				continue
			}
			if !validFmiRadar(code) {
				return nil, fmt.Errorf("invalid FMI radar site %q: expected a code like fivih, fikor, fikuo", code)
			}
			cfg.FmiRadars = append(cfg.FmiRadars, code)
		}
		if len(cfg.FmiRadars) == 0 {
			return nil, fmt.Errorf("FMI_RADARS contained no valid site codes")
		}

		// Files are stored per-radar (subdir + filename prefix); FilePrefix is
		// only used for startup logging here.
		cfg.FilePrefix = "fmi_radar"

	case "dmi_volume":
		cfg.DmiVolumeURL = envOrDefault("DMI_VOLUME_URL", "https://opendataapi.dmi.dk/v1/radardata/collections/volume/items")

		raw := os.Getenv("DMI_RADARS")
		if strings.TrimSpace(raw) == "" {
			return nil, fmt.Errorf("DMI_RADARS is required for SOURCE=dmi_volume (comma-separated radar codes, e.g. dkste,dkrom)")
		}
		for _, part := range strings.Split(raw, ",") {
			code := strings.ToLower(strings.TrimSpace(part))
			if code == "" {
				continue
			}
			if !validDmiRadar(code) {
				return nil, fmt.Errorf("invalid DMI radar code %q: expected a code like dkste, dkrom, dksin", code)
			}
			cfg.DmiRadars = append(cfg.DmiRadars, code)
		}
		if len(cfg.DmiRadars) == 0 {
			return nil, fmt.Errorf("DMI_RADARS contained no valid radar codes")
		}

		cfg.DmiFlatOutput = os.Getenv("DMI_FLAT_OUTPUT") == "true" || os.Getenv("DMI_FLAT_OUTPUT") == "1"

		// With flat output the radar code is in the filename prefix; without it
		// files go into per-radar subdirs. FilePrefix is only used for startup logging.
		cfg.FilePrefix = "dmi_radar"
	}

	if v := os.Getenv("COG_ENABLED"); v != "" {
		switch v {
		case "true", "1":
			cfg.COGEnabled = true
		case "false", "0":
			cfg.COGEnabled = false
		default:
			return nil, fmt.Errorf("invalid COG_ENABLED %q: must be true or false", v)
		}
	}

	if v := os.Getenv("RETENTION"); v != "" {
		if v == "0" || v == "none" {
			cfg.Retention = 0
		} else {
			d, err := time.ParseDuration(v)
			if err != nil {
				return nil, fmt.Errorf("invalid RETENTION %q: %w", v, err)
			}
			cfg.Retention = d
		}
	}

	if v := os.Getenv("POLL_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("invalid POLL_INTERVAL %q: %w", v, err)
		}
		cfg.PollInterval = d
	}

	if v := os.Getenv("ERROR_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("invalid ERROR_INTERVAL %q: %w", v, err)
		}
		cfg.ErrorInterval = d
	}

	if v := os.Getenv("MAX_BACKOFF"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("invalid MAX_BACKOFF %q: %w", v, err)
		}
		cfg.MaxBackoff = d
	}

	if v := os.Getenv("HTTP_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("invalid HTTP_TIMEOUT %q: %w", v, err)
		}
		cfg.HTTPTimeout = d
	}

	if v := os.Getenv("MAX_RETRIES"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid MAX_RETRIES %q: %w", v, err)
		}
		cfg.MaxRetries = n
	}

	if v := os.Getenv("LOG_LEVEL"); v != "" {
		switch v {
		case "debug":
			cfg.LogLevel = slog.LevelDebug
		case "info":
			cfg.LogLevel = slog.LevelInfo
		case "warn":
			cfg.LogLevel = slog.LevelWarn
		case "error":
			cfg.LogLevel = slog.LevelError
		default:
			return nil, fmt.Errorf("invalid LOG_LEVEL %q: must be debug, info, warn, or error", v)
		}
	}

	if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
		return nil, fmt.Errorf("cannot create output directory %q: %w", cfg.OutputDir, err)
	}

	return cfg, nil
}

// validRadarCode reports whether code looks like a "<country><site>" radar code
// — a two-letter country prefix followed by a three-letter site abbreviation,
// all lowercase (e.g. "fivih", "dkste").
func validRadarCode(code, country string) bool {
	if len(code) != 5 || !strings.HasPrefix(code, country) {
		return false
	}
	for _, r := range code {
		if r < 'a' || r > 'z' {
			return false
		}
	}
	return true
}

func validFmiRadar(code string) bool { return validRadarCode(code, "fi") }

func validDmiRadar(code string) bool { return validRadarCode(code, "dk") }

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
