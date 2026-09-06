package ratchettree

import (
	"bytes"
	"testing"
)

func TestEncrypt(t *testing.T) {
	reciverPrivKey, receiverPubKey := GenerateKeypair([]byte("test secret"))

	msg := []byte("Hello man")
	ephPub, ciphertext, err := HPKE_Encrypt(receiverPubKey, msg)
	if err != nil {
		t.Fatalf("failed to encrypt: %v", err)
	}

	decryptedMsg, err := HPKE_Decrypt(reciverPrivKey, ephPub, ciphertext)
	if err != nil {
		t.Fatalf("failed to decrypt: %v", err)
	}

	if !bytes.Equal(msg, decryptedMsg) {
		t.Fatalf("decrypted message does not match original message")
	}

	println("decrypted message: ", string(decryptedMsg))
}
