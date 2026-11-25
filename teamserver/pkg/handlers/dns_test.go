package handlers

import (
	"crypto/rc4"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"net"
	"testing"

	"github.com/miekg/dns"
)

func testDNSConfig() DNSConfig {
	return DNSConfig{
		HostBind:    DNSDefaultBindHost,
		PortBind:    DNSDefaultBindPort,
		Domain:      "example.com",
		PktSize:     DNSDefaultPktSize,
		TTL:         DNSDefaultTTL,
		EncryptKey:  DNSDefaultKey,
		BurstSleep:  50,
		BurstJitter: 0,
	}
}

// TestParseRequestQName covers the QNAME layout of the protocol:
//
//	{SID}.{OP}.{SEQ}.{IDX}.{DATA_LABELS}.{DOMAIN}
func TestParseRequestQName(t *testing.T) {
	transport := &TransportDNS{Config: testDNSConfig()}

	question := dns.Question{
		Name:   "deadbeef.www.0001.idx.MZXW6YTB.example.com.",
		Qtype:  dns.TypeTXT,
		Qclass: dns.ClassINET,
	}

	req := transport.parseRequest(question)

	if req.sid != "deadbeef" {
		t.Fatalf("sid = %q, want %q", req.sid, "deadbeef")
	}
	if req.op != "HI" {
		t.Fatalf("op = %q, want %q", req.op, "HI")
	}

	// The payload is decoded with the standard base32 alphabet without
	// padding, exactly like the reference implementation does.
	expected, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString("MZXW6YTB")
	if err != nil {
		t.Fatalf("base32 decode: %v", err)
	}
	if string(req.data) != string(expected) {
		t.Fatalf("data = %q, want %q", string(req.data), string(expected))
	}
	if req.seq != int(uint32(0x0001)^seqXorMask) {
		t.Fatalf("seq = %x, want %x", req.seq, uint32(0x0001)^seqXorMask)
	}
}

func TestParseRequestOperations(t *testing.T) {
	transport := &TransportDNS{Config: testDNSConfig()}

	tests := []struct {
		name string
		op   string
		want string
	}{
		{"www", "www", "HI"},
		{"hi", "hi", "HI"},
		{"cdn", "cdn", "PUT"},
		{"put", "put", "PUT"},
		{"api", "api", "GET"},
		{"get", "get", "GET"},
		{"hb", "hb", "HB"},
	}

	for _, test := range tests {
		question := dns.Question{
			Name:   "deadbeef." + test.op + ".0001.idx.MZXW6YTB.example.com.",
			Qtype:  dns.TypeTXT,
			Qclass: dns.ClassINET,
		}

		req := transport.parseRequest(question)
		if req.op != test.want {
			t.Fatalf("parseRequest(%q).op = %q, want %q", test.name, req.op, test.want)
		}
	}
}

func TestParseRequestRejectsInvalidSession(t *testing.T) {
	transport := &TransportDNS{Config: testDNSConfig()}

	question := dns.Question{
		Name:   "nothex.www.0001.idx.MZXW6YTB.example.com.",
		Qtype:  dns.TypeTXT,
		Qclass: dns.ClassINET,
	}

	req := transport.parseRequest(question)
	if req.op != "" {
		t.Fatalf("op = %q, want an empty operation", req.op)
	}
}

func TestBase32RoundTrip(t *testing.T) {
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("foo"))
	if encoded != "MZXW6" {
		t.Fatalf("base32(\"foo\") = %q, want %q", encoded, "MZXW6")
	}

	decoded, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(encoded)
	if err != nil {
		t.Fatalf("base32 decode: %v", err)
	}
	if string(decoded) != "foo" {
		t.Fatalf("decoded = %q, want %q", string(decoded), "foo")
	}
}

