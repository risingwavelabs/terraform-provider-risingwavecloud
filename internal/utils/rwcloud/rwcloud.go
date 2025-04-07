package rwcloud

import (
	"encoding/base64"
	"fmt"

	"github.com/google/uuid"
)

const base32Chars = "0123456789abcdefghijklmnopqrstuv"

func BuildEncodedClusterID(nsID uuid.UUID, name string) string {
	return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("rwc-%s-%s", ToBase32(nsID), name)))
}

func ToBase32(uuid uuid.UUID) string {
	chars := []rune(base32Chars)

	// Add bits `10` to have 130 total bits.
	// The first bit is `1` so that the ID always starts with a letter.
	var buffer uint = 2
	bufferLen := 2

	result := make([]rune, 0, 26 /* ceil(128 / 5) */)

	for i := 0; i < 16; i++ {
		buffer = (buffer << 8) | uint(uuid[i])
		bufferLen += 8

		for bufferLen >= 5 {
			shift := bufferLen - 5
			charIdx := (buffer >> shift) % 32
			result = append(result, chars[charIdx])

			bufferLen -= 5
		}
	}

	return string(result)
}
