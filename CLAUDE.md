# CLAUDE.md

## Project

OpenData Radar Downloader — Go program that continuously polls weather radar APIs and downloads GeoTIFF/HDF5 files. Runs in Docker.

**Repo**: git@github.com:fmidev/opendata-radar-downloader.git
**Module**: github.com/fmidev/fmi-radar-downloader
**Go version**: 1.24
**Dependencies**: stdlib only (shells out to GDAL for processing)

## Architecture

Single `package main` with multiple files:

- `main.go` — Entry point, signal handling, poll loop
- `config.go` — Env var parsing, `Config` struct, `LoadConfig()`
- `source.go` — `Source` interface, `RadarFile` struct, `newSource()` factory
- `source_*.go` — One file per data source (fmi, fmi_s3, metno, smhi, dmi, ee, dwd, chmi)
- `downloader.go` — Download, checksum, GDAL processing (reproject, COG, format conversion)
- `Dockerfile` — Multi-stage build, Alpine + gdal-tools + gdal-driver-hdf5, non-root user
- `.github/workflows/build.yml` — CI: go vet, build, push to ghcr.io

## Data sources

| Source | API type | File format | Config key |
|--------|----------|-------------|------------|
| FMI (Finland) | WFS XML | GeoTIFF | `fmi` |
| MET Norway | STAC JSON | COG GeoTIFF | `metno` |
| SMHI (Sweden) | REST JSON | GeoTIFF | `smhi` |
| DMI (Denmark) | STAC JSON | HDF5 ODIM | `dmi` |
| KAIA (Estonia) | POST JSON | HDF5 ODIM | `ee` |
| DWD (Germany) | HTML dir listing | HDF5 ODIM | `dwd` |
| CHMI (Czech Republic) | HTML dir listing | HDF5 ODIM | `chmi` |
| FMI volumes (Finland) | AWS S3 ListObjectsV2 (XML) | HDF5 ODIM PVOL (stored raw) | `fmi_s3` |

## Build & verify

```bash
go vet ./...
go build -o /dev/null .
docker build .
```

## Key patterns

- All config via environment variables — see README.md for full table
- `Source` interface: `Name() string` + `FetchFiles(ctx, client) ([]RadarFile, error)`
- Each source filters to last 1 hour of data
- GDAL pipeline: download → checksum → reproject → COG → atomic rename
- HDF5 files detected by URL suffix (`.h5`) or `RadarFile.IsHDF5` flag
- `RadarFile.Raw` stores the file as-is (skips GDAL); `RadarFile.Subdir`/`Prefix` route per-file output — `fmi_s3` uses them for one directory per radar
- `fmi_s3` lists a public S3 bucket anonymously via ListObjectsV2 XML over `net/http` (no AWS SDK); retention/temp-cleanup walk subdirs and handle `.h5`
- Source-specific env vars are scoped to their `case` block in `LoadConfig()`
- Docker image tag is `:main` (not `:latest`)

## Adding a new source

1. Create `source_xx.go` implementing the `Source` interface
2. Add config fields to `Config` struct in `config.go`
3. Add `"xx"` to valid sources switch and error message in `LoadConfig()`
4. Add `case "xx"` config block with source-specific env vars
5. Add `case "xx"` to `newSource()` in `source.go`
6. Update README.md, docker-compose.yml
