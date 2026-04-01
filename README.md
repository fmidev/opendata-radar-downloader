# OpenData Radar Downloader

Continuously polls radar data APIs and downloads GeoTIFF files as they become available. Supports multiple data sources:

- **FMI Open Data** (Finnish Meteorological Institute) — WFS endpoint
- **MET Norway** (Norwegian Meteorological Institute) — STAC API
- **SMHI** (Swedish Meteorological and Hydrological Institute) — Open Data API

New radar images are published every 5 minutes. The downloader polls at a configurable interval (default 60 s), detects new files, and writes them to disk with atomic writes to prevent partial files.

## Output files

Files are named with the observation timestamp and source prefix:

```
20260331084500_fmi_radar_composite_dbz.tif
20260331084500_metno_radar.tif
20260331084500_smhi_radar.tif
```

## Quick start

### Docker Compose

```bash
docker compose up -d
```

This starts both FMI and MET Norway downloaders. Files are written to `./data/fmi/` and `./data/metno/`.

### Docker (single source)

FMI (default):
```bash
docker run -d \
  -v $(pwd)/data:/data \
  -e OUTPUT_DIR=/data \
  --restart unless-stopped \
  ghcr.io/fmidev/opendata-radar-downloader:main
```

MET Norway:
```bash
docker run -d \
  -v $(pwd)/data:/data \
  -e SOURCE=metno \
  -e OUTPUT_DIR=/data \
  --restart unless-stopped \
  ghcr.io/fmidev/opendata-radar-downloader:main
```

### Build from source

Requires Go 1.24+.

```bash
go build -o opendata-radar-downloader .
OUTPUT_DIR=./data ./opendata-radar-downloader
```

## Configuration

All configuration is via environment variables.

### General

| Variable | Default | Description |
|----------|---------|-------------|
| `SOURCE` | `fmi` | Data source: `fmi`, `metno`, or `smhi` |
| `OUTPUT_DIR` | `.` | Directory to write downloaded files |
| `FILE_PREFIX` | *(auto from source)* | Override filename prefix |
| `POLL_INTERVAL` | `60s` | Time between polls |
| `ERROR_INTERVAL` | `120s` | Initial wait after a failed poll |
| `MAX_BACKOFF` | `5m` | Maximum wait between retries on consecutive errors |
| `HTTP_TIMEOUT` | `60s` | HTTP client timeout |
| `MAX_RETRIES` | `3` | Max download retry attempts per file |
| `LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `TARGET_EPSG` | *(none)* | Reproject to target CRS, e.g. `4326` for WGS84 |
| `COG_ENABLED` | `true` | Convert downloads to Cloud Optimized GeoTIFF |
| `COG_COMPRESS` | `DEFLATE` | COG compression: `DEFLATE`, `LZW`, `ZSTD`, `NONE` |
| `RETENTION` | `24h` | Delete files older than this duration. Set to `0` or `none` to disable |

Duration values use Go duration syntax (e.g., `30s`, `2m`, `1m30s`).

### FMI-specific (SOURCE=fmi)

| Variable | Default | Description |
|----------|---------|-------------|
| `STORED_QUERY` | `fmi::radar::composite::dbz` | FMI stored query ID |
| `WFS_URL` | *(built from STORED_QUERY)* | Full WFS GetFeature URL |

### MET Norway-specific (SOURCE=metno)

| Variable | Default | Description |
|----------|---------|-------------|
| `STAC_URL` | `https://radar-stacapi.met.no/v1/collections/Mosaic-Norway-v1/items` | STAC API endpoint |
| `STAC_LIMIT` | `10` | Items per page |

### SMHI-specific (SOURCE=smhi)

| Variable | Default | Description |
|----------|---------|-------------|
| `SMHI_URL` | `https://opendata-download-radar.smhi.se/api/version/latest/area/sweden/product/comp` | SMHI API base URL |

### Examples

Different FMI radar product:
```bash
docker run -d \
  -v $(pwd)/data:/data \
  -e STORED_QUERY=fmi::radar::composite::rr1h \
  ghcr.io/fmidev/opendata-radar-downloader:main
```

MET Norway with COG re-optimization:
```bash
docker run -d \
  -v $(pwd)/data:/data \
  -e SOURCE=metno \
  -e COG_ENABLED=true \
  -e COG_COMPRESS=ZSTD \
  ghcr.io/fmidev/opendata-radar-downloader:main
```

## Features

- Multiple data sources: FMI WFS, MET Norway STAC API, and SMHI Open Data
- Automatic conversion to Cloud Optimized GeoTIFF (COG) via GDAL
- SHA256 checksum verification (MET Norway)
- Atomic file writes (temp file + rename) to prevent partial files
- Deduplication by checking existing files on disk
- Automatic retention-based cleanup of old files
- Retry with exponential backoff on download failures
- Escalating backoff on consecutive poll errors (up to `MAX_BACKOFF`)
- Graceful shutdown on SIGTERM/SIGINT
- Structured JSON logging via `slog`
- Health check via `.last_successful_poll` timestamp file
- Handles FMI OWS ExceptionReport responses
- No external Go dependencies (stdlib only, GDAL for COG conversion)

## Health check

The container includes a Docker HEALTHCHECK. On each successful poll cycle, a `.last_successful_poll` file is written to the output directory. The health check verifies this file was updated within the last 10 minutes.

## Building the Docker image

```bash
docker build -t opendata-radar-downloader .
```

The CI pipeline (GitHub Actions) automatically builds and pushes to `ghcr.io/fmidev/opendata-radar-downloader` on pushes to `main` and version tags.

## License

- FMI data: [FMI Open Data License](https://en.ilmatieteenlaitos.fi/open-data-licence)
- MET Norway data: [CC-BY-4.0](https://creativecommons.org/licenses/by/4.0/)
- SMHI data: [CC-BY-4.0](https://creativecommons.org/licenses/by/4.0/)
