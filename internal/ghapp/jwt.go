// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Package ghapp authenticates a GitHub App without external cryptographic tools.
package ghapp

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"strings"
	"time"
)

// NormalizePEM rebuilds Privacy Enhanced Mail armor and wraps base64 at 64 columns.
func NormalizePEM(data []byte) ([]byte, error) {
	text := strings.TrimSpace(string(data))
	for _, kind := range []string{"RSA PRIVATE KEY", "PRIVATE KEY"} {
		begin, end := "-----BEGIN "+kind+"-----", "-----END "+kind+"-----"
		if !strings.HasPrefix(text, begin) || !strings.HasSuffix(text, end) {
			continue
		}
		body := strings.Join(strings.Fields(strings.TrimSuffix(strings.TrimPrefix(text, begin), end)), "")
		var out strings.Builder
		out.WriteString(begin + "\n")
		const width = 64
		for len(body) > width {
			out.WriteString(body[:width] + "\n")
			body = body[width:]
		}
		out.WriteString(body + "\n" + end + "\n")
		return []byte(out.String()), nil
	}
	return nil, fmt.Errorf("%w: expected RSA PRIVATE KEY or PRIVATE KEY PEM", os.ErrInvalid)
}

// JWT signs a short-lived RS256 JSON Web Token, backdating issuance for clock skew.
func JWT(appID string, data []byte, now time.Time) (string, error) {
	normalized, err := NormalizePEM(data)
	if err != nil {
		return "", err
	}
	block, _ := pem.Decode(normalized)
	if block == nil {
		return "", fmt.Errorf("%w: malformed private key PEM", os.ErrInvalid)
	}
	key, err := parseKey(block)
	if err != nil {
		return "", err
	}
	const lifetime = 8 * time.Minute
	claims, err := json.Marshal(struct {
		Issuer  string `json:"iss"`
		Issued  int64  `json:"iat"`
		Expires int64  `json:"exp"`
	}{appID, now.Add(-time.Minute).Unix(), now.Add(lifetime).Unix()})
	if err != nil {
		return "", fmt.Errorf("encode JWT: %w", err)
	}
	encode := base64.RawURLEncoding.EncodeToString
	unsigned := encode([]byte(`{"alg":"RS256","typ":"JWT"}`)) + "." + encode(claims)
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign JWT: %w", err)
	}
	return unsigned + "." + encode(signature), nil
}

func parseKey(block *pem.Block) (*rsa.PrivateKey, error) {
	if block.Type == "RSA PRIVATE KEY" {
		key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse RSA key: %w", err)
		}
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("%w: private key must be RSA", os.ErrInvalid)
	}
	return key, nil
}
