#!/bin/bash
# Havoc Framework Parallel Test Runner
#
# Contract:
#   Usage: test/run_parallel_tests.sh
#   Runs every Havoc test package in one parallel pool, then writes a JSON and
#   a Markdown report under test/report/.
#   Exit status: 0 if every package passed, 1 otherwise.
#
# Both Go modules are covered: the teamserver module (pkg/...) and the test
# module (unit, ephemeral, integration). Jobs from both modules share a single
# pool so fast unit tests do not wait for the long payload builds.
#
# Sizing: this machine has 12 CPUs / 32 GB RAM. MAX_PARALLEL=8 leaves headroom
# for the compiler processes started by the integration tests, which are
# themselves capped at 5 concurrent payload builds (-parallel 5). Core
# packages use -race; integration does not, because race instrumentation makes
# the external mingw toolchain significantly slower.

set +e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(dirname "$SCRIPT_DIR")"
TEST_DIR="$SCRIPT_DIR"
TEAMSERVER_DIR="$REPO_ROOT/teamserver"
REPORT_DIR="$SCRIPT_DIR/report"
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
JSON_REPORT="$REPORT_DIR/test-report-${TIMESTAMP}.json"
MD_REPORT="$REPORT_DIR/test-report-${TIMESTAMP}.md"

# Maximum number of test packages running at the same time.
MAX_PARALLEL="${MAX_PARALLEL:-8}"

# Job specification: workdir|package|label|timeout|extra go test flags
JOBS=(
    "$TEAMSERVER_DIR|./pkg/mcp/...|Core: mcp|5m|-race"
    "$TEAMSERVER_DIR|./pkg/operator/...|Core: operator|5m|-race"
    "$TEAMSERVER_DIR|./pkg/profile/...|Core: profile|10m|-race"
    "$TEAMSERVER_DIR|./pkg/logger/...|Core: logger|5m|-race"
    "$TEAMSERVER_DIR|./pkg/common/...|Core: common|5m|-race"
    "$TEAMSERVER_DIR|./pkg/agent/...|Core: agent|5m|-race"
    "$TEAMSERVER_DIR|./pkg/handlers/...|Core: handlers|5m|-race"
    "$TEAMSERVER_DIR|./pkg/events/...|Core: events|5m|-race"
    "$TEAMSERVER_DIR|./pkg/service/...|Core: service|5m|-race"
    "$TEAMSERVER_DIR|./pkg/db/...|Core: db|5m|-race"
    "$TEAMSERVER_DIR|./pkg/packager/...|Core: packager|5m|-race"
    "$TEAMSERVER_DIR|./pkg/utils/...|Core: utils|5m|-race"
    "$TEAMSERVER_DIR|./pkg/socks/...|Core: socks|5m|-race"
    "$TEAMSERVER_DIR|./pkg/webhook/...|Core: webhook|5m|-race"
    "$TEAMSERVER_DIR|./pkg/win32/...|Core: win32|5m|-race"
    "$TEAMSERVER_DIR|./pkg/colors/...|Core: colors|5m|-race"
    "$TEST_DIR|./unit/...|Unit Tests|10m|-race"
    "$TEST_DIR|./ephemeral/...|Ephemeral Tests|5m|-race"
    "$TEST_DIR|./integration/...|Integration Tests|20m|-parallel 5"
)

