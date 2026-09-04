package mls

import (
	"bytes"
	"testing"

	"golang.org/x/crypto/curve25519"
)

func TestGenerateKeypairReturnsPrivateThenPublic(t *testing.T) {
	privateKey, publicKey := GenerateKeypair([]byte("test secret"))
	wantPublicKey, err := curve25519.X25519(privateKey, curve25519.Basepoint)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(publicKey, wantPublicKey) {
		t.Fatal("public key does not match the returned private key")
	}
}
