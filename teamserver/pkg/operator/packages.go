package operator

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"golang.org/x/crypto/sha3"
)

// Package represents a Havoc package structure.
type Package struct {
	Head Head `json:"Head"`
	Body Body `json:"Body"`
}

type Head struct {
	Event    int    `json:"Event"`
	User     string `json:"User"`
	Time     string `json:"Time"`
	OneTime  string `json:"OneTime"`
	SubEvent int    `json:"SubEvent"` // This is actually in Body, but we keep it here for convenience
}

type Body struct {
	SubEvent int                    `json:"SubEvent"`
	Info     map[string]interface{} `json:"Info"`
}

// PackageType constants matching teamserver/pkg/packager/types.go
const (
	// InitConnection
	TypeInitConnection = 0x1
	InitSuccess        = 1
	InitError          = 2
	InitOAuthRequest   = 3
	InitProfile        = 5

	// Listener
	TypeListener = 0x2
	ListenerAdd  = 1
	ListenerEdit = 2
	ListenerRem  = 3
	ListenerMark = 4
	ListenerErr  = 5

	// Chat
	TypeChat = 0x4
	ChatMsg  = 1
	ChatLis  = 2
	ChatSes  = 3
	ChatUsr  = 4
	ChatDis  = 5

	// Session
	TypeSession = 0x7
	SessNew     = 1
	SessRem     = 2
	SessInput   = 3
	SessOutput  = 4
	SessDead    = 5

	// Gate
	TypeGate = 0x5
	GateStaged     = 1
	GateStageless  = 2

	// Teamserver
	TypeTeamserver = 0x10
	TsLog          = 1
	TsProfile      = 2
)

// HashPassword computes SHA3-256 hex of the password.
func HashPassword(password string) string {
	h := sha3.New256()
	h.Write([]byte(password))
	return hex.EncodeToString(h.Sum(nil))
}

// LoginPackage creates an OAuthRequest package.
func LoginPackage(user, passwordHash string) *Package {
	return &Package{
		Head: Head{
			Event: TypeInitConnection,
			User:  user,
			Time:  "", // Will be set by teamserver
		},
		Body: Body{
			SubEvent: InitOAuthRequest,
			Info: map[string]interface{}{
				"User":     user,
				"Password": passwordHash,
			},
		},
	}
}

// TaskPackage creates a Session.Input package for tasking.
func TaskPackage(user, demonID string, commandID int, info map[string]string, commandLine string) *Package {
	// Convert map[string]string to map[string]interface{}
	infoMap := make(map[string]interface{}, len(info))
	for k, v := range info {
		infoMap[k] = v
	}

	return &Package{
		Head: Head{
			Event: TypeSession,
			User:  user,
			Time:  "",
		},
		Body: Body{
			SubEvent: SessInput,
			Info:     infoMap,
		},
	}
}

// MarshalPackage marshals a package to JSON bytes.
func MarshalPackage(pkg *Package) ([]byte, error) {
	return json.Marshal(pkg)
}

// UnmarshalPackage unmarshals JSON bytes to a package.
func UnmarshalPackage(data []byte) (*Package, error) {
	var pkg Package
	err := json.Unmarshal(data, &pkg)
	return &pkg, err
}

// ParsePackageInfo extracts string values from Info map.
func ParsePackageInfo(info map[string]interface{}) map[string]string {
	result := make(map[string]string, len(info))
	for k, v := range info {
		if s, ok := v.(string); ok {
			result[k] = s
		} else if v != nil {
			result[k] = fmt.Sprintf("%v", v)
		}
	}
	return result
}