package operator

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/url"
	"os"
	"time"
)

// Config holds the configuration for connecting to a Havoc teamserver.
type Config struct {
	// URL is the teamserver WebSocket URL (e.g., wss://host:port/havoc/)
	URL string
	// User is the operator username
	User string
	// Password is the operator plaintext password
	Password string
	// Insecure skips TLS certificate verification
	Insecure bool
	// TLSCA is an optional path to a PEM-encoded CA certificate
	TLSCA string
	// NoReconnect disables automatic reconnection
	NoReconnect bool
	// ReconnectInterval is the base interval for exponential backoff
	ReconnectInterval time.Duration
	// MaxReconnectInterval is the maximum interval for exponential backoff
	MaxReconnectInterval time.Duration
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		ReconnectInterval:    time.Second,
		MaxReconnectInterval: 30 * time.Second,
	}
}

// TLSConfig returns a tls.Config based on the Config settings.
func (c *Config) TLSConfig() (*tls.Config, error) {
	cfg := &tls.Config{
		InsecureSkipVerify: c.Insecure,
	}

	// Only set ServerName if not using insecure mode
	if !c.Insecure {
		cfg.ServerName = c.tlsServerName()
	}

	if c.TLSCA != "" {
		caCert, err := os.ReadFile(c.TLSCA)
		if err != nil {
			return nil, err
		}
		caPool := x509.NewCertPool()
		if !caPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		cfg.RootCAs = caPool
	}

	return cfg, nil
}

func (c *Config) tlsServerName() string {
	u, err := url.Parse(c.URL)
	if err != nil {
		return ""
	}
	host := u.Hostname()
	if host == "" {
		return ""
	}
	return host
}