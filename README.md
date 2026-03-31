# FMI Radar Downloader

Continuously polls the [FMI Open Data](https://en.ilmatieteenlaitos.fi/open-data) WFS endpoint and downloads radar composite GeoTIFF files as they become available.

New radar images are published every 5 minutes. The downloader polls at a configurable interval (default 60 s), detects new files, and writes them to disk with atomic writes to prevent partial files.

## Output files

Files are named with the observation timestamp and stored query:

```
20260331084500_fmi_radar_composite_dbz.tif
```

## Quick start

### Docker Compose

```bash
docker compose up -d
```

Files are written to `./data/`.

### Docker

```bash
docker run -d \
  -v $(pwd)/data:/data \
  -e OUTPUT_DIR=/data \
  --restart unless-stopped \
  ghcr.io/fmidev/fmi-radar-downloader:latest
```

### Build from source

Requires Go 1.24+.

```bash
go build -o fmi-radar-downloader .
OUTPUT_DIR=./data ./fmi-radar-downloader
```

## Configuration

All configuration is via environment variables.

| Variable | Default | Description |
|----------|---------|-------------|
| `STORED_QUERY` | `fmi::radar::composite::dbz` | FMI stored query ID |
| `WFS_URL` | *(built from STORED_QUERY)* | Full WFS GetFeature URL (overrides STORED_QUERY for URL construction) |
| `OUTPUT_DIR` | `.` | Directory to write downloaded files |
| `POLL_INTERVAL` | `60s` | Time between polls |
| `ERROR_INTERVAL` | `120s` | Initial wait after a failed poll |
| `MAX_BACKOFF` | `5m` | Maximum wait between retries on consecutive errors |
| `HTTP_TIMEOUT` | `60s` | HTTP client timeout |
| `MAX_RETRIES` | `3` | Max download retry attempts per file |
| `LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error` |

Duration values use Go duration syntax (e.g., `30s`, `2m`, `1m30s`).

### Example: different radar product

```bash
docker run -d \
  -v $(pwd)/data:/data \
  -e OUTPUT_DIR=/data \
  -e STORED_QUERY=fmi::radar::composite::rr1h \
  ghcr.io/fmidev/fmi-radar-downloader:latest
```

## Features

- Polls FMI WFS endpoint and downloads new GeoTIFF files automatically
- Atomic file writes (temp file + rename) to prevent partial files
- Deduplication by checking existing files on disk
- Retry with exponential backoff on download failures
- Escalating backoff on consecutive poll errors (up to `MAX_BACKOFF`)
- Graceful shutdown on SIGTERM/SIGINT
- Structured JSON logging via `slog`
- Health check via `.last_successful_poll` timestamp file
- Handles FMI OWS ExceptionReport responses
- No external Go dependencies (stdlib only)

## Health check

The container includes a Docker HEALTHCHECK. On each successful poll cycle, a `.last_successful_poll` file is written to the output directory. The health check verifies this file was updated within the last 10 minutes.

## Building the Docker image

```bash
docker build -t fmi-radar-downloader .
```

The CI pipeline (GitHub Actions) automatically builds and pushes to `ghcr.io/fmidev/fmi-radar-downloader` on pushes to `main` and version tags.

## License

See [FMI Open Data License](https://en.ilmatieteenlaitos.fi/open-data-licence) for data usage terms.
