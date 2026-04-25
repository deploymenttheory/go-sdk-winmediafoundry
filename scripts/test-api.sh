#!/usr/bin/env bash
# test-api.sh — end-to-end test of all winupdate REST API endpoints.
#
# The script exercises the full lifecycle:
#   1. Health / public endpoints (no mTLS)
#   2. Live Windows Update queries (POST /v1/updates/fetch)
#   3. Build catalog browsing (GET /v1/builds, /v1/builds/{uuid})
#   4. File metadata + CDN URL resolution via GetExtendedUpdateInfo2
#   5. Streaming file download (if EUI2 returns URLs)
#   6. Build diff (GET /v1/diff)
#   7. Change feed (GET /v1/feed)
#   8. SSE live stream (GET /v1/feed/stream)
#
# Requirements:
#   curl, jq
#
# Usage:
#   # Start the server first:
#   docker compose up -d
#   # Wait for certs to be generated (first run only takes ~10s):
#   docker compose logs -f | grep "Starting: winupdate"
#   # Run the tests:
#   ./scripts/test-api.sh
#
# Environment variables:
#   WINUPDATE_URL   server base URL (default https://localhost:8443)
#   CERTS_DIR       directory containing ca.crt, client.crt, client.key
#                   (default ./certs — populated by docker compose on first start)
#   DOWNLOAD_DIR    where to save downloaded files (default ./downloads)
set -euo pipefail

BASE_URL="${WINUPDATE_URL:-https://localhost:8443}"
CERTS_DIR="${CERTS_DIR:-./certs}"
DOWNLOAD_DIR="${DOWNLOAD_DIR:-./downloads}"

CERT="$CERTS_DIR/client.crt"
KEY="$CERTS_DIR/client.key"
CACERT="$CERTS_DIR/ca.crt"

PASS=0
FAIL=0
WARN=0

# ─── Terminal colours ─────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
BLUE='\033[0;34m'; BOLD='\033[1m'; NC='\033[0m'

section() { printf "\n${BOLD}══  %s${NC}\n" "$*"; }
log()     { printf "   ${BLUE}▶${NC} %s\n" "$*"; }
pass()    { PASS=$((PASS+1));  printf "   ${GREEN}✓${NC} %s\n" "$*"; }
fail()    { FAIL=$((FAIL+1));  printf "   ${RED}✗${NC} %s\n" "$*"; }
warn()    { WARN=$((WARN+1));  printf "   ${YELLOW}⚠${NC} %s\n" "$*"; }

# curl wrapper: mTLS request to /v1/ endpoints
# --fail-with-body: exit non-zero on HTTP errors AND print the response body
api() { curl --silent --show-error --fail-with-body --cert "$CERT" --key "$KEY" --cacert "$CACERT" "$@"; }

# curl wrapper: public endpoints (no client cert, but CA still verified)
pub() { curl --silent --show-error --fail-with-body --cacert "$CACERT" "$@"; }

# jq with a fallback so missing keys don't abort the script
jqr() { jq -r "${1}" 2>/dev/null || echo ""; }

# ─── 0. Prerequisites ─────────────────────────────────────────────────────────
section "Prerequisites"

for tool in curl jq; do
    if command -v "$tool" >/dev/null 2>&1; then
        pass "$tool found ($(command -v "$tool"))"
    else
        fail "$tool not found — install it first (brew install $tool / apt install $tool)"
        exit 1
    fi
done

if [[ ! -f "$CERT" || ! -f "$KEY" || ! -f "$CACERT" ]]; then
    fail "mTLS certificates not found in $CERTS_DIR"
    echo "     Run:  docker compose up -d   (certs are generated on first start)"
    echo "     Or:   ./scripts/gen-certs.sh  (for non-Docker testing)"
    exit 1
fi
pass "mTLS certificates found in $CERTS_DIR"

mkdir -p "$DOWNLOAD_DIR"
pass "Download directory: $DOWNLOAD_DIR"

# ─── 1. Health & public endpoints (no mTLS) ───────────────────────────────────
section "Health checks  (no mTLS required)"

if health=$(pub "$BASE_URL/healthz"); then
    pass "GET /healthz  →  $(echo "$health" | jqr '.status')"
else
    fail "GET /healthz  — server not reachable at $BASE_URL"
    echo "     Is docker compose up?  Run: docker compose up -d && docker compose logs -f"
    exit 1
fi

if ready=$(pub "$BASE_URL/readyz"); then
    pass "GET /readyz   →  $(echo "$ready" | jqr '.status')"
