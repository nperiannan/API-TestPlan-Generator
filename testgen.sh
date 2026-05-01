#!/usr/bin/env bash
# testgen.sh — Generate API test plans and export to Excel/CSV (Linux)
# Reads source paths from config/config.yaml
#
# Usage:
#   ./testgen.sh                          # Generate wired features (default)
#   ./testgen.sh wired                    # Generate wired features only
#   ./testgen.sh wireless                 # Generate wireless features only
#   ./testgen.sh all                      # Generate all features (wired + wireless)
#   ./testgen.sh radius-server            # Generate a specific feature only
#   ./testgen.sh "radius-server,ntp-server"  # Multiple specific features

set -euo pipefail

FEATURES="${1:-wired}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG="$SCRIPT_DIR/config/config.yaml"
OUT_DIR="$SCRIPT_DIR/Testplans"
XLSX_DIR="$SCRIPT_DIR/TestplansXlsx"
CSV_DIR="$SCRIPT_DIR/TestplansCsv"
TESTGEN="$SCRIPT_DIR/bin/linux/testgen"
YAML2EXCEL="$SCRIPT_DIR/bin/linux/yaml2excel"
YAML2CSV="$SCRIPT_DIR/bin/linux/yaml2csv"

# ── Colours ────────────────────────────────────────────────────────
CYAN='\033[0;36m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
RED='\033[0;31m'; GRAY='\033[0;37m'; BOLD='\033[1m'; RESET='\033[0m'

echo ""
echo -e "${CYAN}=======================================================${RESET}"
echo -e "${CYAN} API Test Plan Generator${RESET}"
echo -e "${CYAN}=======================================================${RESET}"
echo ""

# ── Parse config/config.yaml for source paths ───────────────────────

if [[ ! -f "$CONFIG" ]]; then
    echo -e "${RED}config/config.yaml not found.${RESET}" >&2
    exit 1
fi

SOURCES_DIR="$SCRIPT_DIR/sources"
YANG_LOCAL_DIR=""; YANG_SPARSE=""
REST_LOCAL_DIR=""; REST_SPARSE=""; REST_FILE=""
NOSAPI_LOCAL_DIR=""; NOSAPI_FILE=""
CURRENT_SRC=""

while IFS= read -r line; do
    [[ "$line" =~ ^[[:space:]]*# ]] && continue
    [[ -z "${line// }" ]] && continue
    if [[ "$line" =~ ^[[:space:]]{0,1}sourcesDir:[[:space:]]*(.+) ]]; then
        SOURCES_DIR="${BASH_REMATCH[1]//\"/}"; SOURCES_DIR="${SOURCES_DIR//\'/}"
        continue
    fi
    if [[ "$line" =~ ^[[:space:]]{2}([a-zA-Z][a-zA-Z0-9_-]*):[[:space:]]*$ ]]; then
        CURRENT_SRC="${BASH_REMATCH[1]}"; continue
    fi
    if [[ "$line" =~ ^[[:space:]]{4}([a-zA-Z][a-zA-Z0-9_-]*):[[:space:]]*(.+) ]]; then
        key="${BASH_REMATCH[1]}"; val="${BASH_REMATCH[2]//\"/}"; val="${val//\'/}"
        case "$CURRENT_SRC/$key" in
            yang/localDir)   YANG_LOCAL_DIR="$val" ;;
            yang/sparse)     YANG_SPARSE="$val" ;;
            restSpec/localDir) REST_LOCAL_DIR="$val" ;;
            restSpec/sparse)   REST_SPARSE="$val" ;;
            restSpec/localFile) REST_FILE="$val" ;;
            nosapiSpec/localDir) NOSAPI_LOCAL_DIR="$val" ;;
            nosapiSpec/localFile) NOSAPI_FILE="$val" ;;
        esac
    fi
done < "$CONFIG"

YANG_DIR="$SOURCES_DIR/$YANG_LOCAL_DIR/$YANG_SPARSE"
REST_SPEC="$SOURCES_DIR/$REST_LOCAL_DIR/$REST_SPARSE/$REST_FILE"
NOSAPI_SPEC="$SOURCES_DIR/$NOSAPI_LOCAL_DIR/$NOSAPI_FILE"

echo -e "${GRAY}Source paths (from config/config.yaml):${RESET}"
echo -e "${GRAY}  YANG dir:    $YANG_DIR${RESET}"
echo -e "${GRAY}  REST spec:   $REST_SPEC${RESET}"
echo -e "${GRAY}  NOSAPI spec: $NOSAPI_SPEC${RESET}"
echo ""

# ── Check executable ────────────────────────────────────────────────

if [[ ! -x "$TESTGEN" ]]; then
    echo -e "${YELLOW}bin/linux/testgen not found. Building...${RESET}"
    mkdir -p "$SCRIPT_DIR/bin/linux"
    go build -o "$TESTGEN" "$SCRIPT_DIR/cmd/testgen/"
fi

[[ ! -d "$YANG_DIR"   ]] && echo -e "${YELLOW}Warning: YANG directory not found: $YANG_DIR${RESET}"
[[ ! -f "$REST_SPEC"  ]] && echo -e "${YELLOW}Warning: REST spec not found: $REST_SPEC${RESET}"
[[ ! -f "$NOSAPI_SPEC" ]] && echo -e "${YELLOW}Warning: NOSAPI spec not found: $NOSAPI_SPEC${RESET}"

# ── Resolve feature mode ────────────────────────────────────────────

FEATURE_CATEGORIES=""
FEATURE_FILTER=""

