# OpenData Radar Downloader

Continuously polls radar data APIs and downloads GeoTIFF files as they become available. Supports multiple data sources:

- **FMI Open Data** (Finnish Meteorological Institute) — WFS endpoint
- **MET Norway** (Norwegian Meteorological Institute) — STAC API
- **SMHI** (Swedish Meteorological and Hydrological Institute) — Open Data API
- **DMI** (Danish Meteorological Institute) — STAC API (HDF5 ODIM format, auto-converted to GeoTIFF)
- **KAIA** (Estonian Environment Agency) — REST API (HDF5 ODIM format, auto-converted to GeoTIFF)
- **DWD** (Deutscher Wetterdienst) — Open Data directory (HDF5 ODIM, HX 250m reflectivity composite)
- **CHMI** (Czech Hydrometeorological Institute) — Open Data directory (HDF5 ODIM, PCAPPI 2km reflectivity composite)
- **FMI radar volumes** (Finnish Meteorological Institute) — public AWS S3 bucket (HDF5 ODIM polar volumes, individual radars, stored raw)
- **DMI radar volumes** (Danish Meteorological Institute) — STAC API (HDF5 ODIM volume scans, individual radars, stored raw)

New radar images are published every 5 minutes. The downloader polls at a configurable interval (default 60 s), detects new files, and writes them to disk with atomic writes to prevent partial files.

## Output files

Files are named with the observation timestamp and source prefix:

```
20260331084500_fmi_radar_composite_dbz.tif
20260331084500_metno_radar.tif
20260331084500_smhi_radar.tif
20260331084500_dmi_radar.tif
20260331084500_ee_radar.tif
20260331084500_ee_radar_eehar.tif
20260331084500_dwd_radar.tif
20260331084500_chmi_radar.tif
```

The `fmi_s3` and `dmi_volume` sources store raw ODIM HDF5 volume scans, with one directory per radar (`OUTPUT_DIR/<radar>/`):

```
fivih/20260331084500_fivih.h5
fikor/20260331084500_fikor.h5
dkste/20260331084500_dkste.h5
dkrom/20260331084500_dkrom.h5
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
| `SOURCE` | `fmi` | Data source: `fmi`, `fmi_s3`, `metno`, `smhi`, `dmi`, `dmi_volume`, `ee`, `dwd`, or `chmi` |
| `OUTPUT_DIR` | `.` | Directory to write downloaded files |
| `FILE_PREFIX` | *(auto from source)* | Override filename prefix |
| `POLL_INTERVAL` | `60s` | Time between polls |
| `ERROR_INTERVAL` | `120s` | Initial wait after a failed poll |
| `MAX_BACKOFF` | `5m` | Maximum wait between retries on consecutive errors |
| `HTTP_TIMEOUT` | `60s` | HTTP client timeout |
| `MAX_RETRIES` | `3` | Max download retry attempts per file |
| `LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `NODATA` | *(none)* | Set nodata value, e.g. `255` |
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

### DMI-specific (SOURCE=dmi)

| Variable | Default | Description |
|----------|---------|-------------|
| `DMI_URL` | `https://opendataapi.dmi.dk/v1/radardata/collections/composite/items` | DMI STAC API endpoint |

### Estonian-specific (SOURCE=ee)

| Variable | Default | Description |
|----------|---------|-------------|
| `EE_URL` | `https://avaandmed.keskkonnaportaal.ee/api/lists/active/items/query` | Estonian Environment Agency API endpoint |
| `RADAR_OBJECT` | `COMP` | `COMP` for composite, `SCAN` for individual radar |
| `RADAR_NODE` | *(none)* | OPERA node code to filter to a single radar when `RADAR_OBJECT=SCAN`. Omit to fetch all nodes. |

Available radar nodes: `eehar` (Harku), `eesur` (Sürgavere).

When `RADAR_OBJECT=SCAN` and `RADAR_NODE` is omitted, files from all nodes are downloaded into the same `OUTPUT_DIR` with the node code in the filename (e.g. `20260331084500_ee_radar_eehar.h5`).

### DWD-specific (SOURCE=dwd)

| Variable | Default | Description |
|----------|---------|-------------|
| `DWD_URL` | `https://opendata.dwd.de/weather/radar/composite/hx/` | DWD open data directory URL |

When `SOURCE=dwd`, `NODATA` defaults to `65535` (the HX composite is uint16).

### CHMI-specific (SOURCE=chmi)

| Variable | Default | Description |
|----------|---------|-------------|
| `CHMI_URL` | `https://opendata.chmi.cz/meteorology/weather/radar/composite/pseudocappi2km/hdf5/` | CHMI open data directory URL |

When `SOURCE=chmi`, `NODATA` defaults to `255` (the PCAPPI composite is uint8).

### FMI S3-specific (SOURCE=fmi_s3)

