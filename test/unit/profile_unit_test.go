package unit

import (
	"os"
	"path/filepath"
	"testing"

	"Havoc/pkg/profile"
)

func getProfilePath(t *testing.T, name string) string {
	t.Helper()
	// Get the repo root by going up from the test directory
	repoRoot := filepath.Join("..", "..", "..")
	profPath := filepath.Join(repoRoot, "profiles", name)
	
	// Verify file exists
	if _, err := os.Stat(profPath); os.IsNotExist(err) {
		t.Fatalf("profile not found at %s", profPath)
	}
	
	return profPath
}

// TestProfileParsing tests parsing the default profile
func TestProfileParsing(t *testing.T) {
	profPath := getProfilePath(t, "havoc.yaotl")
	prof := profile.NewProfile()
	err := prof.SetProfile(profPath, false)
	if err != nil {
		t.Fatalf("failed to parse profile: %v", err)
	}

	if prof.ServerHost() == "" {
		t.Error("expected teamserver host")
	}

	if prof.ServerPort() == 0 {
		t.Error("expected teamserver port")
	}

	usernames := prof.ListOfUsernames()
	if len(usernames) == 0 {
		t.Error("expected at least one operator")
	}

	t.Logf("Parsed profile: host=%s, port=%d, operators=%d",
		prof.ServerHost(), prof.ServerPort(), len(usernames))
}

// TestProfileOperators tests operator configuration
func TestProfileOperators(t *testing.T) {
	profPath := getProfilePath(t, "havoc.yaotl")
	prof := profile.NewProfile()
	err := prof.SetProfile(profPath, false)
	if err != nil {
		t.Fatalf("failed to parse profile: %v", err)
	}

	usernames := prof.ListOfUsernames()
	for _, name := range usernames {
		t.Logf("Operator: %s", name)
	}
}

// TestMultipleProfiles tests parsing different profile files
func TestMultipleProfiles(t *testing.T) {
	profiles := []string{
		"havoc.yaotl",
		"dns_example.yaotl",
		"http_smb_dns.yaotl",
		"webhook_example.yaotl",
	}

	for _, profName := range profiles {
		t.Run(profName, func(t *testing.T) {
			profPath := getProfilePath(t, profName)
			prof := profile.NewProfile()
			err := prof.SetProfile(profPath, false)
			if err != nil {
				t.Fatalf("failed to parse %s: %v", profPath, err)
			}

			if prof.ServerHost() == "" {
				t.Errorf("%s: missing teamserver host", profPath)
			}
			t.Logf("Profile %s: host=%s, port=%d, operators=%d", profPath, prof.ServerHost(), prof.ServerPort(), len(prof.ListOfUsernames()))
		})
	}
}