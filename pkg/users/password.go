package users

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

type Argon2idParams struct {
	Memory      uint32
	Iterations  uint32
	Parallelism uint8
	SaltLen     uint32
	KeyLen      uint32
}

const MinPasswordLength = 6

var (
	ErrPasswordRequired = errors.New("password is required")
	ErrPasswordTooShort = errors.New("password is too short")
)

var DefaultArgon2idParams = Argon2idParams{
	Memory:      64 * 1024,
	Iterations:  3,
	Parallelism: 2,
	SaltLen:     16,
	KeyLen:      32,
}

func ValidatePassword(password string) error {
	if strings.TrimSpace(password) == "" {
		return ErrPasswordRequired
	}
	if len(password) < MinPasswordLength {
		return ErrPasswordTooShort
	}
	return nil
}

// HashPassword returns a PHC-encoded Argon2id hash string.
// Example:
//
//	$argon2id$v=19$m=65536,t=3,p=2$<salt>$<hash>
func HashPassword(password string, p Argon2idParams) (string, error) {
	if password == "" {
		return "", fmt.Errorf("password is required")
	}
	if p.Memory == 0 || p.Iterations == 0 || p.Parallelism == 0 || p.SaltLen == 0 || p.KeyLen == 0 {
		return "", fmt.Errorf("invalid argon2id params")
	}

	salt := make([]byte, p.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("read salt: %w", err)
	}

	hash := argon2.IDKey([]byte(password), salt, p.Iterations, p.Memory, p.Parallelism, p.KeyLen)

	b64 := base64.RawStdEncoding
	saltB64 := b64.EncodeToString(salt)
	hashB64 := b64.EncodeToString(hash)

	encoded := fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		p.Memory, p.Iterations, p.Parallelism, saltB64, hashB64)
	return encoded, nil
}

// VerifyPassword checks a plaintext password against a PHC-encoded Argon2id hash.
func VerifyPassword(password, encoded string) (bool, error) {
	if password == "" {
		return false, fmt.Errorf("password is required")
	}
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return false, fmt.Errorf("hash is required")
	}

	parts := strings.Split(encoded, "$")
	// Expect: ["", "argon2id", "v=19", "m=...,t=...,p=...", "<salt>", "<hash>"]
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, fmt.Errorf("invalid argon2id hash format")
	}
	if parts[2] != "v=19" {
		return false, fmt.Errorf("unsupported argon2id version")
	}

	var p Argon2idParams
	pStr := parts[3]
	for _, kv := range strings.Split(pStr, ",") {
		kv = strings.TrimSpace(kv)
		if kv == "" {
			continue
		}
		s := strings.SplitN(kv, "=", 2)
		if len(s) != 2 {
			return false, fmt.Errorf("invalid argon2id params")
		}
		key := s[0]
		val := s[1]
		switch key {
		case "m":
			u, err := strconv.ParseUint(val, 10, 32)
			if err != nil {
				return false, fmt.Errorf("invalid argon2id memory")
			}
			p.Memory = uint32(u)
		case "t":
			u, err := strconv.ParseUint(val, 10, 32)
			if err != nil {
				return false, fmt.Errorf("invalid argon2id iterations")
			}
			p.Iterations = uint32(u)
		case "p":
			u, err := strconv.ParseUint(val, 10, 8)
			if err != nil {
				return false, fmt.Errorf("invalid argon2id parallelism")
			}
			p.Parallelism = uint8(u)
		default:
			return false, fmt.Errorf("unknown argon2id param %q", key)
		}
	}

	b64 := base64.RawStdEncoding
	salt, err := b64.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("invalid argon2id salt encoding")
	}
	hash, err := b64.DecodeString(parts[5])
	if err != nil {
		return false, fmt.Errorf("invalid argon2id hash encoding")
	}
	if len(hash) == 0 {
		return false, fmt.Errorf("invalid argon2id hash length")
	}

	// Derive with the parameters and compare in constant time.
	other := argon2.IDKey([]byte(password), salt, p.Iterations, p.Memory, p.Parallelism, uint32(len(hash)))
	if subtle.ConstantTimeCompare(hash, other) == 1 {
		return true, nil
	}
	return false, nil
}
