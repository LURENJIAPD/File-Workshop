package domain

import "github.com/alexedwards/argon2id"

type PasswordHasher interface {
	Hash(password string) (string, error)
	Compare(password, encodedHash string) (bool, error)
}

type Argon2IDHasher struct {
	params *argon2id.Params
}

func NewArgon2IDHasher() *Argon2IDHasher {
	return &Argon2IDHasher{params: &argon2id.Params{
		Memory:      19 * 1024,
		Iterations:  2,
		Parallelism: 1,
		SaltLength:  16,
		KeyLength:   32,
	}}
}

func (h *Argon2IDHasher) Hash(password string) (string, error) {
	return argon2id.CreateHash(password, h.params)
}

func (h *Argon2IDHasher) Compare(password, encodedHash string) (bool, error) {
	return argon2id.ComparePasswordAndHash(password, encodedHash)
}
