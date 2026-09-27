package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"testing"
)

func TestPassword(t *testing.T) {
	h, err := HashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(h, "correct horse") {
		t.Fatal("right password rejected")
	}
	if CheckPassword(h, "wrong horse") {
		t.Fatal("wrong password accepted")
	}
	if CheckPassword("garbage", "x") {
		t.Fatal("malformed hash accepted")
	}
}

func enc(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func authData(rpID string, flags byte, count uint32) []byte {
	h := sha256.Sum256([]byte(rpID))
	out := append([]byte{}, h[:]...)
	out = append(out, flags)
	var c [4]byte
	binary.BigEndian.PutUint32(c[:], count)
	return append(out, c[:]...)
}

func TestPasskeyRoundTrip(t *testing.T) {
	const rp = "wackcluborchard.example.com"
	origins := []string{"https://wackcluborchard.example.com"}
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	spki, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)

	regCD, _ := json.Marshal(map[string]string{"type": "webauthn.create", "challenge": "c1", "origin": origins[0]})
	pk, count, err := VerifyRegistration(Registration{ID: "cred", ClientDataJSON: enc(regCD), AuthenticatorData: enc(authData(rp, 0x45, 0)), PublicKey: enc(spki)}, "c1", rp, origins)
	if err != nil {
		t.Fatal(err)
	}

	sign := func(challenge, origin string, n uint32) Assertion {
		cd, _ := json.Marshal(map[string]string{"type": "webauthn.get", "challenge": challenge, "origin": origin})
		ad := authData(rp, 0x05, n)
		cdh := sha256.Sum256(cd)
		digest := sha256.Sum256(append(append([]byte{}, ad...), cdh[:]...))
		sig, _ := ecdsa.SignASN1(rand.Reader, key, digest[:])
		return Assertion{ID: "cred", ClientDataJSON: enc(cd), AuthenticatorData: enc(ad), Signature: enc(sig)}
	}
	if n, err := VerifyAssertion(sign("c2", origins[0], 1), pk, count, "c2", rp, origins); err != nil || n != 1 {
		t.Fatalf("valid assertion: n=%d err=%v", n, err)
	}
	if _, err := VerifyAssertion(sign("c3", origins[0], 2), pk, 1, "other", rp, origins); err == nil {
		t.Fatal("challenge mismatch accepted")
	}
	if _, err := VerifyAssertion(sign("c4", "https://evil.example", 3), pk, 1, "c4", rp, origins); err == nil {
		t.Fatal("foreign origin accepted")
	}
	if _, err := VerifyAssertion(sign("c5", origins[0], 1), pk, 5, "c5", rp, origins); err == nil {
		t.Fatal("non-increasing counter accepted")
	}
	bad := sign("c6", origins[0], 9)
	bad.Signature = enc([]byte("not a signature"))
	if _, err := VerifyAssertion(bad, pk, 1, "c6", rp, origins); err == nil {
		t.Fatal("bad signature accepted")
	}
}
