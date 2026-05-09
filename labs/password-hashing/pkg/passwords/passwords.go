// Package passwords implements password hashing across three stages:
//
//   - v0: bcrypt wrapper — intentionally slow by design (cost=10 → ~100ms)
//   - v1: Argon2id — memory-hard, the 2023 recommendation (m=64MB per hash)
//   - v2: PepperKeyStore — server-side secret + transparent key rotation
//
// Threat model: the database is stolen. An attacker has all hashed passwords
// and unlimited GPU time. The defenses:
//
//  1. Slow algorithms (bcrypt, Argon2id): each hash attempt costs ~100ms,
//     limiting an attacker to ~10,000 attempts/sec on a GPU (vs ~10 billion/sec
//     with SHA-256).
//  2. Memory-hardness (Argon2id): 64MB RAM required per hash. A GPU with 24GB
//     VRAM can only run 375 hashes in parallel, regardless of compute power.
//  3. Peppering: the hash in the DB is argon2id(password + pepper). Without the
//     pepper (a server-side secret), the stolen DB hashes are uncrackable.
package passwords

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

// ─── v0: bcrypt ────────────────────────────────────────────────────────────────

// HashBcrypt hashes password with the given cost factor using bcrypt.
//
// Cost determines the work factor: each increment doubles the computation time.
// Cost=10 targets ~100ms on a modern CPU. Cost=14 targets ~1,600ms (2^4 = 16x).
//
// The returned hash is self-describing: "$2a$10$<22-char-salt><31-char-hash>".
// The cost and salt are embedded, so you can verify the hash even after changing
// the cost factor — old hashes remember their own parameters.
func HashBcrypt(password string, cost int) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return "", fmt.Errorf("bcrypt: hash failed: %w", err)
	}
	return string(hash), nil
}

// VerifyBcrypt checks whether password matches a bcrypt hash.
//
// Always returns false on mismatch, never returns an error for wrong passwords.
// bcrypt.CompareHashAndPassword uses constant-time comparison internally.
func VerifyBcrypt(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// BcryptCostBenchmark times HashBcrypt at the given cost and returns the duration.
// This makes the cost/time relationship visible: cost=14 is 16x slower than cost=10.
func BcryptCostBenchmark(cost int) time.Duration {
	start := time.Now()
	_, _ = HashBcrypt("benchmark-password", cost)
	return time.Since(start)
}

// ─── v1: Argon2id ──────────────────────────────────────────────────────────────

// Argon2Params holds the tuning parameters for Argon2id.
//
// Memory is in KiB (not bytes). 64MB = 64*1024 = 65536 KiB.
// The encoded hash string embeds all params, making it self-describing.
type Argon2Params struct {
	Memory      uint32 // KiB — 65536 = 64MB (limits GPU parallelism via VRAM)
	Iterations  uint32 // time cost — number of passes over memory
	Parallelism uint8  // number of threads
	SaltLength  uint32 // bytes — 16 is standard
	KeyLength   uint32 // output hash bytes — 32 is standard
}

// DefaultArgon2Params returns the OWASP-recommended Argon2id parameters (2023).
//
// These target ~120ms on a modern server CPU. The 64MB memory requirement
// is the key defense against GPU attacks: a 24GB VRAM GPU can run at most
// 24,576 MB / 64 MB = 384 hashes in parallel (vs thousands for bcrypt).
func DefaultArgon2Params() Argon2Params {
	return Argon2Params{
		Memory:      64 * 1024, // 64 MB
		Iterations:  3,
		Parallelism: 4,
		SaltLength:  16,
		KeyLength:   32,
	}
}

// HashArgon2id hashes password+pepper using Argon2id.
//
// The pepper is a server-side secret appended to the password before hashing.
// It is NOT stored in the database — if the DB is stolen without the pepper,
// the hashes cannot be cracked.
//
// The returned encoded string is self-describing:
//
//	$argon2id$v=19$m=65536,t=3,p=4$<base64-salt>$<base64-hash>
//
// This format matches the Argon2 reference implementation and is interoperable
// with Java's Spring Security Argon2PasswordEncoder.
func HashArgon2id(password, pepper string, params Argon2Params) (string, error) {
	salt := make([]byte, params.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("argon2id: failed to generate salt: %w", err)
	}

	// Append pepper to password before hashing. The pepper never leaves the
	// application process (not stored in DB, not logged, not returned in responses).
	input := password + pepper

	hash := argon2.IDKey(
		[]byte(input),
		salt,
		params.Iterations,
		params.Memory,
		params.Parallelism,
		params.KeyLength,
	)

	// Encode in PHC string format: $argon2id$v=19$m=...,t=...,p=...$salt$hash
	encoded := fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		params.Memory,
		params.Iterations,
		params.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)
	return encoded, nil
}