// TestPutAckByteLayout verifies the acknowledgement of an upload:
//
//	ip[0]     flags (0x01 complete, 0x02 needs reset)
//	ip[1:4]   next expected offset, 24 bit big endian
func TestPutAckByteLayout(t *testing.T) {
	transport := &TransportDNS{Config: testDNSConfig()}

	key, err := rc4Key(DNSDefaultKey)
	if err != nil {
		t.Fatalf("rc4Key: %v", err)
	}

	cipher, err := rc4.NewCipher(key)
	if err != nil {
		t.Fatalf("rc4.NewCipher: %v", err)
	}

	// A fragmented upload: the first fragment carries 16 bytes of a message
	// that is 64 bytes long.
	frame := make([]byte, 8+16)
	binary.BigEndian.PutUint32(frame[0:4], 64)
	binary.BigEndian.PutUint32(frame[4:8], 0)
	copy(frame[8:], "0123456789abcdef")
	cipher.XORKeyStream(frame, frame)

	req := &dnsRequest{
		sid:   "deadbeef",
		op:    "PUT",
		seq:   1,
		data:  frame,
		qtype: dns.TypeA,
		qname: "deadbeef.cdn.0001.0.MZXW6YTB.example.com.",
	}

	ack := transport.handlePUT(req)
	if ack.total != 64 {
		t.Fatalf("ack.total = %d, want 64", ack.total)
	}
	if ack.nextExpectedOff != 16 {
		t.Fatalf("ack.nextExpectedOff = %d, want 16", ack.nextExpectedOff)
	}
	if ack.complete {
		t.Fatal("ack.complete is true, want false")
	}

	rr := transport.buildPutAckResponse(req, ack, 10)
	record, ok := rr.(*dns.A)
	if !ok {
		t.Fatalf("buildPutAckResponse returned %T, want *dns.A", rr)
	}

	ip := record.A.To4()
	if ip == nil {
		t.Fatal("the answer does not carry an IPv4 address")
	}
	if ip[0] != 0x00 {
		t.Fatalf("flags = %#x, want 0x00", ip[0])
	}
	if ip[1] != 0x00 || ip[2] != 0x00 || ip[3] != 16 {
		t.Fatalf("nextExpectedOff = %#x, want 0x000010", ip[1:4])
	}
}

// TestPutAckCompleteFlag verifies the complete flag of an upload that fits
// into a single fragment.
func TestPutAckCompleteFlag(t *testing.T) {
	transport := &TransportDNS{Config: testDNSConfig()}

	key, err := rc4Key(DNSDefaultKey)
	if err != nil {
		t.Fatalf("rc4Key: %v", err)
	}

	cipher, err := rc4.NewCipher(key)
	if err != nil {
		t.Fatalf("rc4.NewCipher: %v", err)
	}

	payload := []byte("hello world")
	frame := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(frame[0:4], uint32(len(payload)))
	binary.BigEndian.PutUint32(frame[4:8], 0)
	copy(frame[8:], payload)
	cipher.XORKeyStream(frame, frame)

	req := &dnsRequest{
		sid:   "deadbeef",
		op:    "PUT",
		data:  frame,
		qtype: dns.TypeA,
		qname: "deadbeef.cdn.0001.0.MZXW6YTB.example.com.",
	}

	ack := transport.handlePUT(req)
	if !ack.complete {
		t.Fatal("ack.complete is false, want true")
	}

	rr := transport.buildPutAckResponse(req, ack, 10)
	record := rr.(*dns.A)
	ip := record.A.To4()

	if ip[0]&0x01 != 0x01 {
		t.Fatalf("flags = %#x, want the complete flag", ip[0])
	}
	if ip[1] != 0x00 || ip[2] != 0x00 || ip[3] != byte(len(payload)) {
		t.Fatalf("nextExpectedOff = %#x, want %d", ip[1:4], len(payload))
	}
}

