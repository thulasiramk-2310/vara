package identity

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Password hashing for RFC-0020 §6.1: a salted, memory-hard KDF (argon2id), NEVER
// a fast hash and NEVER reversible. Hashes are stored in the standard PHC string
// form ($argon2id$v=..$m=..,t=..,p=..$salt$hash), which is self-describing so the
// cost can be raised over time and old hashes upgraded on next successful login
// (S3). Verification is constant-time.

type argon2Params struct {
	memory  uint32 // KiB
	time    uint32 // iterations
	threads uint8
	keyLen  uint32
	saltLen uint32
}

// defaultArgon2 targets tens of milliseconds per verification on commodity
// hardware (64 MiB, 2 passes). Raise these over time; stored hashes carry their
// own parameters so old ones still verify and are re-hashed on login.
var defaultArgon2 = argon2Params{memory: 64 * 1024, time: 2, threads: 4, keyLen: 32, saltLen: 16}

// HashPassword returns a PHC-encoded argon2id hash of plain with a fresh random
// salt. The plaintext is never stored or returned.
func HashPassword(plain string) (string, error) {
	salt := make([]byte, defaultArgon2.saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	p := defaultArgon2
	sum := argon2.IDKey([]byte(plain), salt, p.time, p.memory, p.threads, p.keyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.memory, p.time, p.threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(sum),
	), nil
}

// VerifyPassword reports whether plain matches the PHC-encoded hash, and whether
// the stored hash uses weaker-than-current parameters and should be re-hashed. It
// compares in constant time; the KDF's own cost dominates timing either way.
func VerifyPassword(encoded, plain string) (match bool, needsRehash bool, err error) {
	p, salt, want, err := parsePHC(encoded)
	if err != nil {
		return false, false, err
	}
	got := argon2.IDKey([]byte(plain), salt, p.time, p.memory, p.threads, uint32(len(want)))
	match = subtle.ConstantTimeCompare(got, want) == 1
	needsRehash = p.memory != defaultArgon2.memory || p.time != defaultArgon2.time || p.threads != defaultArgon2.threads
	return match, needsRehash, nil
}

// dummyHash is a valid argon2id hash of a fixed string, used to spend the same
// KDF time on a login for a nonexistent/disabled account as for a real one, so
// timing does not reveal which usernames exist (RFC-0020 §12, S3).
var dummyHash, _ = HashPassword("vara-constant-time-placeholder")

// SpendVerifyTime runs a verification against the dummy hash and discards the
// result. Call it on the absent/disabled-account path of a login so that path
// costs the same as a real verification.
func SpendVerifyTime(plain string) {
	_, _, _ = VerifyPassword(dummyHash, plain)
}

func parsePHC(encoded string) (argon2Params, []byte, []byte, error) {
	// $argon2id$v=19$m=65536,t=2,p=4$<salt>$<hash>
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return argon2Params{}, nil, nil, fmt.Errorf("password: not an argon2id PHC string")
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return argon2Params{}, nil, nil, fmt.Errorf("password: unsupported argon2 version")
	}
	var p argon2Params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.memory, &p.time, &p.threads); err != nil {
		return argon2Params{}, nil, nil, fmt.Errorf("password: bad argon2 parameters")
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return argon2Params{}, nil, nil, fmt.Errorf("password: bad salt encoding")
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return argon2Params{}, nil, nil, fmt.Errorf("password: bad hash encoding")
	}
	return p, salt, hash, nil
}
