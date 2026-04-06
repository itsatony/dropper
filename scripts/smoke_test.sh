#!/usr/bin/env bash
# dropper container smoke test
# Builds the image, starts a container, runs E2E checks, cleans up.
set -euo pipefail

# --- Constants ---
readonly DOCKER_IMAGE="vaudience/dropper"
readonly DOCKER_TAG="smoke-test"
readonly BASE_PORT=18080
readonly MAX_WAIT_SECONDS=30
readonly WAIT_INTERVAL=1

# Test auth credential (not a real secret — placeholder for smoke test)
readonly SMOKE_AUTH="changeme-smoke-test"

# Unique container name to avoid collisions
readonly CONTAINER_NAME="dropper-smoke-$$"

# Test port: base + last 4 digits of PID to reduce collisions
readonly TEST_PORT=$(( BASE_PORT + ($$ % 1000) ))
readonly BASE_URL="http://localhost:${TEST_PORT}"

# --- State ---
PASS=0
FAIL=0

# --- Color output ---
red()   { printf '\033[0;31m  FAIL: %s\033[0m\n' "$*"; }
green() { printf '\033[0;32m  PASS: %s\033[0m\n' "$*"; }
bold()  { printf '\033[1m%s\033[0m\n' "$*"; }

# --- Cleanup on exit ---
cleanup() {
    bold "Cleaning up..."
    docker stop "${CONTAINER_NAME}" 2>/dev/null || true
    docker rm -f "${CONTAINER_NAME}" 2>/dev/null || true
}
trap cleanup EXIT

# --- Test helpers ---
assert_contains() {
    local label="$1" response="$2" expected="$3"
    if echo "${response}" | grep -q "${expected}"; then
        green "${label}"
        PASS=$((PASS + 1))
    else
        red "${label} — expected '${expected}' in response"
        FAIL=$((FAIL + 1))
    fi
}

assert_status() {
    local label="$1" status="$2" expected="$3"
    if [ "${status}" = "${expected}" ]; then
        green "${label}"
        PASS=$((PASS + 1))
    else
        red "${label} — expected status ${expected}, got ${status}"
        FAIL=$((FAIL + 1))
    fi
}

# --- Check docker is available ---
if ! command -v docker &>/dev/null; then
    echo "Error: docker not found. Cannot run smoke test."
    exit 1
fi

# --- Build image ---
bold "=== dropper smoke test ==="
echo ""
bold "Building image ${DOCKER_IMAGE}:${DOCKER_TAG}..."

docker build -q -t "${DOCKER_IMAGE}:${DOCKER_TAG}" . >/dev/null

bold "Starting container ${CONTAINER_NAME} on port ${TEST_PORT}..."

# Create temp data dir
TEMP_DATA="$(mktemp -d)"
trap 'cleanup; rm -rf "${TEMP_DATA}"' EXIT

docker run -d \
    --name "${CONTAINER_NAME}" \
    -p "${TEST_PORT}:8080" \
    -v "${TEMP_DATA}:/data" \
    -e "DROPPER_SECRET=${SMOKE_AUTH}" \
    -e "DROPPER_AUDIT_LOG_PATH=/dev/null" \
    -e "DROPPER_LOGGING_FORMAT=console" \
    "${DOCKER_IMAGE}:${DOCKER_TAG}" >/dev/null

# --- Wait for container to be ready ---
bold "Waiting for container (max ${MAX_WAIT_SECONDS}s)..."
elapsed=0
while [ ${elapsed} -lt ${MAX_WAIT_SECONDS} ]; do
    if curl -sf "${BASE_URL}/healthz" >/dev/null 2>&1; then
        green "Container ready after ${elapsed}s"
        break
    fi
    sleep ${WAIT_INTERVAL}
    elapsed=$((elapsed + WAIT_INTERVAL))
done

if [ ${elapsed} -ge ${MAX_WAIT_SECONDS} ]; then
    red "Container did not become ready within ${MAX_WAIT_SECONDS}s"
    echo "Container logs:"
    docker logs "${CONTAINER_NAME}" 2>&1 | tail -20
    exit 1
fi

echo ""
bold "Running tests..."
echo ""

# --- Test 1: GET /healthz returns status ok ---
response="$(curl -sf "${BASE_URL}/healthz")"
assert_contains "GET /healthz returns status ok" "${response}" '"status":"ok"'

# --- Test 2: GET /version returns project name ---
response="$(curl -sf "${BASE_URL}/version")"
assert_contains "GET /version returns project name" "${response}" '"name":"dropper"'

# --- Test 3: GET /login returns 200 ---
status="$(curl -so /dev/null -w '%{http_code}' "${BASE_URL}/login")"
assert_status "GET /login returns 200" "${status}" "200"

# --- Test 4: GET / without auth redirects (303) ---
status="$(curl -so /dev/null -w '%{http_code}' "${BASE_URL}/")"
assert_status "GET / without auth returns 303" "${status}" "303"

# --- Test 5: POST /login with correct credential sets session cookie ---
login_response="$(curl -s -D - -o /dev/null \
    -X POST \
    -d "secret=${SMOKE_AUTH}" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    "${BASE_URL}/login")"
assert_contains "POST /login sets session cookie" "${login_response}" "dropper_session="

# Extract session cookie for subsequent requests
session_cookie="$(echo "${login_response}" | grep -i 'set-cookie:' | grep -o 'dropper_session=[^;]*' | head -1)"

# --- Test 6: GET / with session cookie returns 200 ---
if [ -n "${session_cookie}" ]; then
    status="$(curl -so /dev/null -w '%{http_code}' -b "${session_cookie}" "${BASE_URL}/")"
    assert_status "GET / with session returns 200" "${status}" "200"
else
    red "GET / with session — no session cookie obtained"
    FAIL=$((FAIL + 1))
fi

# --- Test 7: GET /metrics contains dropper metrics ---
response="$(curl -sf "${BASE_URL}/metrics")"
assert_contains "GET /metrics contains dropper_http_requests_total" "${response}" "dropper_http_requests_total"

# --- Test 8: POST /login with wrong credential fails ---
status="$(curl -so /dev/null -w '%{http_code}' \
    -X POST \
    -d "secret=wrong-credential-value" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    "${BASE_URL}/login")"
assert_status "POST /login with wrong credential returns 401" "${status}" "401"

# --- Test 9: POST /logout destroys session ---
if [ -n "${session_cookie}" ]; then
    status="$(curl -so /dev/null -w '%{http_code}' \
        -X POST \
        -b "${session_cookie}" \
        "${BASE_URL}/logout")"
    assert_status "POST /logout returns 303" "${status}" "303"
fi

# --- Results ---
echo ""
bold "=== Results: ${PASS} passed, ${FAIL} failed ==="

if [ ${FAIL} -gt 0 ]; then
    echo ""
    echo "Container logs (last 20 lines):"
    docker logs "${CONTAINER_NAME}" 2>&1 | tail -20
    exit 1
fi

exit 0