// TestBuildResponseChunk verifies the frame of a download:
//
//	[0:4] total length, [4:8] offset, [8:] chunk
func TestBuildResponseChunk(t *testing.T) {
	transport := &TransportDNS{Config: testDNSConfig()}

	payload := []byte("0123456789")
	down := newDownBuf(payload, 0)

	frame := transport.buildResponseChunk(down, 0, false)
	if len(frame) != 8+len(payload) {
		t.Fatalf("the frame is %d bytes, want %d", len(frame), 8+len(payload))
	}
	if binary.BigEndian.Uint32(frame[0:4]) != uint32(len(payload)) {
		t.Fatalf("total = %d, want %d", binary.BigEndian.Uint32(frame[0:4]), len(payload))
	}
	if binary.BigEndian.Uint32(frame[4:8]) != 0 {
		t.Fatalf("offset = %d, want 0", binary.BigEndian.Uint32(frame[4:8]))
	}
	if string(frame[8:]) != string(payload) {
		t.Fatalf("chunk = %q, want %q", string(frame[8:]), string(payload))
	}

	frame = transport.buildResponseChunk(down, 4, false)
	if binary.BigEndian.Uint32(frame[4:8]) != 4 {
		t.Fatalf("offset = %d, want 4", binary.BigEndian.Uint32(frame[4:8]))
	}
	if string(frame[8:]) != string(payload[4:]) {
		t.Fatalf("chunk = %q, want %q", string(frame[8:]), string(payload[4:]))
	}
}

// TestBuildResponseChunkIsSafeForUDP verifies that the answer of a download
// never exceeds the size that is safe for a UDP datagram.
func TestBuildResponseChunkIsSafeForUDP(t *testing.T) {
	transport := &TransportDNS{Config: testDNSConfig()}

	payload := make([]byte, 4096)
	down := newDownBuf(payload, 0)

	frame := transport.buildResponseChunk(down, 0, false)
	if len(frame)-8 > dnsSafeChunkSize {
		t.Fatalf("the chunk carries %d bytes, want at most %d", len(frame)-8, dnsSafeChunkSize)
	}
}

// TestRc4RoundTrip verifies that the payload of a query is encrypted with the
// key of the listener.
func TestRc4RoundTrip(t *testing.T) {
	transport := &TransportDNS{Config: testDNSConfig()}

	payload := []byte("the quick brown fox")
	encrypted := transport.crypt(payload)

	if string(encrypted) == string(payload) {
		t.Fatal("the payload was not encrypted")
	}

	decrypted := transport.crypt(encrypted)
	if string(decrypted) != string(payload) {
		t.Fatalf("decrypted = %q, want %q", string(decrypted), string(payload))
	}
}

func TestNormalizeDNSDomain(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"c2.example.com", "c2.example.com", false},
		{"C2.Example.COM.", "c2.example.com", false},
		{"  c2.example.com  ", "c2.example.com", false},
		{"", "", true},
		{"not a domain", "", true},
		{"...", "", true},
	}

	for _, test := range tests {
		got, err := normalizeDNSDomain(test.in)
		if (err != nil) != test.wantErr {
			t.Fatalf("normalizeDNSDomain(%q) error = %v, wantErr %v", test.in, err, test.wantErr)
		}
		if got != test.want {
			t.Fatalf("normalizeDNSDomain(%q) = %q, want %q", test.in, got, test.want)
		}
	}
}

func TestDNSCryptKey(t *testing.T) {
	key, err := DNSCryptKey("000102030405060708090a0b0c0d0e0f")
	if err != nil {
		t.Fatalf("DNSCryptKey: %v", err)
	}
	if len(key) != 16 {
		t.Fatalf("the key is %d bytes, want 16", len(key))
	}

	if _, err := DNSCryptKey("too short"); err == nil {
		t.Fatal("expected an invalid key to be rejected")
	}
}

// testResponseWriter captures the answer of the DNS handler.
type testResponseWriter struct {
	message *dns.Msg
}

func (w *testResponseWriter) LocalAddr() net.Addr {
	return &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 53}
}

func (w *testResponseWriter) RemoteAddr() net.Addr {
	return &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 40000}
}

func (w *testResponseWriter) WriteMsg(message *dns.Msg) error {
	w.message = message
	return nil
}

func (w *testResponseWriter) Write(data []byte) (int, error) { return len(data), nil }
func (w *testResponseWriter) Close() error                   { return nil }
func (w *testResponseWriter) TsigStatus() error              { return nil }
func (w *testResponseWriter) TsigTimersOnly(bool)            {}
func (w *testResponseWriter) Hijack()                        {}

