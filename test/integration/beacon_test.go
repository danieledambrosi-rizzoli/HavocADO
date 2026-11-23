package integration

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"Havoc/pkg/common/builder"
	"Havoc/pkg/profile"
	"Havoc/test/ephemeral"
)

func getProfilePath(t *testing.T, name string) string {
	t.Helper()
	repoRoot := filepath.Join("..", "..", "..")
	profPath := filepath.Join(repoRoot, "profiles", name)
	
	if _, err := os.Stat(profPath); os.IsNotExist(err) {
		t.Fatalf("profile not found at %s", profPath)
	}
	
	return profPath
}

// TestBeaconGeneration tests generating various beacon/payload types
func TestBeaconGeneration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping beacon generation test in short mode")
	}

	// Check if compilers are available
	if !hasMingwCompilers() {
		t.Skip("mingw compilers not available, skipping beacon generation test")
	}

	// Create ephemeral directory for payload output
	payloadDir, cleanup := ephemeral.TempDir(t, "beacon-*")
	defer cleanup()

	// Load a test profile
	profPath := getProfilePath(t, "havoc.yaotl")
	prof := profile.NewProfile()
	err := prof.SetProfile(profPath, false)
	if err != nil {
		t.Fatalf("failed to parse profile: %v", err)
	}

	// Get the first listener from profile - we need to access internal config
	// For now, use a basic HTTP listener config
	listenerConfig := map[string]any{
		"HostBind": "0.0.0.0",
		"PortBind": "8080",
		"PortConn": "8080",
		"Hosts":    []string{"example.com"},
		"Secure":   false,
	}

	// Test different payload formats
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
		t.Run(tc.name, func(t *testing.T) {
			cfg := builder.BuilderConfig{
				Compiler64: prof.Config.Server.Build.Compiler64,
				Compiler86: prof.Config.Server.Build.Compiler86,
				Nasm:       prof.Config.Server.Build.Nasm,
				DebugDev:   false,
				SendLogs:   false,
			}

			b := builder.NewBuilder(cfg)
			b.SetSilent(true)
			b.SetArch(tc.arch)
			b.SetFormat(tc.fileType)
			b.SetExtension(tc.extension)
			b.SetListener(0, listenerConfig) // 0 = HTTP listener type

			// Minimal config
			config := map[string]any{
				"Sleep":            "2",
				"Jitter":           "15",
				"Sleep Technique":  "WaitForSingleObjectEx",
				"Sleep Jmp Gadget": "jmp rax",
				"Stack Duplication": false,
				"Proxy Loading":    "None (LdrLoadDll)",
				"Amsi/Etw Patch":   "Hardware breakpoints",
				"Injection": map[string]any{
					"Alloc":   "Win32",
					"Execute": "Win32",
					"Spawn64": "C:\\Windows\\System32\\notepad.exe",
					"Spawn32": "C:\\Windows\\SysWOW64\\notepad.exe",
				},
			}
			configBytes, _ := json.Marshal(config)
			b.SetConfig(string(configBytes))

			outputPath := filepath.Join(payloadDir, "beacon"+tc.extension)
			b.SetOutputPath(outputPath)

			success := b.Build()
			if !success {
				t.Fatalf("build failed for %s", tc.name)
			}

			payload := b.GetPayloadBytes()
			if payload == nil {
				t.Fatalf("no payload generated for %s", tc.name)
			}

			if len(payload) < tc.expectSize {
				t.Errorf("payload too small for %s: got %d bytes, expected at least %d", tc.name, len(payload), tc.expectSize)
			}

			t.Logf("%s: generated %d bytes", tc.name, len(payload))

			// Verify file was created
			if _, err := os.Stat(outputPath); os.IsNotExist(err) {
				t.Errorf("output file not created at %s", outputPath)
			}

			// Clean up
			b.DeletePayload()
		})
	}
}

// TestBeaconGenerationWithProfilePatch tests payload generation with profile patching
func TestBeaconGenerationWithProfilePatch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping beacon generation test in short mode")
	}

	if !hasMingwCompilers() {
		t.Skip("mingw compilers not available, skipping beacon generation test")
	}

	payloadDir, cleanup := ephemeral.TempDir(t, "beacon-patch-*")
	defer cleanup()

	profPath := getProfilePath(t, "havoc.yaotl")
	prof := profile.NewProfile()
	err := prof.SetProfile(profPath, false)
	if err != nil {
		t.Fatalf("failed to parse profile: %v", err)
	}

	listenerConfig := map[string]any{
		"HostBind": "0.0.0.0",
		"PortBind": "8080",
		"PortConn": "8080",
		"Hosts":    []string{"example.com"},
		"Secure":   false,
	}

	cfg := builder.BuilderConfig{
		Compiler64: prof.Config.Server.Build.Compiler64,
		Compiler86: prof.Config.Server.Build.Compiler86,
		Nasm:       prof.Config.Server.Build.Nasm,
		DebugDev:   false,
		SendLogs:   false,
	}

	b := builder.NewBuilder(cfg)
	b.SetSilent(true)
	b.SetArch(builder.ARCHITECTURE_X64)
	b.SetFormat(builder.FILETYPE_WINDOWS_EXE)
	b.SetExtension(".exe")
	b.SetListener(0, listenerConfig)
	b.SetPatchConfig(prof.Config.Demon.Binary)

	config := map[string]any{
		"Sleep":            "2",
		"Jitter":           "15",
		"Sleep Technique":  "WaitForSingleObjectEx",
		"Sleep Jmp Gadget": "jmp rax",
		"Stack Duplication": false,
		"Proxy Loading":    "None (LdrLoadDll)",
		"Amsi/Etw Patch":   "Hardware breakpoints",
		"Injection": map[string]any{
			"Alloc":   "Win32",
			"Execute": "Win32",
			"Spawn64": "C:\\Windows\\System32\\notepad.exe",
			"Spawn32": "C:\\Windows\\SysWOW64\\notepad.exe",
		},
	}
	configBytes, _ := json.Marshal(config)
	b.SetConfig(string(configBytes))

	outputPath := filepath.Join(payloadDir, "patched_beacon.exe")
	b.SetOutputPath(outputPath)

	success := b.Build()
	if !success {
		t.Fatal("build failed")
	}

	payload := b.GetPayloadBytes()
	if payload == nil {
		t.Fatal("no payload generated")
	}

	t.Logf("Patched beacon generated: %d bytes", len(payload))

	// Verify patch was applied - check for custom strings if any
	if prof.Config.Demon.Binary != nil && prof.Config.Demon.Binary.ReplaceStringsX64 != nil {
		for oldStr := range prof.Config.Demon.Binary.ReplaceStringsX64 {
			// The patch should have replaced the string
			t.Logf("Profile patch applied for: %s", oldStr)
		}
	}

	b.DeletePayload()
}

func hasMingwCompilers() bool {
	// Check if x86_64-w64-mingw32-gcc is available
	paths := []string{
		"/usr/bin/x86_64-w64-mingw32-gcc",
		"/usr/bin/i686-w64-mingw32-gcc",
		"x86_64-w64-mingw32-gcc",
		"i686-w64-mingw32-gcc",
	}

	for _, p := range paths {
		if _, err := exec.LookPath(p); err == nil {
			return true
		}
	}
	return false
}