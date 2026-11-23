package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// TestResult represents the result of a test package
type TestResult struct {
	Package    string        `json:"package"`
	Passed     int           `json:"passed"`
	Failed     int           `json:"failed"`
	Skipped    int           `json:"skipped"`
	Duration   time.Duration `json:"duration"`
	Output     string        `json:"output"`
	Error      string        `json:"error,omitempty"`
}

// TestReport represents the full test report
type TestReport struct {
	Timestamp   time.Time     `json:"timestamp"`
	TotalPassed int           `json:"total_passed"`
	TotalFailed int           `json:"total_failed"`
	TotalSkipped int          `json:"total_skipped"`
	TotalDuration time.Duration `json:"total_duration"`
	Results     []TestResult  `json:"results"`
	Summary     string        `json:"summary"`
}

func main() {
	fmt.Println("=== Havoc Framework Test Runner ===")
	fmt.Println()

	// Test packages to run
	testPackages := []string{
		"./test/ephemeral/...",
		"./test/unit/...",
		"./test/integration/...",
		// Core packages
		"./pkg/mcp/...",
		"./pkg/operator/...",
		"./pkg/profile/...",
		"./pkg/logger/...",
		"./pkg/common/...",
		"./pkg/agent/...",
		"./pkg/handlers/...",
		"./pkg/events/...",
		"./pkg/service/...",
		"./pkg/db/...",
		"./pkg/packager/...",
		"./pkg/utils/...",
	}

	report := TestReport{
		Timestamp: time.Now(),
		Results:   make([]TestResult, 0),
	}

	for _, pkg := range testPackages {
		fmt.Printf("Running tests for %s...\n", pkg)
		result := runTestPackage(pkg)
		report.Results = append(report.Results, result)
		report.TotalPassed += result.Passed
		report.TotalFailed += result.Failed
		report.TotalSkipped += result.Skipped
		report.TotalDuration += result.Duration

		status := "PASS"
		if result.Failed > 0 {
			status = "FAIL"
		} else if result.Passed == 0 && result.Skipped > 0 {
			status = "SKIP"
		}
		fmt.Printf("  %s: passed=%d, failed=%d, skipped=%d (%.2fs)\n",
			status, result.Passed, result.Failed, result.Skipped, result.Duration.Seconds())
	}

	// Generate summary
	report.Summary = fmt.Sprintf(
		"Total: %d passed, %d failed, %d skipped in %.2fs",
		report.TotalPassed, report.TotalFailed, report.TotalSkipped, report.TotalDuration.Seconds())

	// Write JSON report
	reportPath := "test-report.json"
	if err := writeReport(report, reportPath); err != nil {
		fmt.Printf("Failed to write report: %v\n", err)
	} else {
		fmt.Printf("\nReport written to %s\n", reportPath)
	}

	// Print summary
	fmt.Println("\n=== Test Summary ===")
	fmt.Println(report.Summary)

	// Exit with error code if any tests failed
	if report.TotalFailed > 0 {
		os.Exit(1)
	}
}

func runTestPackage(pkg string) TestResult {
	start := time.Now()

	// Run go test with JSON output
	cmd := exec.Command("go", "test", "-v", "-json", pkg)
	cmd.Dir = "/home/kali/Havoc/teamserver"

	output, err := cmd.CombinedOutput()
	duration := time.Since(start)

	result := TestResult{
		Package:  pkg,
		Duration: duration,
		Output:   string(output),
	}

	if err != nil {
		result.Error = err.Error()
	}

	// Parse JSON output
	passed, failed, skipped := parseTestOutput(string(output))
	result.Passed = passed
	result.Failed = failed
	result.Skipped = skipped

	return result
}

func parseTestOutput(output string) (passed, failed, skipped int) {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var event struct {
			Action  string `json:"Action"`
			Package string `json:"Package"`
			Test    string `json:"Test"`
			Output  string `json:"Output"`
		}

		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}

		switch event.Action {
		case "pass":
			passed++
		case "fail":
			failed++
		case "skip":
			skipped++
		}
	}

	return passed, failed, skipped
}

func writeReport(report TestReport, path string) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}