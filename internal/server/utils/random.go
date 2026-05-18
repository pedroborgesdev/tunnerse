package utils

import (
	"math/rand"
)

func RandomCode(width int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz123456789"
	code := make([]byte, width)
	for i := range code {
		code[i] = charset[rand.Intn(len(charset))]
	}
	return string(code)
}