case "${FEATURES,,}" in
    wired)
        echo -e "${GREEN}Mode: Wired features (global-profile, wired-blueprint, service-profile)${RESET}"
        FEATURE_CATEGORIES="global-profile,wired-blueprint,service-profile"
        ;;
    wireless)
        echo -e "${GREEN}Mode: Wireless features (wireless-blueprint)${RESET}"
        FEATURE_CATEGORIES="wireless-blueprint"
        ;;
    all)
        echo -e "${GREEN}Mode: All features (wired + wireless)${RESET}"
        FEATURE_CATEGORIES="global-profile,wired-blueprint,wireless-blueprint,service-profile"
        ;;
    *)
        echo -e "${GREEN}Mode: Specific features: $FEATURES${RESET}"
        FEATURE_CATEGORIES="global-profile,wired-blueprint,wireless-blueprint,service-profile"
        FEATURE_FILTER="$FEATURES"
        ;;
esac

echo ""
echo -e "${GREEN}Running testgen...${RESET}"
echo ""

# ── Run generator ───────────────────────────────────────────────────

ARGS=(
    "--yang-dir"           "$YANG_DIR"
    "--rest-spec"          "$REST_SPEC"
    "--nosapi-spec"        "$NOSAPI_SPEC"
    "--out-dir"            "$OUT_DIR"
    "--feature-categories" "$FEATURE_CATEGORIES"
    "--include-categories" "functional,boundary,negative,scale,performance"
    "--scope-types"        "site-group,device"
    "--target-types"       "site-group,device"
    "--deployment-methods" "rolling,immediate"
    "--scale-factor"       "100"
    "--performance-iterations" "10"
    "--one-file-per-feature"   "true"
    "--features"           "$FEATURE_FILTER"
)

"$TESTGEN" "${ARGS[@]}"

# ── Post-generation summary ─────────────────────────────────────────

echo ""
echo -e "${CYAN}=======================================================${RESET}"
echo -e "${CYAN} Test Plans Generated${RESET}"
echo -e "${CYAN}=======================================================${RESET}"

declare -A CAT_TOTALS
CATEGORIES=(functional boundary negative performance scale)
for c in "${CATEGORIES[@]}"; do CAT_TOTALS[$c]=0; done
TOTAL_TESTS=0
FEATURE_COUNT=0

while IFS= read -r -d '' yaml_file; do
    [[ "$(basename "$yaml_file")" == "example-test.yaml" ]] && continue
    count=$(grep -c 'testCaseID:' "$yaml_file" 2>/dev/null || true)
    [[ "$count" -eq 0 ]] && continue
    (( FEATURE_COUNT++ )) || true
    (( TOTAL_TESTS += count )) || true
    for c in "${CATEGORIES[@]}"; do
        n=$(grep -cP "^\s+type:\s+${c}\s*$" "$yaml_file" 2>/dev/null || true)
        (( CAT_TOTALS[$c] += n )) || true
    done
done < <(find "$OUT_DIR" -name '*.yaml' -print0)

printf " %-12s %8s\n" "Category" "Tests"
printf " %s\n" "────────────────────"
for c in "${CATEGORIES[@]}"; do
    [[ "${CAT_TOTALS[$c]}" -gt 0 ]] && printf " %-12s %8d\n" "$c" "${CAT_TOTALS[$c]}"
done
printf " %s\n" "────────────────────"
printf "${GREEN}${BOLD} %-12s %8d${RESET}\n" "TOTAL" "$TOTAL_TESTS"
echo ""
echo -e "${GREEN} Features : $FEATURE_COUNT${RESET}"
echo -e "${GREEN} Output   : $OUT_DIR${RESET}"

# ── Excel export (batch) ────────────────────────────────────────────

echo ""
echo -e "${CYAN}=======================================================${RESET}"
echo -e "${CYAN} Exporting to Excel...${RESET}"
echo -e "${CYAN}=======================================================${RESET}"
echo ""

if [[ ! -x "$YAML2EXCEL" ]]; then
    echo -e "${YELLOW}bin/linux/yaml2excel not found. Building...${RESET}"
    mkdir -p "$SCRIPT_DIR/bin/linux"
    go build -o "$YAML2EXCEL" "$SCRIPT_DIR/cmd/yaml2excel/"
fi

"$YAML2EXCEL"

XLSX_COUNT=$(find "$XLSX_DIR" -name '*.xlsx' 2>/dev/null | wc -l)

# ── CSV export (batch) ──────────────────────────────────────────────

echo ""
echo -e "${CYAN}=======================================================${RESET}"
echo -e "${CYAN} Exporting to CSV...${RESET}"
echo -e "${CYAN}=======================================================${RESET}"
echo ""

if [[ ! -x "$YAML2CSV" ]]; then
    echo -e "${YELLOW}bin/linux/yaml2csv not found. Building...${RESET}"
    mkdir -p "$SCRIPT_DIR/bin/linux"
    go build -o "$YAML2CSV" "$SCRIPT_DIR/cmd/yaml2csv/"
fi

"$YAML2CSV"

CSV_COUNT=$(find "$CSV_DIR" -name '*.csv' 2>/dev/null | wc -l)

echo ""
echo -e "${CYAN}=======================================================${RESET}"
echo -e "${GREEN} All done!${RESET}"
echo -e "${CYAN}=======================================================${RESET}"
echo -e "${GREEN} Test plans  : $FEATURE_COUNT YAML files in $OUT_DIR${RESET}"
echo -e "${GREEN} Excel files : $XLSX_COUNT .xlsx files in $XLSX_DIR${RESET}"
echo -e "${GREEN} CSV files   : $CSV_COUNT .csv files in $CSV_DIR${RESET}"
echo -e "${CYAN}=======================================================${RESET}"