// agentQuery builds a query the same way the agent does: the payload is
// encrypted with RC4, encoded with base32 and carried in the QNAME.
func agentQuery(t *testing.T, transport *TransportDNS, op string, sequence uint32, payload []byte, qtype uint16) *dns.Msg {
	t.Helper()

	key, err := rc4Key(transport.Config.EncryptKey)
	if err != nil {
		t.Fatalf("rc4Key: %v", err)
	}

	cipher, err := rc4.NewCipher(key)
	if err != nil {
		t.Fatalf("rc4.NewCipher: %v", err)
	}

	encrypted := make([]byte, len(payload))
	copy(encrypted, payload)
	cipher.XORKeyStream(encrypted, encrypted)

	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(encrypted)

	labels := ""
	for i := 0; i < len(encoded); i += 63 {
		if i > 0 {
			labels += "."
		}
		end := min(i+63, len(encoded))
		labels += encoded[i:end]
	}

	name := fmt.Sprintf("deadbeef.%s.%08x.0.%s.%s.", op, sequence^seqXorMask, labels, transport.Config.Domain)

	message := new(dns.Msg)
	message.SetQuestion(name, qtype)

	return message
}

// TestProtocolRoundTrip walks through the exchange of the agent and the
// teamserver: an upload is acknowledged and the answer of the teamserver is
// downloaded in frames.
func TestProtocolRoundTrip(t *testing.T) {
	transport := &TransportDNS{Config: testDNSConfig()}

	payload := []byte("the quick brown fox jumps over the lazy dog")

	// the agent splits the message into frames of [total][offset][chunk]
	frame := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(frame[0:4], uint32(len(payload)))
	binary.BigEndian.PutUint32(frame[4:8], 0)
	copy(frame[8:], payload)

	writer := &testResponseWriter{}
	transport.handleDNS(writer, agentQuery(t, transport, "cdn", 1, frame, dns.TypeA))

	if writer.message == nil || len(writer.message.Answer) != 1 {
		t.Fatal("the handler did not answer the upload")
	}

	record, ok := writer.message.Answer[0].(*dns.A)
	if !ok {
		t.Fatalf("the answer is %T, want *dns.A", writer.message.Answer[0])
	}

	ip := record.A.To4()
	if ip[0]&0x01 != 0x01 {
		t.Fatalf("flags = %#x, want the complete flag", ip[0])
	}
	if ip[1] != 0x00 || ip[2] != 0x00 || ip[3] != byte(len(payload)) {
		t.Fatalf("nextExpectedOff = %#x, want %d", ip[1:4], len(payload))
	}

	// the answer of the teamserver is downloaded with a GET request
	response := []byte("the answer of the teamserver")
	transport.downFrags = map[string]*dnsDownBuf{
		"deadbeef": newDownBuf(response, 0),
	}

	request := make([]byte, 4)
	binary.BigEndian.PutUint32(request, 0)

	writer = &testResponseWriter{}
	transport.handleDNS(writer, agentQuery(t, transport, "api", 2, request, dns.TypeTXT))

	if writer.message == nil || len(writer.message.Answer) != 1 {
		t.Fatal("the handler did not answer the download")
	}

	text, ok := writer.message.Answer[0].(*dns.TXT)
	if !ok {
		t.Fatalf("the answer is %T, want *dns.TXT", writer.message.Answer[0])
	}

	encoded := ""
	for _, value := range text.Txt {
		encoded += value
	}

	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("base64 decode: %v", err)
	}

	decrypted := transport.crypt(decoded)
	if len(decrypted) < 8 {
		t.Fatalf("the frame is %d bytes, want at least 8", len(decrypted))
	}
	if binary.BigEndian.Uint32(decrypted[0:4]) != uint32(len(response)) {
		t.Fatalf("total = %d, want %d", binary.BigEndian.Uint32(decrypted[0:4]), len(response))
	}
	if binary.BigEndian.Uint32(decrypted[4:8]) != 0 {
		t.Fatalf("offset = %d, want 0", binary.BigEndian.Uint32(decrypted[4:8]))
	}
	if string(decrypted[8:]) != string(response) {
		t.Fatalf("chunk = %q, want %q", string(decrypted[8:]), string(response))
	}
}
