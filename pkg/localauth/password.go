package localauth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	// Argon2id parameters. These follow the OWASP recommendation of 19 MiB of memory,
	// 2 iterations, and 1 degree of parallelism.
	argonTime    = 2
	argonMemory  = 19 * 1024 // KiB
	argonThreads = 1
	argonKeyLen  = 32
	argonSaltLen = 16

	// minPasswordLength is the shortest password that can be set on a local user.
	minPasswordLength = 12
)

var (
	// ErrInvalidPassword is returned when a password does not match its hash.
	ErrInvalidPassword = errors.New("invalid password")

	errInvalidHash = errors.New("invalid password hash")
)

// HashPassword hashes a plaintext password with argon2id and returns it in the PHC string format,
// which carries the parameters and salt alongside the derived key.
func HashPassword(password string) (string, error) {
	if len(password) < minPasswordLength {
		return "", fmt.Errorf("password must be at least %d characters", minPasswordLength)
	}

	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("failed to generate salt: %w", err)
	}

	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// VerifyPassword checks a plaintext password against a PHC-encoded argon2id hash.
// It returns ErrInvalidPassword if the password does not match.
func VerifyPassword(encodedHash, password string) error {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return errInvalidHash
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return errInvalidHash
	} else if version != argon2.Version {
		return fmt.Errorf("%w: unsupported argon2 version %d", errInvalidHash, version)
	}

	var memory, time uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return errInvalidHash
	}

	salt, err := base64.RawStdEncoding.Strict().DecodeString(parts[4])
	if err != nil {
		return errInvalidHash
	}

	key, err := base64.RawStdEncoding.Strict().DecodeString(parts[5])
	if err != nil {
		return errInvalidHash
	}

	//nolint:gosec // the key length is bounded by the stored hash, which we produced.
	other := argon2.IDKey([]byte(password), salt, time, memory, threads, uint32(len(key)))
	if subtle.ConstantTimeCompare(key, other) != 1 {
		return ErrInvalidPassword
	}

	return nil
}
