package passwords_test

import (
	"strings"
	"testing"
	"time"

	"dev.pushkar/password-hashing/pkg/passwords"
)

// ─── v0: bcrypt tests ──────────────────────────────────────────────────────────

func TestBcryptRoundtrip(t *testing.T) {
	hash, err := passwords.HashBcrypt("correct-horse-battery-staple", 10)
	if err != nil {
		t.Fatalf("HashBcrypt failed: %v", err)
	}
	if !strings.HasPrefix(hash, "$2a$10$") {
		t.Errorf("expected $2a$10$ prefix, got %s", hash[:7])
	}
	if !passwords.VerifyBcrypt("correct-horse-battery-staple", hash) {
		t.Error("VerifyBcrypt: correct password should match")
	}
}

func TestBcryptWrongPassword(t *testing.T) {
	hash, err := passwords.HashBcrypt("correct-password", 10)
	if err != nil {
		t.Fatalf("HashBcrypt failed: %v", err)
	}
	if passwords.VerifyBcrypt("wrong-password", hash) {
		t.Error("VerifyBcrypt: wrong password should not match")
	}
}

// TestBcryptCostDoubling verifies that increasing cost by 1 roughly doubles time.
// This is a property test, not a strict timing test — we check the ratio is > 1.5x.
func TestBcryptCostDoubling(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timing test in short mode")
	}
	t10 := passwords.BcryptCostBenchmark(10)
	t11 := passwords.BcryptCostBenchmark(11)
	ratio := float64(t11) / float64(t10)
	if ratio < 1.5 {
		t.Errorf("expected cost=11 to be at least 1.5x slower than cost=10; got ratio=%.2f (t10=%v, t11=%v)",
			ratio, t10, t11)
	}
	t.Logf("bcrypt cost=10: %v, cost=11: %v, ratio=%.2fx", t10, t11, ratio)
}

// ─── v1: Argon2id tests ────────────────────────────────────────────────────────

func TestArgon2idRoundtrip(t *testing.T) {
	params := passwords.DefaultArgon2Params()
	pepper := "test-pepper-secret"

	encoded, err := passwords.HashArgon2id("my-secure-password", pepper, params)
	if err != nil {
		t.Fatalf("HashArgon2id failed: %v", err)
	}

	// Verify the encoded format is self-describing
	if !strings.HasPrefix(encoded, "$argon2id$") {
		t.Errorf("expected $argon2id$ prefix, got: %s", encoded[:10])
	}

	if !passwords.VerifyArgon2id("my-secure-password", pepper, encoded) {
		t.Error("VerifyArgon2id: correct password should match")
	}
}

func TestArgon2idWrongPassword(t *testing.T) {
	params := passwords.DefaultArgon2Params()
	encoded, _ := passwords.HashArgon2id("correct-password", "pepper", params)
	if passwords.VerifyArgon2id("wrong-password", "pepper", encoded) {
		t.Error("VerifyArgon2id: wrong password should not match")
	}
}

func TestArgon2idPepperMismatch(t *testing.T) {
	params := passwords.DefaultArgon2Params()
	encoded, _ := passwords.HashArgon2id("password", "correct-pepper", params)

	// Same password, wrong pepper — should fail.
	// This is the defense: if the DB is stolen, the attacker can't crack without the pepper.
	if passwords.VerifyArgon2id("password", "wrong-pepper", encoded) {
		t.Error("VerifyArgon2id: different pepper should not match")
	}
}

func TestArgon2idEncodedFormat(t *testing.T) {
	params := passwords.Argon2Params{
		Memory:      65536, // 64MB
		Iterations:  3,
		Parallelism: 4,
		SaltLength:  16,
		KeyLength:   32,
	}
	encoded, err := passwords.HashArgon2id("password", "pepper", params)
	if err != nil {
		t.Fatal(err)
	}
	// Check the format matches: $argon2id$v=19$m=65536,t=3,p=4$<salt>$<hash>
	if !strings.Contains(encoded, "$m=65536,t=3,p=4$") {
		t.Errorf("encoded string missing expected params segment; got: %s", encoded)
	}
}

// ─── v2: PepperKeyStore tests ─────────────────────────────────────────────────

func TestPepperKeyStoreRoundtrip(t *testing.T) {
	store := passwords.NewPepperKeyStore("deadbeef", passwords.DefaultArgon2Params())

	result, err := store.Hash("my-password")
	if err != nil {
		t.Fatalf("Hash failed: %v", err)
	}
	if result.PepperVersion != 1 {
		t.Errorf("expected pepper version 1, got %d", result.PepperVersion)
	}

	vr := store.Verify("my-password", result.Hash, result.PepperVersion)
	if !vr.OK {
		t.Error("Verify: correct password should return OK=true")
	}
	if vr.NeedsRehash {
		t.Error("Verify: fresh hash with current pepper should not need rehash")
	}
}