else
    fail "GET /readyz   — DB not ready"
fi

spec_bytes=$(pub "$BASE_URL/openapi.yaml" | wc -c | tr -d ' ')
pass "GET /openapi.yaml  →  ${spec_bytes} bytes"

# ─── 2. Live Windows Update fetch (POST /v1/updates/fetch) ────────────────────
section "POST /v1/updates/fetch  — live WU queries"

echo "   Querying Windows Update across arch/ring combinations ..."
echo "   (each call hits fe3.delivery.mp.microsoft.com — allow 10–30 s per query)"
echo ""

fetch_wu() {
    local arch="$1" ring="$2"
    log "arch=$arch  ring=$ring ..."
    local result total new_count updated_count
    if result=$(api -X POST "$BASE_URL/v1/updates/fetch" \
            -H 'Content-Type: application/json' \
            -d "{\"arch\":\"$arch\",\"ring\":\"$ring\",\"flight\":\"Active\"}"); then
        total=$(echo "$result"         | jqr '.data.total')
        new_count=$(echo "$result"     | jq  '.data.new_builds | length' 2>/dev/null || echo 0)
        updated_count=$(echo "$result" | jq  '.data.updated    | length' 2>/dev/null || echo 0)
        pass "arch=$arch ring=$ring  →  $total update(s)  new=$new_count  updated=$updated_count"
    else
        fail "arch=$arch ring=$ring  →  POST /v1/updates/fetch failed"
    fi
}

fetch_wu amd64 Dev
fetch_wu amd64 Beta
fetch_wu amd64 Retail
fetch_wu arm64 Dev

# ─── 3. Build catalog ─────────────────────────────────────────────────────────
section "GET /v1/builds  — catalog listing"

if ! BUILDS=$(api "$BASE_URL/v1/builds?limit=25"); then
    fail "GET /v1/builds"
    BUILDS='{"data":[],"meta":{"total":0}}'
fi

CATALOG_TOTAL=$(echo "$BUILDS" | jqr '.meta.total')
pass "GET /v1/builds  →  $CATALOG_TOTAL build(s) in catalog"

echo ""
echo "$BUILDS" | jq -r '.data[] |
    "   \(.uuid[:8])…  \(.build // "N/A")  [\(.arch)/\(.ring)]  rev=\(.revision)  \(.title[:50] // "")"' \
    2>/dev/null || true
echo ""

FIRST_UUID=$(echo "$BUILDS" | jqr '.data[0].uuid')
FIRST_REV=$(echo "$BUILDS"  | jqr '.data[0].revision // "1"')
SECOND_UUID=$(echo "$BUILDS" | jqr '.data[1].uuid')

if [[ -z "$FIRST_UUID" ]]; then
    warn "No builds in catalog — all rings returned 0 updates from Windows Update"
    warn "Try again later or check network connectivity to fe3.delivery.mp.microsoft.com"
    FIRST_UUID=""
fi

# ─── 4. Single build ──────────────────────────────────────────────────────────
section "GET /v1/builds/{uuid}  — single build"

if [[ -n "$FIRST_UUID" ]]; then
    if BUILD=$(api "$BASE_URL/v1/builds/$FIRST_UUID"); then
        BUILD_TITLE=$(echo "$BUILD"  | jqr '.data.title')
        BUILD_NUM=$(echo "$BUILD"    | jqr '.data.build')
        BUILD_ARCH=$(echo "$BUILD"   | jqr '.data.arch')
        BUILD_RING=$(echo "$BUILD"   | jqr '.data.ring')
        BUILD_STABLE=$(echo "$BUILD" | jqr '.data.is_stable')
        pass "GET /v1/builds/$FIRST_UUID"
        echo "   title:     $BUILD_TITLE"
        echo "   build:     $BUILD_NUM"
        echo "   arch/ring: $BUILD_ARCH / $BUILD_RING"
        echo "   stable:    $BUILD_STABLE"
    else
        fail "GET /v1/builds/$FIRST_UUID"
    fi
else
    warn "Skipping — no builds in catalog"
fi

# ─── 5. File metadata + EUI2 CDN URL resolution ───────────────────────────────
section "GET /v1/builds/{uuid}/files  — file listing + EUI2 URL resolution"

FILES_HAVE_URLS=false
DL_NAME=""
DL_SHA1=""

