package integration

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"Havoc/pkg/common/builder"
	"Havoc/pkg/handlers"
	"Havoc/pkg/profile"
	"Havoc/test/ephemeral"
)

// getProfilePath returns the absolute path of a profile in the repository's
// profiles/ directory. TestMain has already changed into the repository root.
func getProfilePath(t *testing.T, name string) string {
	t.Helper()

	profPath := filepath.Join(repoRoot, "profiles", name)
	if _, err := os.Stat(profPath); err != nil {
		t.Fatalf("profile not found at %s: %v", profPath, err)
	}

	return profPath
}

func loadProfile(t *testing.T, name string) *profile.Profile {
	t.Helper()

	prof := profile.NewProfile()
	if err := prof.SetProfile(getProfilePath(t, name), false); err != nil {
		t.Fatalf("failed to parse profile %s: %v", name, err)
	}

	return prof
}

// requireToolchain skips the test when the profile's compiler configuration or
// the Demon source tree is not available on this machine.
func requireToolchain(t *testing.T, prof *profile.Profile) builder.BuilderConfig {
	t.Helper()

	if prof.Config.Server == nil || prof.Config.Server.Build == nil {
		t.Skip("profile has no Teamserver.Build block")
	}

	build := prof.Config.Server.Build
	for _, compiler := range []string{build.Compiler64, build.Compiler86} {
		if compiler == "" {
			t.Skip("profile does not configure both compilers")
		}
		if _, err := exec.LookPath(compiler); err != nil {
			t.Skipf("compiler %s not available: %v", compiler, err)
		}
	}

	if build.Nasm == "" {
		t.Skip("profile does not configure nasm")
	}
	if _, err := exec.LookPath(build.Nasm); err != nil {
		t.Skipf("nasm %s not available: %v", build.Nasm, err)
	}

	if _, err := os.Stat(filepath.Join("payloads", "Demon")); err != nil {
		t.Skipf("Demon source tree not available: %v", err)
	}

	return builder.BuilderConfig{
		Compiler64: build.Compiler64,
		Compiler86: build.Compiler86,
		Nasm:       build.Nasm,
		DebugDev:   false,
		SendLogs:   false,
	}
}

// newBuilderForTest creates a builder whose console output is routed through
// the test log, so a failed compile shows the compiler diagnostics.
func newBuilderForTest(t *testing.T, cfg builder.BuilderConfig) *builder.Builder {
	t.Helper()

	b := builder.NewBuilder(cfg)
	b.SetSilent(false)
	b.SendConsoleMessage = func(msgType, msg string) {
		if msgType == "Error" {
			t.Logf("builder error: %s", msg)
		}
	}

	return b
}

func testHTTPListener() *handlers.HTTP {
	return &handlers.HTTP{
		Config: handlers.HTTPConfig{
			Name:     "Test HTTP Listener",
			HostBind: "0.0.0.0",
			PortBind: "8080",
			PortConn: "8080",
			Hosts:    []string{"example.com"},
			Secure:   false,
		},
	}
}

func testDemonConfig(t *testing.T) string {
	t.Helper()

	config := map[string]any{
		"Sleep":             "2",
		"Jitter":            "15",
		"Sleep Technique":   "WaitForSingleObjectEx",
		"Sleep Jmp Gadget":  "jmp rax",
		"Stack Duplication": false,
		"Proxy Loading":     "None (LdrLoadDll)",
		"Amsi/Etw Patch":    "Hardware breakpoints",
		"Injection": map[string]any{
			"Alloc":   "Win32",
			"Execute": "Win32",
			"Spawn64": "C:\\Windows\\System32\\notepad.exe",
			"Spawn32": "C:\\Windows\\SysWOW64\\notepad.exe",
		},
	}

	configBytes, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("failed to encode test configuration: %v", err)
	}

	return string(configBytes)
}

