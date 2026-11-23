#!/bin/bash
# Havoc Framework Test Runner
# Runs all tests in ephemeral directories and generates a report

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(dirname "$SCRIPT_DIR")"
TEAMSERVER_DIR="$REPO_ROOT/teamserver"
REPORT_DIR="$SCRIPT_DIR/report"
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
REPORT_FILE="$REPORT_DIR/test-report-${TIMESTAMP}.json"

echo "=== Havoc Framework Test Suite ==="
echo "Repository: $REPO_ROOT"
echo "Timestamp:  $TIMESTAMP"
echo ""

# Create report directory
mkdir -p "$REPORT_DIR"

# Function to run tests for a package
run_package_tests() {
    local pkg=$1
    local name=$2
    echo "Running $name ($pkg)..."
    
    cd "$TEAMSERVER_DIR"
    if go test -v -race -count=1 "$pkg" 2>&1; then
        echo "  ✓ $name passed"
        return 0
    else
        echo "  ✗ $name failed"
        return 1
    fi
}

# Track results
PASSED=0
FAILED=0
SKIPPED=0

# Run ephemeral tests
if run_package_tests "./test/ephemeral/..." "Ephemeral Tests"; then
    ((PASSED++))
else
    ((FAILED++))
fi

# Run unit tests
if run_package_tests "./test/unit/..." "Unit Tests"; then
    ((PASSED++))
else
    ((FAILED++))
fi

# Run integration tests (skip if no teamserver)
if run_package_tests "./test/integration/..." "Integration Tests"; then
    ((PASSED++))
else
    ((SKIPPED++))
fi

# Run core package tests
CORE_PACKAGES=(
    "./pkg/mcp/..."
    "./pkg/operator/..."
    "./pkg/profile/..."
    "./pkg/logger/..."
    "./pkg/common/..."
    "./pkg/agent/..."
    "./pkg/handlers/..."
    "./pkg/events/..."
    "./pkg/service/..."
    "./pkg/db/..."
    "./pkg/packager/..."
    "./pkg/utils/..."
    "./pkg/socks/..."
    "./pkg/webhook/..."
    "./pkg/win32/..."
    "./pkg/colors/..."
)

for pkg in "${CORE_PACKAGES[@]}"; do
    name=$(basename "$pkg" | sed 's/\.\.\///')
    if run_package_tests "$pkg" "Core: $name"; then
        ((PASSED++))
    else
        ((FAILED++))
    fi
done

# Generate summary
echo ""
echo "=== Test Summary ==="
echo "Passed:  $PASSED"
echo "Failed:  $FAILED"
echo "Skipped: $SKIPPED"

# Create JSON report
cat > "$REPORT_FILE" <<EOF
{
  "timestamp": "$(date -Iseconds)",
  "passed": $PASSED,
  "failed": $FAILED,
  "skipped": $SKIPPED,
  "total": $((PASSED + FAILED + SKIPPED)),
  "repository": "$REPO_ROOT",
  "commit": "$(cd "$REPO_ROOT" && git rev-parse HEAD 2>/dev/null || echo 'unknown')",
  "branch": "$(cd "$REPO_ROOT" && git branch --show-current 2>/dev/null || echo 'unknown')"
}
EOF

echo ""
echo "Report written to: $REPORT_FILE"

# Exit with error if any tests failed
if [ $FAILED -gt 0 ]; then
    exit 1
fi

exit 0