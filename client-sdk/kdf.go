// https://datatracker.ietf.org/doc/html/rfc9420#section-8
package mls

import (
	"crypto/hkdf"
	"crypto/sha256"

	"golang.org/x/crypto/curve25519"
)

// DeriveParentSecret derives a parent secret from the current node secret.
func DeriveParentSecret(secret []byte) []byte {
	return expandWithLabel(secret, "parent_secret", 32)
}

func GenerateKeypair(secret []byte) (privateKeyBytes, publicKeyBytes []byte) {
	var (
		nodeSecret = deriveNodeSecret(secret)
		privateKey = [32]byte{}
		publicKey  = [32]byte{}
	)

	copy(privateKey[:], nodeSecret)
	copy(publicKey[:], nodeSecret)

	curve25519.ScalarBaseMult(&publicKey, &privateKey)

	return privateKey[:], publicKey[:]
}

// deriveNodeSecret generates the key-pair material for a node.
func deriveNodeSecret(secret []byte) []byte {
	return expandWithLabel(secret, "generate_keypair", 32)
}

// ExpandWithLabel(Secret, Label, Context, Length) = KDF.Expand(Secret, KDFLabel, Length)
// KDFLabel = Context || L || Label
func expandWithLabel(secret []byte, label string, length int) []byte {
	label = "MLS 1.0 " + label // TODO: use the RFC 9420 section 8.3 label structure.
	rs, err := hkdf.Expand(sha256.New, secret, label, length)
	if err != nil {
		panic(err)
	}
	return rs
}
