package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"math/big"
)

const (
	apiKeyPrefix = "bhi_sk_"
	base62Chars  = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
)

func GenerateAPIKey() (fullKey, keyHash, keyPrefix, keySuffix string, err error) {
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return
	}

	encoded := encodeBase62(raw)
	fullKey = apiKeyPrefix + encoded

	hash := sha256.Sum256([]byte(fullKey))
	keyHash = hex.EncodeToString(hash[:])

	keyPrefix = fullKey[:12]
	keySuffix = fullKey[len(fullKey)-4:]
	return
}

func HashAPIKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}

func IsBodhiAPIKey(key string) bool {
	return len(key) > len(apiKeyPrefix) && key[:len(apiKeyPrefix)] == apiKeyPrefix
}

func encodeBase62(data []byte) string {
	num := new(big.Int).SetBytes(data)
	base := big.NewInt(62)
	zero := big.NewInt(0)
	mod := new(big.Int)

	var result []byte
	for num.Cmp(zero) > 0 {
		num.DivMod(num, base, mod)
		result = append(result, base62Chars[mod.Int64()])
	}

	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}

	return string(result)
}
