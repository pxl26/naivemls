/*
	HPKE: Hybrid Public Key Encryption

	1: Diffie-hellman to arragement of shared secret
	2: HKDF to derive key
	3: AES-GCM to encrypt/decrypt
*/

package ratchettree

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"

	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"
)

func HPKE_Encrypt(receiverPubKey []byte, payload []byte) (ephemeralPubKey []byte, ciphertext []byte, err error) {
	// TODO: learn about how it work :D
	var ephPriv, ephPub [32]byte
	if _, err := io.ReadFull(rand.Reader, ephPriv[:]); err != nil {
		return nil, nil, err
	}
	curve25519.ScalarBaseMult(&ephPub, &ephPriv)

	sharedSecret, err := curve25519.X25519(ephPriv[:], receiverPubKey) // diffie-hellman
	if err != nil {
		return nil, nil, err
	}

	aesKey := make([]byte, 32)
	kdf := hkdf.New(sha256.New, sharedSecret, nil, []byte("mls_hybrid_encryption"))
	if _, err := io.ReadFull(kdf, aesKey); err != nil {
		return nil, nil, err
	}

	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}

	ciphertext = gcm.Seal(nonce, nonce, payload, nil)

	return ephPub[:], ciphertext, nil
}

func HPKE_Decrypt(receiverPrivKey []byte, ephemeralPubKey []byte, ciphertext []byte) (payload []byte, err error) {
	sharedSecret, err := curve25519.X25519(receiverPrivKey, ephemeralPubKey) // diffie-hellman
	if err != nil {
		return nil, err
	}

	aesKey := make([]byte, 32)
	kdf := hkdf.New(sha256.New, sharedSecret, nil, []byte("mls_hybrid_encryption"))
	if _, err := io.ReadFull(kdf, aesKey); err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce, actualCiphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]

	payload, err = gcm.Open(nil, nonce, actualCiphertext, nil)
	if err != nil {
		return nil, err
	}

	return payload, nil
}
