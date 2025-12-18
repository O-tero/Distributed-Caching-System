# Tests
This directory contains:
- Bash-based endpoint smoke tests (`test-all-endpoints.sh`)
- Go integration/e2e test suites (`./integration`, `./e2e`)
- Optional Postman/Newman collection (`./collections/postman`)
- Helpers to inventory Encore endpoints (`../scripts/list-endpoints.sh`)

## Prerequisites
- Backend services running (typically via Encore)
- `curl`

Optional (recommended):
- `jq` (enables JSON assertions in the bash test runner)
- `newman` (runs Postman collections)

## Start the backend locally
If your repo uses the provided local runner:
```bash
./scripts/run_local.sh
```
Or start Encore directly:
```bash
encore run
```
Verify the dashboard is up:
```bash
curl -f http://localhost:9400/health
```

Verify the API gateway is responding (default `http://localhost:4000`):
```bash
curl -f http://localhost:4000/api/cache/metrics
```

## 1) Inventory all Encore endpoints
This lists endpoints extracted from `//encore:api` annotations in Go source.
```bash
./scripts/list-endpoints.sh
```
Include private endpoints too:
```bash
./scripts/list-endpoints.sh --include-private
```
JSON output (no `jq` required):
```bash
./scripts/list-endpoints.sh --json
```

## 2) Run the Bash endpoint test suite
The main script is:
- `tests/test-all-endpoints.sh`

It supports a single API base URL (default `http://localhost:4000`) and optional per-service overrides.

Note: In this repo, `http://localhost:9400` usually serves the Encore dashboard UI, while `http://localhost:4000` serves the API gateway.

### Run everything (recommended)
```bash
./tests/test-all-endpoints.sh
```
Verbose output:
```bash
./tests/test-all-endpoints.sh -v
```
Target a specific API base URL:
```bash
./tests/test-all-endpoints.sh --base-url http://localhost:4000
```

### Run a specific suite
```bash
./tests/test-all-endpoints.sh --service cache-manager
./tests/test-all-endpoints.sh --service invalidation
./tests/test-all-endpoints.sh --service warming
./tests/test-all-endpoints.sh --service monitoring
./tests/test-all-endpoints.sh --service dashboard
./tests/test-all-endpoints.sh --service integration
```

### Auth
If your APIs require auth, export a token:
```bash
export API_TOKEN_ADMIN="<your-token>"
./tests/test-all-endpoints.sh
```

### Useful environment variables
- `APP_URL` – Base URL for the Encore gateway (default `http://localhost:${ENCORE_PORT:-9400}`)
- `CACHE_MANAGER_URL`, `INVALIDATION_URL`, `WARMING_URL`, `MONITORING_URL` – Override base URLs per service
- `API_TOKEN_ADMIN` – Bearer token used by the script
- `CONNECT_TIMEOUT_SECONDS` – curl connect timeout (default `2`)
- `MAX_TIME_SECONDS` – curl max request time (default `10`)

## 3) Run Postman/Newman collection (optional)
Collection and environment files:
- `tests/collections/postman/distributed-cache-system.postman_collection.json`
- `tests/collections/postman/distributed-cache-system.postman_environment.json`

Install newman:
```bash
npm install -g newman
```

Run using the helper script:
```bash
BASE_URL=http://localhost:4000 AUTH_TOKEN="$API_TOKEN_ADMIN" ./tests/run-postman.sh
```

Notes:
- `AUTH_TOKEN` can be provided directly, or the script will fall back to `API_TOKEN_ADMIN`.
- The collection uses variables `baseUrl` and `authToken`.

## 4) Run Go integration tests
The Go tests under `tests/integration` and `tests/e2e` are HTTP-based and expect a running backend.

To avoid failing on machines without the services running, they are gated behind an env var.

### Run (skips by default)
```bash
go test ./tests/... -v
```

### Run against a live server
```bash
RUN_INTEGRATION_TESTS=1 BASE_URL=http://localhost:4000 go test ./tests/... -v
```

Auth for Go tests:
```bash
RUN_INTEGRATION_TESTS=1 \
  BASE_URL=http://localhost:4000 \
  API_TOKEN_ADMIN="<your-token>" \
  go test ./tests/... -v
```

Environment variables used by Go tests:
- `RUN_INTEGRATION_TESTS=1` – required to execute live HTTP tests
- `BASE_URL` (or `APP_URL`) – base URL for the running Encore server
- `AUTH_TOKEN` (or `API_TOKEN_ADMIN`) – bearer token

## Troubleshooting
### Getting HTTP 405 / HTML responses
That usually means you are hitting the wrong server/port (e.g. Encore dashboard/UI instead of the API gateway) or services are not running.
- Confirm `/health` is `200` on your target base URL.
- Run `./scripts/list-endpoints.sh` and compare paths.

### Database-backed endpoints
Some endpoints (e.g. invalidation audit logs) require the DB to be up. If you use the provided infra:
```bash
./scripts/run_local.sh
```