TOTAL=${#JOBS[@]}

mkdir -p "$REPORT_DIR"

TMPDIR=$(mktemp -d)
RESULT_DIR="$TMPDIR/results"
mkdir -p "$RESULT_DIR"
trap 'rm -rf "$TMPDIR"' EXIT

# run_job runs one package. $6 is the output file; the exit code and duration
# are written to "$6.meta" so the dispatcher can report the result.
run_job() {
    local workdir="$1"
    local pkg="$2"
    local timeout="$3"
    local extra="$4"
    local out_file="$5"

    local start end code
    start=$(date +%s%3N)
    # shellcheck disable=SC2086 # extra flags are intentionally word-split
    ( cd "$workdir" && nice -n 10 go test -v -count=1 -timeout "$timeout" $extra "$pkg" ) >"$out_file" 2>&1
    code=$?
    end=$(date +%s%3N)

    printf '%s|%s\n' "$code" "$((end - start))" >"$out_file.meta"
}

# record_result stores the outcome for the report generator. One file per job
# index keeps the result order independent of completion order.
record_result() {
    local idx="$1"
    local pkg="$2"
    local label="$3"
    local out_file="$4"

    local code=-1 duration=0 status
    if [ -f "$out_file.meta" ]; then
        IFS='|' read -r code duration <"$out_file.meta"
    fi

    if [ "$code" -eq 0 ]; then
        status="PASSED"
        printf '  PASS %s (%sms)\n' "$label" "$duration"
    else
        status="FAILED"
        printf '  FAIL %s (exit %s, %sms)\n' "$label" "$code" "$duration"
    fi

    # Fields are tab separated because the report generator splits on tabs.
    printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
        "$idx" "$pkg" "$label" "$status" "$code" "$duration" "$out_file" \
        >"$RESULT_DIR/$idx"
}

echo "=== Havoc Framework Parallel Test Suite ==="
echo "Repository: $REPO_ROOT"
echo "Timestamp:  $TIMESTAMP"
echo "Packages:   $TOTAL"
echo "Parallel:   $MAX_PARALLEL"
echo ""

declare -A JOB_INDEX=()
running=0
next=0

while [ "$next" -lt "$TOTAL" ] || [ "$running" -gt 0 ]; do
    while [ "$next" -lt "$TOTAL" ] && [ "$running" -lt "$MAX_PARALLEL" ]; do
        IFS='|' read -r workdir pkg label timeout extra <<<"${JOBS[$next]}"

        out_file="$TMPDIR/job_${next}.out"
        echo "  START $label ($pkg)"
        run_job "$workdir" "$pkg" "$timeout" "$extra" "$out_file" &
        JOB_INDEX[$!]=$next
        next=$((next + 1))
        running=$((running + 1))
    done

    if [ "$running" -gt 0 ]; then
        finished_pid=""
        wait -n -p finished_pid
        idx="${JOB_INDEX[$finished_pid]}"
        unset "JOB_INDEX[$finished_pid]"
        running=$((running - 1))

        IFS='|' read -r workdir pkg label timeout extra <<<"${JOBS[$idx]}"
        record_result "$idx" "$pkg" "$label" "$TMPDIR/job_${idx}.out"
    fi
done

echo ""

# Generate the JSON and Markdown reports from the per-job result files.
COMMIT=$(cd "$REPO_ROOT" && git rev-parse HEAD 2>/dev/null || echo "unknown")
BRANCH=$(cd "$REPO_ROOT" && git branch --show-current 2>/dev/null || echo "unknown")

python3 - "$RESULT_DIR" "$JSON_REPORT" "$MD_REPORT" "$REPO_ROOT" "$COMMIT" "$BRANCH" <<'PY'
import json
import os
import sys
from datetime import datetime, timezone

result_dir, json_path, md_path, repo, commit, branch = sys.argv[1:7]

rows = []
for name in sorted(os.listdir(result_dir), key=int):
    with open(os.path.join(result_dir, name), encoding="utf-8", errors="replace") as handle:
        fields = handle.read().rstrip("\n").split("\t", 6)
    if len(fields) != 7:
        continue
    idx, pkg, label, status, code, duration, out_file = fields
    try:
        with open(out_file, encoding="utf-8", errors="replace") as handle:
            output = handle.read()
    except OSError:
        output = ""
    rows.append({
        "name": label,
        "pkg": pkg,
        "passed": status == "PASSED",
        "status": status,
        "code": int(code),
        "duration_ms": int(duration),
        "output": output,
    })

passed = sum(1 for row in rows if row["passed"])
failed = len(rows) - passed
now = datetime.now(timezone.utc).astimezone().isoformat()

report = {
    "timestamp": now,
    "passed": passed,
    "failed": failed,
    "total": len(rows),
    "repository": repo,
    "commit": commit,
    "branch": branch,
    "results": rows,
}
with open(json_path, "w", encoding="utf-8") as handle:
    json.dump(report, handle, indent=2)
    handle.write("\n")

lines = [
    "# Havoc Framework Test Report",
    "",
    f"**Timestamp:** {now}",
    f"**Repository:** {repo}",
    f"**Commit:** {commit}",
    f"**Branch:** {branch}",
    "",
    "## Summary",
    "",
    "| Status | Count |",
    "|--------|-------|",
    f"| Passed | {passed} |",
    f"| Failed | {failed} |",
    f"| Total  | {len(rows)} |",
    "",
    "## Test Results",
    "",
    "| Package | Name | Status | Duration | Exit Code |",
    "|---------|------|--------|----------|-----------|",
]
for row in rows:
    mark = "PASSED" if row["passed"] else "FAILED"
    lines.append(
        f"| `{row['pkg']}` | {row['name']} | {mark} | {row['duration_ms']}ms | {row['code']} |"
    )

lines += ["", "## Failed Test Details", ""]
if failed == 0:
    lines.append("All tests passed.")
else:
    for row in rows:
        if row["passed"]:
            continue
        lines += [
            f"### {row['name']} (`{row['pkg']}`)",
            "",
            f"**Exit Code:** {row['code']}",
            "",
            f"**Duration:** {row['duration_ms']}ms",
            "",
            "**Output:**",
            "",
            "```",
            row["output"].rstrip(),
            "```",
            "",
            "---",
            "",
        ]

with open(md_path, "w", encoding="utf-8") as handle:
    handle.write("\n".join(lines) + "\n")

print(f"Passed: {passed}")
print(f"Failed: {failed}")
print(f"Total:  {len(rows)}")
sys.exit(0 if failed == 0 else 1)
PY
report_status=$?

echo ""
echo "JSON Report:     $JSON_REPORT"
echo "Markdown Report: $MD_REPORT"

exit "$report_status"
