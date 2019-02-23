package authn

import (
	"crypto/sha256"
	"encoding/base64"

	"github.com/jmoiron/sqlx"
	"golang.org/x/crypto/bcrypt"
)

// CheckPassword verifies that the password is correct for the specified user.
// It returns true if the username and password are valid.
func CheckPassword(tx *sqlx.Tx, uid int, password string) bool {
	var (
		userPassword string
		hashed       [32]byte
		encoded      []byte
		err          error
	)

	// Get the user data.
	if err = tx.QueryRow(`SELECT password FROM user WHERE id=?`, uid).Scan(&userPassword); err != nil {
		panic(err)
	}

	// Prepare the password for bcrypt.  Raw bcrypt has a 72 character
	// maximum (bad for pass-phrases) and doesn't allow NUL characters (bad
	// for binary).  So we start by hashing and base64-encoding the result.
	// That's what we use as the actual password.
	hashed = sha256.Sum256([]byte(password))
	encoded = make([]byte, base64.StdEncoding.EncodedLen(len(hashed)))
	base64.StdEncoding.Encode(encoded, hashed[:])

	// Compare the passwords.
	return bcrypt.CompareHashAndPassword([]byte(userPassword), encoded) == nil
}
