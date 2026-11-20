package crypt

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"

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

    logger.FileLog("aes", "Encryption Key %s", hex.EncodeToString(AESKey))
    logger.FileLog("aes", "IV Key %s", hex.EncodeToString(AESIv))
    
    return ReverseXBytes
}