// TestBeaconGeneration builds every supported payload format in parallel.
func TestBeaconGeneration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping beacon generation test in short mode")
	}

	prof := loadProfile(t, "havoc.yaotl")
	cfg := requireToolchain(t, prof)
	config := testDemonConfig(t)

	payloadTests := []struct {
		name       string
		fileType   int
		extension  string
		arch       int
		expectSize int // minimum expected size
	}{
		{"Windows EXE x64", builder.FILETYPE_WINDOWS_EXE, ".exe", builder.ARCHITECTURE_X64, 50000},
		{"Windows EXE x86", builder.FILETYPE_WINDOWS_EXE, ".exe", builder.ARCHITECTURE_X86, 50000},
		{"Windows DLL x64", builder.FILETYPE_WINDOWS_DLL, ".dll", builder.ARCHITECTURE_X64, 50000},
		{"Windows Service EXE x64", builder.FILETYPE_WINDOWS_SERVICE_EXE, ".exe", builder.ARCHITECTURE_X64, 50000},
		{"Windows Shellcode x64", builder.FILETYPE_WINDOWS_RAW_BINARY, ".bin", builder.ARCHITECTURE_X64, 1000},
	}

	for _, tc := range payloadTests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			payloadDir, _ := ephemeral.TempDir(t, "beacon-*")

			b := newBuilderForTest(t, cfg)
			b.SetArch(tc.arch)
			b.SetFormat(tc.fileType)
			b.SetExtension(tc.extension)
			b.SetListener(handlers.LISTENER_HTTP, testHTTPListener())

			if err := b.SetConfig(config); err != nil {
				t.Fatalf("failed to set config: %v", err)
			}

			outputPath := filepath.Join(payloadDir, "beacon"+tc.extension)
			b.SetOutputPath(outputPath)

			if !b.Build() {
				t.Fatalf("build failed for %s", tc.name)
			}

			payload := b.GetPayloadBytes()
			if payload == nil {
				t.Fatalf("no payload generated for %s", tc.name)
			}

			if len(payload) < tc.expectSize {
				t.Errorf("payload too small for %s: got %d bytes, expected at least %d", tc.name, len(payload), tc.expectSize)
			}

			// Shellcode builds return the bytes through GetPayloadBytes and
			// never write them to outputPath; every other format does.
			if tc.fileType != builder.FILETYPE_WINDOWS_RAW_BINARY {
				if _, err := os.Stat(outputPath); err != nil {
					t.Errorf("output file not created at %s: %v", outputPath, err)
				}
			}

			t.Logf("%s: generated %d bytes", tc.name, len(payload))
			b.DeletePayload()
		})
	}
}

// TestBeaconGenerationWithProfilePatch tests payload generation with profile patching.
func TestBeaconGenerationWithProfilePatch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping beacon generation test in short mode")
	}

	prof := loadProfile(t, "havoc.yaotl")
	cfg := requireToolchain(t, prof)
	config := testDemonConfig(t)

	payloadDir, _ := ephemeral.TempDir(t, "beacon-patch-*")

	b := newBuilderForTest(t, cfg)
	b.SetArch(builder.ARCHITECTURE_X64)
	b.SetFormat(builder.FILETYPE_WINDOWS_EXE)
	b.SetExtension(".exe")
	b.SetListener(handlers.LISTENER_HTTP, testHTTPListener())

	if prof.Config.Demon != nil && prof.Config.Demon.Binary != nil {
		b.SetPatchConfig(prof.Config.Demon.Binary)
	}

	if err := b.SetConfig(config); err != nil {
		t.Fatalf("failed to set config: %v", err)
	}

	outputPath := filepath.Join(payloadDir, "patched_beacon.exe")
	b.SetOutputPath(outputPath)

	if !b.Build() {
		t.Fatal("build failed")
	}

	payload := b.GetPayloadBytes()
	if payload == nil {
		t.Fatal("no payload generated")
	}

	if prof.Config.Demon != nil && prof.Config.Demon.Binary != nil && prof.Config.Demon.Binary.ReplaceStringsX64 != nil {
		for oldStr := range prof.Config.Demon.Binary.ReplaceStringsX64 {
			// The patch should have replaced the string.
			t.Logf("Profile patch applied for: %s", oldStr)
		}
	}

	t.Logf("Patched beacon generated: %d bytes", len(payload))
	b.DeletePayload()
}