// VerifyArgon2id checks whether password+pepper matches an encoded Argon2id hash.
//
// It parses the encoded string to extract parameters, recomputes the hash with
// the same salt, and compares using constant-time bytes.Equal. A wrong password
// takes the same time as a correct one (timing-safe).
func VerifyArgon2id(password, pepper, encoded string) bool {
	params, salt, hash, err := parseArgon2Encoded(encoded)
	if err != nil {
		return false
	}

	input := password + pepper

	candidate := argon2.IDKey(
		[]byte(input),
		salt,
		params.Iterations,
		params.Memory,
		params.Parallelism,
		params.KeyLength,
	)

	// subtle.ConstantTimeCompare: always compares all bytes regardless of where
	// the first mismatch occurs. Prevents timing attacks where an attacker
	// measures response time to learn which bytes of the hash match their guess.
	return subtle.ConstantTimeCompare(hash, candidate) == 1
}

// parseArgon2Encoded parses a PHC-format Argon2id encoded string.
//
// Format: $argon2id$v=19$m=65536,t=3,p=4$<base64-salt>$<base64-hash>
func parseArgon2Encoded(encoded string) (Argon2Params, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	// parts[0] = "" (before first $), parts[1] = "argon2id", parts[2] = "v=19",
	// parts[3] = "m=65536,t=3,p=4", parts[4] = base64-salt, parts[5] = base64-hash
	if len(parts) != 6 {
		return Argon2Params{}, nil, nil, errors.New("argon2id: invalid encoded format")
	}
	if parts[1] != "argon2id" {
		return Argon2Params{}, nil, nil, errors.New("argon2id: not an argon2id hash")
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return Argon2Params{}, nil, nil, fmt.Errorf("argon2id: bad version field: %w", err)
	}

	var params Argon2Params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d",
		&params.Memory, &params.Iterations, &params.Parallelism); err != nil {
		return Argon2Params{}, nil, nil, fmt.Errorf("argon2id: bad params field: %w", err)
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return Argon2Params{}, nil, nil, fmt.Errorf("argon2id: bad salt encoding: %w", err)
	}

	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return Argon2Params{}, nil, nil, fmt.Errorf("argon2id: bad hash encoding: %w", err)
	}

	params.SaltLength = uint32(len(salt))
	params.KeyLength = uint32(len(hash))

	return params, salt, hash, nil
}

// ─── v2: PepperKeyStore — server-side secret + transparent key rotation ────────

// PepperKey holds a versioned pepper secret.
//
// The pepper is a server-side secret that is NOT stored in the database.
// Storing it separately from the database (env var, secrets manager, HSM)
// means a stolen database alone cannot be used to crack passwords.
type PepperKey struct {
	Version int
	Secret  string // hex-encoded pepper bytes
}

// PepperKeyStore manages pepper key rotation.
//
// CurrentKey is used for all new hashes. PreviousKeys are retained so that
// hashes created with old peppers can still be verified (transparently, on login).
// needsRehash returns true when a verified password used an old pepper version,
// signaling the caller to re-hash with the current pepper on successful login.
type PepperKeyStore struct {
	CurrentKey   PepperKey
	PreviousKeys []PepperKey
	Params       Argon2Params
}

