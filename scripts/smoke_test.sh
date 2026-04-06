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
TEMP_DATA=""
cleanup() {
    bold "Cleaning up..."
    ${RUNTIME} stop "${CONTAINER_NAME}" 2>/dev/null || true
    ${RUNTIME} rm -f "${CONTAINER_NAME}" 2>/dev/null || true
    if [ -n "${TEMP_DATA}" ] && [ -d "${TEMP_DATA}" ]; then
        rm -rf "${TEMP_DATA}"
    fi
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

assert_header_present() {
    local label="$1" headers="$2" header_name="$3"
    if echo "${headers}" | grep -qi "^${header_name}:"; then
        green "${label}"
        PASS=$((PASS + 1))
    else
        red "${label} — header '${header_name}' not found in response"
        FAIL=$((FAIL + 1))
    fi
}

# --- Container management ---
start_container() {
    local extra_env="${1:-}"

    TEMP_DATA="$(mktemp -d)"

    local run_args=(
        -d
        --name "${CONTAINER_NAME}"
        -p "${TEST_PORT}:8080"
        -v "${TEMP_DATA}:/data"
        -e "DROPPER_SECRET=${SMOKE_AUTH}"
        -e "DROPPER_AUDIT_LOG_PATH=/dev/null"
        -e "DROPPER_LOGGING_FORMAT=console"
    )

    # Add any extra env vars passed as arguments.
    if [ -n "${extra_env}" ]; then
        while IFS= read -r env_line; do
            [ -n "${env_line}" ] && run_args+=(-e "${env_line}")
        done <<< "${extra_env}"
    fi

    run_args+=("${DOCKER_IMAGE}:${DOCKER_TAG}")

    ${RUNTIME} run "${run_args[@]}" >/dev/null

    # Wait for container to be ready.
    local elapsed=0
    while [ ${elapsed} -lt ${MAX_WAIT_SECONDS} ]; do
        if curl -sf "${BASE_URL}/healthz" >/dev/null 2>&1; then
            return 0
        fi
        sleep ${WAIT_INTERVAL}
        elapsed=$((elapsed + WAIT_INTERVAL))
    done

    red "Container did not become ready within ${MAX_WAIT_SECONDS}s"
    echo "Container logs:"
    ${RUNTIME} logs "${CONTAINER_NAME}" 2>&1 | tail -20
    return 1
}

stop_container() {
    ${RUNTIME} stop "${CONTAINER_NAME}" 2>/dev/null || true
    ${RUNTIME} rm -f "${CONTAINER_NAME}" 2>/dev/null || true
    if [ -n "${TEMP_DATA}" ] && [ -d "${TEMP_DATA}" ]; then
        rm -rf "${TEMP_DATA}"
    fi
    TEMP_DATA=""
}

restart_container() {
    local extra_env="${1:-}"
    stop_container
    start_container "${extra_env}"
}

# --- Detect container runtime ---
if [ -n "${CONTAINER_RUNTIME:-}" ]; then
    RUNTIME="${CONTAINER_RUNTIME}"
elif command -v podman &>/dev/null; then
    RUNTIME="podman"
elif command -v docker &>/dev/null; then
    RUNTIME="docker"
else
    echo "Error: neither podman nor docker found. Cannot run smoke test."
    echo "Set CONTAINER_RUNTIME env var to override."
    exit 1
fi

# --- Build image ---
bold "=== dropper smoke test ==="
echo ""
bold "Using runtime: ${RUNTIME}"
bold "Building image ${DOCKER_IMAGE}:${DOCKER_TAG}..."

# Pass version build args (same as Makefile)
SMOKE_GIT_COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")"
SMOKE_GIT_TAG="v$(grep '  version:' versions.yaml | sed 's/.*"\(.*\)"/\1/')"
SMOKE_BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

${RUNTIME} build -q \
    --build-arg "GIT_COMMIT=${SMOKE_GIT_COMMIT}" \
    --build-arg "GIT_TAG=${SMOKE_GIT_TAG}" \
    --build-arg "BUILD_TIME=${SMOKE_BUILD_TIME}" \
    -t "${DOCKER_IMAGE}:${DOCKER_TAG}" . >/dev/null

bold "Starting container ${CONTAINER_NAME} on port ${TEST_PORT}..."
start_container

green "Container ready"

echo ""
bold "Running tests..."
echo ""

# ============================================================
# Part 1: Health, version, auth flow (tests 1-9)
# ============================================================

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
    -H "Origin: ${BASE_URL}" \
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
    -H "Origin: ${BASE_URL}" \
    "${BASE_URL}/login")"
assert_status "POST /login with wrong credential returns 401" "${status}" "401"