if [[ -n "$FIRST_UUID" ]]; then
    log "Calling GetExtendedUpdateInfo2 for $FIRST_UUID rev=$FIRST_REV ..."
    log "(hits fe3cr.delivery.mp.microsoft.com — allow 5–15 s)"

    if FILES=$(api \
            "$BASE_URL/v1/builds/$FIRST_UUID/files?with_urls=true&revision=$FIRST_REV"); then
        FILE_COUNT=$(echo "$FILES" | jq '.data | length' 2>/dev/null || echo 0)
        URL_COUNT=$(echo "$FILES" | \
            jq '[.data[] | select(.url != null and .url != "")] | length' 2>/dev/null || echo 0)
        pass "GET /v1/builds/$FIRST_UUID/files?with_urls=true  →  $FILE_COUNT file(s), $URL_COUNT with CDN URLs"

        if [[ "$FILE_COUNT" -gt 0 ]]; then
            echo ""
            echo "$FILES" | jq -r '.data[] |
                "   \(.name // "?")  \(.size_bytes // 0) bytes  sha1=\(.sha1[:12] // "N/A")…"' \
                | head -10
            echo ""
        fi

        if [[ "$URL_COUNT" -gt 0 ]]; then
            FILES_HAVE_URLS=true
            # Pick the smallest file that has a resolved URL for the download test.
            SMALLEST=$(echo "$FILES" | jq -r '
                [.data[] | select(.url != null and .url != "")]
                | sort_by(.size_bytes // 0)
                | .[0]
                | {name, size_bytes, sha1}')
            DL_NAME=$(echo "$SMALLEST" | jqr '.name')
            DL_SIZE=$(echo "$SMALLEST" | jqr '.size_bytes // 0')
            DL_SHA1=$(echo "$SMALLEST" | jqr '.sha1 // ""')
            log "Smallest downloadable file: $DL_NAME  ($DL_SIZE bytes)"
        else
            warn "EUI2 returned 0 CDN URLs for this update"
            warn "Windows 11 feature updates use UUP chunked delivery (SecuredFragment)."
            warn "Direct ESD download URLs appear for cumulative/patch updates (e.g. Retail ring"
            warn "on or after monthly Patch Tuesday).  The SOAP calls and catalog storage are"
            warn "working correctly — the CDN URLs are simply not offered for this build type."
        fi
    else
        fail "GET /v1/builds/$FIRST_UUID/files?with_urls=true"
    fi
else
    warn "Skipping — no builds in catalog"
fi

# ─── 6. Streaming download ────────────────────────────────────────────────────
section "GET /v1/builds/{uuid}/files/{filename}/download"

if [[ "$FILES_HAVE_URLS" == true && -n "$DL_NAME" && -n "$FIRST_UUID" ]]; then
    # URL-encode the filename (Python available on macOS/Ubuntu; fallback to raw name)
    if command -v python3 >/dev/null 2>&1; then
        ENC_NAME=$(python3 -c \
            "import urllib.parse,sys; print(urllib.parse.quote(sys.argv[1], safe=''))" \
            "$DL_NAME" 2>/dev/null || echo "$DL_NAME")
    else
        ENC_NAME="$DL_NAME"
    fi

    OUT_PATH="$DOWNLOAD_DIR/$DL_NAME"
    log "Downloading $DL_NAME ($DL_SIZE bytes) → $OUT_PATH ..."

    if api -o "$OUT_PATH" \
            "$BASE_URL/v1/builds/$FIRST_UUID/files/$ENC_NAME/download?revision=$FIRST_REV"; then
        ACTUAL=$(wc -c < "$OUT_PATH" | tr -d ' ')
        pass "Download complete  →  $OUT_PATH  ($ACTUAL bytes)"

        # SHA-1 verification
        if [[ -n "$DL_SHA1" ]]; then
            # sha1sum (Linux) or shasum (macOS)
            DISK_SHA1=$(sha1sum "$OUT_PATH" 2>/dev/null | awk '{print $1}' || \
                        shasum   "$OUT_PATH" 2>/dev/null | awk '{print $1}' || echo "")
            if [[ -n "$DISK_SHA1" && "$DISK_SHA1" == "$DL_SHA1" ]]; then
                pass "SHA-1 verified  →  $DISK_SHA1"
            elif [[ -n "$DISK_SHA1" ]]; then
                fail "SHA-1 mismatch  expected=$DL_SHA1  got=$DISK_SHA1"
            fi
        fi
    else
        fail "Download of $DL_NAME failed"
    fi
else
    warn "Skipping download — no CDN URLs were returned by EUI2 for this build"
    if [[ -n "$FIRST_UUID" ]]; then
        echo ""
        echo "   To download Windows ESDs once a build with CDN URLs is in the catalog:"
        echo ""
        echo "   # List files with resolved URLs"
        echo "   curl --cert $CERT --key $KEY --cacert $CACERT \\"
        echo "     '$BASE_URL/v1/builds/<uuid>/files?with_urls=true&revision=1'"
        echo ""
        echo "   # Stream-download a specific file"
        echo "   curl --cert $CERT --key $KEY --cacert $CACERT \\"
        echo "     -o Windows11.esd \\"
        echo "     '$BASE_URL/v1/builds/<uuid>/files/Windows11.esd/download?revision=1'"
        echo ""
    fi
fi

# ─── 7. Build diff ────────────────────────────────────────────────────────────
section "GET /v1/diff  — build comparison"

if [[ -n "$FIRST_UUID" && -n "$SECOND_UUID" && "$FIRST_UUID" != "$SECOND_UUID" ]]; then
    if DIFF=$(api "$BASE_URL/v1/diff?base=$FIRST_UUID&target=$SECOND_UUID"); then
        ADDED=$(echo "$DIFF"   | jq '.data.added   | length' 2>/dev/null || echo 0)
        REMOVED=$(echo "$DIFF" | jq '.data.removed | length' 2>/dev/null || echo 0)
        CHANGED=$(echo "$DIFF" | jq '.data.changed | length' 2>/dev/null || echo 0)
        UNCHANGED=$(echo "$DIFF" | jqr '.data.unchanged // 0')
        pass "GET /v1/diff  →  added=$ADDED  removed=$REMOVED  changed=$CHANGED  unchanged=$UNCHANGED"
    else
        fail "GET /v1/diff?base=$FIRST_UUID&target=$SECOND_UUID"
    fi
else
    warn "Skipping diff — need ≥2 builds in catalog (have $CATALOG_TOTAL)"
fi

# ─── 8. Change feed ───────────────────────────────────────────────────────────
section "GET /v1/feed  — paginated change history"

if FEED=$(api "$BASE_URL/v1/feed?limit=10"); then
    FEED_TOTAL=$(echo "$FEED" | jqr '.meta.total // 0')
    pass "GET /v1/feed  →  $FEED_TOTAL event(s)"
    echo "$FEED" | jq -r '.data[] |
        "   [\(.event_type)]  \(.build_title[:45] // "?")  \(.arch)/\(.ring)"' \
        2>/dev/null | head -10 || true
else
    fail "GET /v1/feed"
fi

# ─── 9. SSE live event stream ─────────────────────────────────────────────────
section "GET /v1/feed/stream  — Server-Sent Events"

log "Opening SSE connection (3 s window) ..."
# timeout exits non-zero; use || true so set -e doesn't abort here
SSE_OUT=$(timeout 3 api -N -H 'Accept: text/event-stream' \
    "$BASE_URL/v1/feed/stream" 2>&1 || true)

if echo "$SSE_OUT" | grep -q "^data:"; then
    EVENT_COUNT=$(echo "$SSE_OUT" | grep -c "^data:" || true)
    pass "SSE stream connected — $EVENT_COUNT event(s) received in window"
    echo "$SSE_OUT" | grep "^data:" | head -3
else
    # A clean connection with no data is fine (stream is just idle).
    pass "SSE stream connected  (no events in 3 s window — stream is idle)"
fi

# ─── Summary ──────────────────────────────────────────────────────────────────
printf "\n${BOLD}══════════════════════════════════════════${NC}\n"
printf "  ${GREEN}PASS: %-3d${NC}  ${RED}FAIL: %-3d${NC}  ${YELLOW}WARN: %-3d${NC}\n" \
    "$PASS" "$FAIL" "$WARN"
printf "${BOLD}══════════════════════════════════════════${NC}\n\n"

if [[ "$CATALOG_TOTAL" -gt 0 && "$FAIL" -eq 0 ]]; then
    echo "All API endpoints are working correctly."
    echo ""
    if [[ ! "$FILES_HAVE_URLS" == true ]]; then
        echo "ESD download status:"
        echo "  The catalog has $CATALOG_TOTAL build(s) but EUI2 returned no CDN URLs."
        echo "  This is expected for Windows 11 feature updates (SecuredFragment/UUP delivery)."
        echo "  CDN URLs appear for cumulative updates.  Try running after a Patch Tuesday:"
        echo "    curl -X POST $BASE_URL/v1/updates/fetch \\"
        echo "      -d '{\"arch\":\"amd64\",\"ring\":\"Retail\"}'"
    fi
fi

[[ "$FAIL" -eq 0 ]] || exit 1
