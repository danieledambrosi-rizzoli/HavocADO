package crypt

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"fmt"

	"Havoc/pkg/logger"
)

func XCryptBytesAES256(XBytes []byte, AESKey []byte, AESIv []byte) []byte {
    var (
        ReverseXBytes = make([]byte, len(XBytes))
    )

    block, err := aes.NewCipher(AESKey)
    if err != nil {
        logger.Error("Decryption Error: " + err.Error())
        return []byte{}
    }

    stream := cipher.NewCTR(block, AESIv)
    stream.XORKeyStream(ReverseXBytes, XBytes)

    fmt.Println("Encryption Key", hex.EncodeToString(AESKey))
    fmt.Println("IV Key", hex.EncodeToString(AESIv))
    
    return ReverseXBytes
}