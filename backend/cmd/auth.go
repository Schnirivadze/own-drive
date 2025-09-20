package main

import (
	"crypto/rand"
	"encoding/hex"
)

func generateRawToken() (string, error) {
	b := make([]byte, 32) 
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil 
}