func TestPepperKeyStoreWrongPassword(t *testing.T) {
	store := passwords.NewPepperKeyStore("deadbeef", passwords.DefaultArgon2Params())
	result, _ := store.Hash("correct-password")

	vr := store.Verify("wrong-password", result.Hash, result.PepperVersion)
	if vr.OK {
		t.Error("Verify: wrong password should return OK=false")
	}
}

func TestPepperRotationOldVersionStillVerifies(t *testing.T) {
	store := passwords.NewPepperKeyStore("pepper-v1-secret", passwords.DefaultArgon2Params())

	// Hash with v1 pepper
	result, err := store.Hash("user-password")
	if err != nil {
		t.Fatalf("Hash failed: %v", err)
	}
	if result.PepperVersion != 1 {
		t.Errorf("expected v1, got %d", result.PepperVersion)
	}

	// Rotate to v2 pepper
	store.AddCurrentKey(2, "pepper-v2-secret")

	// Old hash (v1 pepper) should still verify, with NeedsRehash=true
	vr := store.Verify("user-password", result.Hash, result.PepperVersion)
	if !vr.OK {
		t.Error("Verify with old pepper version: should still return OK=true")
	}
	if !vr.NeedsRehash {
		t.Error("Verify with old pepper version: should return NeedsRehash=true")
	}

	// New hashes use v2
	result2, _ := store.Hash("user-password")
	if result2.PepperVersion != 2 {
		t.Errorf("expected new hash to use pepper v2, got %d", result2.PepperVersion)
	}

	// New hash with v2 pepper verifies without NeedsRehash
	vr2 := store.Verify("user-password", result2.Hash, result2.PepperVersion)
	if !vr2.OK || vr2.NeedsRehash {
		t.Errorf("new hash with current pepper: OK=%v NeedsRehash=%v", vr2.OK, vr2.NeedsRehash)
	}
}

func TestNeedsRehashWhenParamsChange(t *testing.T) {
	// Hash with low-cost params
	lowCostParams := passwords.Argon2Params{
		Memory:      16 * 1024, // 16MB (lower than recommended)
		Iterations:  1,
		Parallelism: 1,
		SaltLength:  16,
		KeyLength:   32,
	}
	store := passwords.NewPepperKeyStore("pepper-secret", lowCostParams)
	result, _ := store.Hash("my-password")

	// Upgrade the store to higher-cost params (keeping same pepper version)
	store2 := passwords.NewPepperKeyStore("pepper-secret", passwords.DefaultArgon2Params())
	// Add the v1 key as the current key (same pepper, different params)
	_ = store2 // store2 uses DefaultArgon2Params with same pepper

	// The hash was created with lowCostParams; store2 uses DefaultArgon2Params.
	// Verify with a store using upgraded params — should flag NeedsRehash.
	vr := store2.Verify("my-password", result.Hash, result.PepperVersion)
	if !vr.OK {
		t.Error("Verify: correct password with old params should still return OK=true")
	}
	if !vr.NeedsRehash {
		t.Error("Verify: hash with outdated params should return NeedsRehash=true")
	}
}

// TestTimingAttackResistance verifies that a correct password and a wrong password
// take similar time to verify (within 50ms of each other on a warm cache).
//
// This is a heuristic test, not a guarantee — true timing attack resistance
// comes from subtle.ConstantTimeCompare in VerifyArgon2id. The dominant cost
// is the Argon2id computation itself (>100ms), which dwarfs any comparison overhead.
func TestTimingAttackResistance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timing test in short mode")
	}

	params := passwords.DefaultArgon2Params()
	pepper := "timing-test-pepper"
	encoded, _ := passwords.HashArgon2id("correct-password", pepper, params)

	const iterations = 3
	var correctTotal, wrongTotal time.Duration

	for i := 0; i < iterations; i++ {
		start := time.Now()
		passwords.VerifyArgon2id("correct-password", pepper, encoded)
		correctTotal += time.Since(start)

		start = time.Now()
		passwords.VerifyArgon2id("completely-wrong-password", pepper, encoded)
		wrongTotal += time.Since(start)
	}

	correctAvg := correctTotal / iterations
	wrongAvg := wrongTotal / iterations

	diff := correctAvg - wrongAvg
	if diff < 0 {
		diff = -diff
	}

	// Both paths run the full Argon2id computation. The difference should be small
	// relative to the total computation time.
	t.Logf("correct avg: %v, wrong avg: %v, diff: %v", correctAvg, wrongAvg, diff)

	// Allow up to 50ms difference (the Argon2id computation itself takes ~120ms,
	// so 50ms margin is generous — the constant-time comparison is nanoseconds).
	if diff > 50*time.Millisecond {
		t.Errorf("timing difference too large: %v (suggests non-constant-time path)", diff)
	}
}