# --- Test 9: POST /logout destroys session ---
if [ -n "${session_cookie}" ]; then
    status="$(curl -so /dev/null -w '%{http_code}' \
        -X POST \
        -b "${session_cookie}" \
        -H "Origin: ${BASE_URL}" \
        "${BASE_URL}/logout")"
    assert_status "POST /logout returns 303" "${status}" "303"
fi

# Re-login for file operation tests (previous session was destroyed).
login_response="$(curl -s -D - -o /dev/null \
    -X POST \
    -d "secret=${SMOKE_AUTH}" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -H "Origin: ${BASE_URL}" \
    "${BASE_URL}/login")"
session_cookie="$(echo "${login_response}" | grep -i 'set-cookie:' | grep -o 'dropper_session=[^;]*' | head -1)"

# ============================================================
# Part 2: File operations (tests 10-13)
# ============================================================

echo ""
bold "File operation tests..."
echo ""

# --- Test 10: File upload via multipart POST ---
UPLOAD_FILE="$(mktemp)"
echo "smoke test content" > "${UPLOAD_FILE}"
upload_response="$(curl -s -w '\n%{http_code}' \
    -b "${session_cookie}" \
    -H "Origin: ${BASE_URL}" \
    -F "file=@${UPLOAD_FILE};filename=smoke_test.txt" \
    "${BASE_URL}/files/upload?path=.")"
upload_status="$(echo "${upload_response}" | tail -1)"
upload_body="$(echo "${upload_response}" | sed '$d')"
rm -f "${UPLOAD_FILE}"
assert_status "POST /files/upload returns 200" "${upload_status}" "200"
assert_contains "POST /files/upload reports 1 uploaded" "${upload_body}" '"uploaded":1'

# --- Test 11: File listing via JSON ---
list_response="$(curl -s -w '\n%{http_code}' \
    -b "${session_cookie}" \
    -H "Accept: application/json" \
    "${BASE_URL}/files?path=.")"
list_status="$(echo "${list_response}" | tail -1)"
list_body="$(echo "${list_response}" | sed '$d')"
assert_status "GET /files?path=. (JSON) returns 200" "${list_status}" "200"
assert_contains "GET /files lists uploaded file" "${list_body}" 'smoke_test.txt'

# --- Test 12: File download ---
download_response="$(curl -s -D - -o /dev/null -w '%{http_code}' \
    -b "${session_cookie}" \
    "${BASE_URL}/files/download?path=smoke_test.txt")"
download_status="$(echo "${download_response}" | tail -1)"
assert_status "GET /files/download returns 200" "${download_status}" "200"
assert_contains "GET /files/download sets Content-Disposition" "${download_response}" "attachment"

# --- Test 13: Directory creation ---
mkdir_response="$(curl -s -w '\n%{http_code}' \
    -X POST \
    -b "${session_cookie}" \
    -H "Origin: ${BASE_URL}" \
    "${BASE_URL}/files/mkdir?path=.&name=testfolder")"
mkdir_status="$(echo "${mkdir_response}" | tail -1)"
mkdir_body="$(echo "${mkdir_response}" | sed '$d')"
assert_status "POST /files/mkdir returns 201" "${mkdir_status}" "201"
assert_contains "POST /files/mkdir returns folder name" "${mkdir_body}" '"name":"testfolder"'

# ============================================================
# Part 3: Security (tests 14-15)
# ============================================================

echo ""
bold "Security tests..."
echo ""

# --- Test 14: Security headers present ---
headers="$(curl -sI "${BASE_URL}/healthz")"
assert_header_present "X-Content-Type-Options header present" "${headers}" "X-Content-Type-Options"
assert_header_present "X-Frame-Options header present" "${headers}" "X-Frame-Options"
assert_header_present "Content-Security-Policy header present" "${headers}" "Content-Security-Policy"
assert_header_present "Referrer-Policy header present" "${headers}" "Referrer-Policy"
assert_header_present "Permissions-Policy header present" "${headers}" "Permissions-Policy"
assert_header_present "X-Permitted-Cross-Domain-Policies header present" "${headers}" "X-Permitted-Cross-Domain-Policies"

# --- Test 15: CSRF rejection with mismatched Origin ---
csrf_response="$(curl -s -w '\n%{http_code}' \
    -X POST \
    -b "${session_cookie}" \
    -H "Origin: https://evil.example.com" \
    -F "file=@/dev/null;filename=evil.txt" \
    "${BASE_URL}/files/upload?path=.")"
csrf_status="$(echo "${csrf_response}" | tail -1)"
csrf_body="$(echo "${csrf_response}" | sed '$d')"
assert_status "POST with mismatched Origin returns 403 (CSRF)" "${csrf_status}" "403"
assert_contains "CSRF rejection returns csrf_rejected code" "${csrf_body}" '"code":"csrf_rejected"'