// HashResult is the output of PepperKeyStore.Hash.
type HashResult struct {
	Hash          string
	PepperVersion int
}

// VerifyResult is the output of PepperKeyStore.Verify.
type VerifyResult struct {
	OK          bool
	NeedsRehash bool // true if the pepper version is outdated — caller should rehash on login
}

// NewPepperKeyStore creates a PepperKeyStore with a single current key.
//
// Generate peppers with: head -c 32 /dev/urandom | hexdump -v -e '/1 "%02x"'
// In production: load from environment variables or a secrets manager.
func NewPepperKeyStore(currentSecret string, params Argon2Params) *PepperKeyStore {
	return &PepperKeyStore{
		CurrentKey: PepperKey{Version: 1, Secret: currentSecret},
		Params:     params,
	}
}

// AddCurrentKey rotates the pepper. The old current key moves to PreviousKeys.
// All new hashes will use the new pepper. Old hashes still verify via PreviousKeys.
func (s *PepperKeyStore) AddCurrentKey(newVersion int, newSecret string) {
	s.PreviousKeys = append(s.PreviousKeys, s.CurrentKey)
	s.CurrentKey = PepperKey{Version: newVersion, Secret: newSecret}
}

// Hash creates an Argon2id hash using the current pepper key.
//
// The returned HashResult includes the pepper version so the caller can store it
// alongside the hash and pass it back to Verify later.
func (s *PepperKeyStore) Hash(password string) (HashResult, error) {
	pepper := decodePepper(s.CurrentKey.Secret)
	encoded, err := HashArgon2id(password, pepper, s.Params)
	if err != nil {
		return HashResult{}, err
	}
	return HashResult{Hash: encoded, PepperVersion: s.CurrentKey.Version}, nil
}

// Verify checks password against a hash created with the given pepper version.
//
// If the stored pepperVersion matches the current key, verification is straightforward.
// If it matches a previous key, we verify with that old pepper and set NeedsRehash=true.
// The caller should re-hash with the current pepper on a successful login (transparent upgrade).
//
// NeedsRehash is also true when the stored hash was created with different Argon2id
// parameters (detected by parsing the encoded string).
func (s *PepperKeyStore) Verify(password, hash string, pepperVersion int) VerifyResult {
	// Find the pepper for this version
	pepper, found := s.findPepper(pepperVersion)
	if !found {
		return VerifyResult{OK: false}
	}

	ok := VerifyArgon2id(password, decodePepper(pepper.Secret), hash)
	if !ok {
		return VerifyResult{OK: false}
	}

	// Rehash if: (a) the pepper version is outdated, or (b) the hash params differ
	needsRehash := pepper.Version != s.CurrentKey.Version || s.paramsChanged(hash)
	return VerifyResult{OK: true, NeedsRehash: needsRehash}
}

// findPepper locates the pepper for a given version number.
func (s *PepperKeyStore) findPepper(version int) (PepperKey, bool) {
	if s.CurrentKey.Version == version {
		return s.CurrentKey, true
	}
	for _, k := range s.PreviousKeys {
		if k.Version == version {
			return k, true
		}
	}
	return PepperKey{}, false
}

// paramsChanged returns true if the hash was created with different Argon2id params
// than the store's current params. This detects when params were upgraded.
func (s *PepperKeyStore) paramsChanged(encoded string) bool {
	storedParams, _, _, err := parseArgon2Encoded(encoded)
	if err != nil {
		return false
	}
	return storedParams.Memory != s.Params.Memory ||
		storedParams.Iterations != s.Params.Iterations ||
		storedParams.Parallelism != s.Params.Parallelism
}

// decodePepper returns the pepper string for use in Argon2id.
// If the secret is valid hex, it decodes it to raw bytes; otherwise uses it as-is.
// Production peppers should be 32 random bytes (64 hex characters).
func decodePepper(secret string) string {
	b, err := hex.DecodeString(secret)
	if err != nil {
		return secret // treat as raw string for test convenience
	}
	return string(b)
}
