package auth

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
)

// Passkeys use the browser's getPublicKey()/getAuthenticatorData()
// accessors, which hand over a DER SubjectPublicKeyInfo and raw
// authenticator data, so no CBOR decoding is needed. Attestation is
// "none": we trust the key, not the make of authenticator.

type clientData struct {
	Type      string `json:"type"`
	Challenge string `json:"challenge"`
	Origin    string `json:"origin"`
}

func b64(s string) ([]byte, error) {
	if b, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.StdEncoding.DecodeString(s)
}

func checkClientData(raw []byte, typ, challenge string, origins []string) error {
	var cd clientData
	if err := json.Unmarshal(raw, &cd); err != nil {
		return fmt.Errorf("client data: %w", err)
	}
	if cd.Type != typ {
		return fmt.Errorf("client data type %q, want %q", cd.Type, typ)
	}
	if cd.Challenge != challenge {
		return errors.New("challenge mismatch")
	}
	for _, o := range origins {
		if cd.Origin == o {
			return nil
		}
	}
	return fmt.Errorf("origin %q is not allowed", cd.Origin)
}

func checkAuthData(ad []byte, rpID string) (uint32, error) {
	if len(ad) < 37 {
		return 0, errors.New("authenticator data too short")
	}
	h := sha256.Sum256([]byte(rpID))
	if !bytes.Equal(ad[:32], h[:]) {
		return 0, errors.New("relying party id mismatch")
	}
	if ad[32]&0x01 == 0 {
		return 0, errors.New("user presence flag not set")
	}
	return binary.BigEndian.Uint32(ad[33:37]), nil
}

type Registration struct {
	ID                string `json:"id"`
	ClientDataJSON    string `json:"clientDataJSON"`
	AuthenticatorData string `json:"authenticatorData"`
	PublicKey         string `json:"publicKey"` // SPKI DER, base64url
}

// VerifyRegistration returns the credential's public key and counter.
func VerifyRegistration(r Registration, challenge, rpID string, origins []string) ([]byte, uint32, error) {
	cd, err := b64(r.ClientDataJSON)
	if err != nil {
		return nil, 0, err
	}
	if err := checkClientData(cd, "webauthn.create", challenge, origins); err != nil {
		return nil, 0, err
	}
	ad, err := b64(r.AuthenticatorData)
	if err != nil {
		return nil, 0, err
	}
	count, err := checkAuthData(ad, rpID)
	if err != nil {
		return nil, 0, err
	}
	pk, err := b64(r.PublicKey)
	if err != nil {
		return nil, 0, err
	}
	if _, err := x509.ParsePKIXPublicKey(pk); err != nil {
		return nil, 0, fmt.Errorf("public key: %w", err)
	}
	return pk, count, nil
}

type Assertion struct {
	ID                string `json:"id"`
	ClientDataJSON    string `json:"clientDataJSON"`
	AuthenticatorData string `json:"authenticatorData"`
	Signature         string `json:"signature"`
}

// VerifyAssertion checks a login signature and returns the new counter.
func VerifyAssertion(a Assertion, spki []byte, prevCount uint32, challenge, rpID string, origins []string) (uint32, error) {
	cd, err := b64(a.ClientDataJSON)
	if err != nil {
		return 0, err
	}
	if err := checkClientData(cd, "webauthn.get", challenge, origins); err != nil {
		return 0, err
	}
	ad, err := b64(a.AuthenticatorData)
	if err != nil {
		return 0, err
	}
	count, err := checkAuthData(ad, rpID)
	if err != nil {
		return 0, err
	}
	if count != 0 && count <= prevCount {
		return 0, errors.New("signature counter did not increase; the credential may be cloned")
	}
	sig, err := b64(a.Signature)
	if err != nil {
		return 0, err
	}
	cdHash := sha256.Sum256(cd)
	signed := append(append([]byte{}, ad...), cdHash[:]...)
	pub, err := x509.ParsePKIXPublicKey(spki)
	if err != nil {
		return 0, err
	}
	digest := sha256.Sum256(signed)
	switch k := pub.(type) {
	case *ecdsa.PublicKey:
		if !ecdsa.VerifyASN1(k, digest[:], sig) {
			return 0, errors.New("bad signature")
		}
	case *rsa.PublicKey:
		if err := rsa.VerifyPKCS1v15(k, crypto.SHA256, digest[:], sig); err != nil {
			return 0, errors.New("bad signature")
		}
	case ed25519.PublicKey:
		if !ed25519.Verify(k, signed, sig) {
			return 0, errors.New("bad signature")
		}
	default:
		return 0, errors.New("unsupported key type")
	}
	return count, nil
}