# ============================================================
# Part 4: Readonly mode (test 16)
# ============================================================

echo ""
bold "Readonly mode tests..."
echo ""

# Restart container with readonly mode enabled.
restart_container "DROPPER_READONLY=true"

# Login again after restart.
login_response="$(curl -s -D - -o /dev/null \
    -X POST \
    -d "secret=${SMOKE_AUTH}" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -H "Origin: ${BASE_URL}" \
    "${BASE_URL}/login")"
session_cookie="$(echo "${login_response}" | grep -i 'set-cookie:' | grep -o 'dropper_session=[^;]*' | head -1)"

# Upload should be rejected in readonly mode.
UPLOAD_FILE="$(mktemp)"
echo "readonly test" > "${UPLOAD_FILE}"
readonly_upload_status="$(curl -so /dev/null -w '%{http_code}' \
    -b "${session_cookie}" \
    -H "Origin: ${BASE_URL}" \
    -F "file=@${UPLOAD_FILE};filename=readonly_test.txt" \
    "${BASE_URL}/files/upload?path=.")"
rm -f "${UPLOAD_FILE}"
assert_status "Readonly: POST /files/upload returns 403" "${readonly_upload_status}" "403"

# Mkdir should be rejected in readonly mode.
readonly_mkdir_status="$(curl -so /dev/null -w '%{http_code}' \
    -X POST \
    -b "${session_cookie}" \
    -H "Origin: ${BASE_URL}" \
    "${BASE_URL}/files/mkdir?path=.&name=shouldfail")"
assert_status "Readonly: POST /files/mkdir returns 403" "${readonly_mkdir_status}" "403"

# Browse should still work in readonly mode.
readonly_browse_status="$(curl -so /dev/null -w '%{http_code}' \
    -b "${session_cookie}" \
    -H "Accept: application/json" \
    "${BASE_URL}/files?path=.")"
assert_status "Readonly: GET /files returns 200" "${readonly_browse_status}" "200"

# ============================================================
# Part 5: Extension filtering (test 17)
# ============================================================

echo ""
bold "Extension filtering tests..."
echo ""

# Restart with extension whitelist.
restart_container "DROPPER_ALLOWED_EXTENSIONS=.txt,.md"

# Login again after restart.
login_response="$(curl -s -D - -o /dev/null \
    -X POST \
    -d "secret=${SMOKE_AUTH}" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -H "Origin: ${BASE_URL}" \
    "${BASE_URL}/login")"
session_cookie="$(echo "${login_response}" | grep -i 'set-cookie:' | grep -o 'dropper_session=[^;]*' | head -1)"

# Upload .txt should succeed.
TXT_FILE="$(mktemp)"
echo "allowed extension" > "${TXT_FILE}"
ext_txt_response="$(curl -s -w '\n%{http_code}' \
    -b "${session_cookie}" \
    -H "Origin: ${BASE_URL}" \
    -F "file=@${TXT_FILE};filename=allowed.txt" \
    "${BASE_URL}/files/upload?path=.")"
ext_txt_status="$(echo "${ext_txt_response}" | tail -1)"
ext_txt_body="$(echo "${ext_txt_response}" | sed '$d')"
rm -f "${TXT_FILE}"
assert_status "Extension filter: .txt upload returns 200" "${ext_txt_status}" "200"
assert_contains "Extension filter: .txt upload succeeds" "${ext_txt_body}" '"uploaded":1'

# Upload .exe should be rejected.
EXE_FILE="$(mktemp)"
echo "blocked extension" > "${EXE_FILE}"
ext_exe_response="$(curl -s -w '\n%{http_code}' \
    -b "${session_cookie}" \
    -H "Origin: ${BASE_URL}" \
    -F "file=@${EXE_FILE};filename=blocked.exe" \
    "${BASE_URL}/files/upload?path=.")"
ext_exe_status="$(echo "${ext_exe_response}" | tail -1)"
ext_exe_body="$(echo "${ext_exe_response}" | sed '$d')"
rm -f "${EXE_FILE}"
assert_status "Extension filter: .exe upload returns 200" "${ext_exe_status}" "200"
assert_contains "Extension filter: .exe upload reports 0 uploaded" "${ext_exe_body}" '"uploaded":0'
assert_contains "Extension filter: .exe upload reports 1 failed" "${ext_exe_body}" '"failed":1'

# --- Results ---
echo ""
bold "=== Results: ${PASS} passed, ${FAIL} failed ==="

if [ ${FAIL} -gt 0 ]; then
    echo ""
    echo "Container logs (last 20 lines):"
    ${RUNTIME} logs "${CONTAINER_NAME}" 2>&1 | tail -20
    exit 1
fi

exit 0