Downloads individual-radar ODIM HDF5 **polar volumes** (PVOL) directly from the
public AWS S3 bucket `s3://fmi-opendata-radar-volume-hdf5/`. The bucket is read
anonymously over HTTPS (S3 ListObjectsV2 REST API + object GETs — no AWS
credentials or SDK required).

| Variable | Default | Description |
|----------|---------|-------------|
| `FMI_RADARS` | *(required)* | Comma-separated radar site codes, e.g. `fivih,fikor` |
| `FMI_S3_URL` | `https://fmi-opendata-radar-volume-hdf5.s3.amazonaws.com/` | S3 bucket base URL |

Each radar's files are written to a separate subdirectory under `OUTPUT_DIR`
(`OUTPUT_DIR/<site>/`) and stored as **raw `.h5`** — polar volumes are not
georeferenced rasters, so the GDAL pipeline (`COG_ENABLED`, `TARGET_EPSG`,
`NODATA`, `COG_COMPRESS`) does not apply to this source.

Known radar sites: `fianj` (Anjalankoski), `fikan` (Kankaanpää), `fikau`
(Kauhava), `fikes` (Kesälahti), `fikor` (Korpo), `fikuo` (Kuopio), `filuo`
(Luosto), `finur` (Nurmes), `fipet` (Petäjävesi), `fiuta` (Utajärvi), `fivih`
(Vihti), `fivim` (Vimpeli).

### DMI volume-specific (SOURCE=dmi_volume)

Downloads individual-radar ODIM HDF5 **volume scans** from DMI's STAC API
(the `volume` collection). The API is queried anonymously over HTTPS (no API
key required) and paginated via STAC `next` links.

| Variable | Default | Description |
|----------|---------|-------------|
| `DMI_RADARS` | *(required)* | Comma-separated radar codes, e.g. `dkste,dkrom` |
| `DMI_VOLUME_URL` | `https://opendataapi.dmi.dk/v1/radardata/collections/volume/items` | STAC items endpoint |

Like `fmi_s3`, each radar's files are written to a separate subdirectory under
`OUTPUT_DIR` (`OUTPUT_DIR/<radar>/`) and stored as **raw `.h5`** — volume scans
are not georeferenced rasters, so the GDAL pipeline (`COG_ENABLED`,
`TARGET_EPSG`, `NODATA`, `COG_COMPRESS`) does not apply.

Each radar emits both `doppler` and `fullRange` scan types (one file per
~5 min slot); all are downloaded. The scan type is recorded inside the ODIM
file, not in the filename.

Known radars: `dkste` (Stevns), `dkrom` (Rømø), `dksin` (Sindal), `dkbor`
(Bornholm), `dksam` (Samsø).

### Examples

Different FMI radar product:
```bash
docker run -d \
  -v $(pwd)/data:/data \
  -e STORED_QUERY=fmi::radar::composite::rr1h \
  ghcr.io/fmidev/opendata-radar-downloader:main
```

Estonian composite:
```bash
docker run -d \
  -v $(pwd)/data:/data \
  -e SOURCE=ee \
  -e OUTPUT_DIR=/data \
  ghcr.io/fmidev/opendata-radar-downloader:main
```

Estonian individual radar (Harku):
```bash
docker run -d \
  -v $(pwd)/data:/data \
  -e SOURCE=ee \
  -e RADAR_OBJECT=SCAN \
  -e RADAR_NODE=eehar \
  -e OUTPUT_DIR=/data \
  ghcr.io/fmidev/opendata-radar-downloader:main
```

FMI radar volumes from S3 (Vihti + Korpo, one directory per radar):
```bash
docker run -d \
  -v $(pwd)/data:/data \
  -e SOURCE=fmi_s3 \
  -e FMI_RADARS=fivih,fikor \
  -e OUTPUT_DIR=/data \
  ghcr.io/fmidev/opendata-radar-downloader:main
```

DMI radar volumes from STAC (Stevns + Rømø, one directory per radar):
```bash
docker run -d \
  -v $(pwd)/data:/data \
  -e SOURCE=dmi_volume \
  -e DMI_RADARS=dkste,dkrom \
  -e OUTPUT_DIR=/data \
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

- Multiple data sources: FMI, MET Norway, SMHI, DMI, Estonian KAIA, DWD, CHMI, plus FMI and DMI radar volumes
- Anonymous AWS S3 access (ListObjectsV2 over HTTPS) — no AWS SDK or credentials
- Individual-radar downloads with one output directory per radar (`fmi_s3`, `dmi_volume`)
- Raw passthrough for ODIM volume scans (stored as `.h5`, bypassing GDAL)
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
- DMI data: [DMI Open Data](https://opendatadocs.dmi.govcloud.dk/)
- Estonian data: [Estonian Environment Agency Open Data](https://avaandmed.keskkonnaportaal.ee/)
- DWD data: [DWD Open Data](https://opendata.dwd.de/)
- CHMI data: [CHMI Open Data](https://opendata.chmi.cz/)
