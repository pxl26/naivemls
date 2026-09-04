package mls

// MessageCodec is the client-side boundary for MLS application-message
// encryption and decryption.
type MessageCodec interface {
	Encrypt(epoch uint64, plaintext []byte) ([]byte, error)
	Decrypt(epoch uint64, ciphertext []byte) ([]byte, error)
}

// PassthroughCodec is useful while MLS application-message encryption is being
// implemented. It does not provide encryption and must not be used in production.
type PassthroughCodec struct{}

func (PassthroughCodec) Encrypt(_ uint64, plaintext []byte) ([]byte, error) {
	return append([]byte(nil), plaintext...), nil
}

func (PassthroughCodec) Decrypt(_ uint64, ciphertext []byte) ([]byte, error) {
	return append([]byte(nil), ciphertext...), nil
}
