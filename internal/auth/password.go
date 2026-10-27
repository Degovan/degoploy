package auth

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 12

// MinPasswordLen is enforced for local accounts.
const MinPasswordLen = 10

var ErrWeakPassword = errors.New("password must be at least 10 characters")

// dummyHash lets VerifyPassword spend the same time when the user does not exist.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("degoploy-timing-equalizer"), bcryptCost)

func HashPassword(pw string) (string, error) {
	if len(pw) < MinPasswordLen {
		return "", ErrWeakPassword
	}
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcryptCost)
	return string(h), err
}

// VerifyPassword compares pw to hash; an empty hash still burns a bcrypt round.
func VerifyPassword(hash, pw string) bool {
	if hash == "" {
		bcrypt.CompareHashAndPassword(dummyHash, []byte(pw))
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}
