package auth

import (
	"errors"
	"regexp"

	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 12

var usernameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{1,63}$`)

// UsernameRule describes what ValidUsername accepts.
const UsernameRule = "use 2–64 letters, digits, dots, dashes or underscores"

// ValidUsername reports whether s can be a username.
func ValidUsername(s string) bool { return usernameRe.MatchString(s) }

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
