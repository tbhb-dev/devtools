// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package ghapp

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestJWT(t *testing.T) {
	t.Parallel()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	now := time.Unix(1700000000, 0)
	for _, block := range []*pem.Block{{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}, {Type: "PRIVATE KEY", Bytes: pkcs8}} {
		encoded := pem.EncodeToMemory(block)
		for _, data := range []string{string(encoded), strings.ReplaceAll(string(encoded), "\n", " "), strings.ReplaceAll(string(encoded), "\n", "\r\n")} {
			normalized, normalizeErr := NormalizePEM([]byte(data))
			require.NoError(t, normalizeErr)
			require.Equal(t, string(encoded), string(normalized))
			token, jwtErr := JWT("1234", []byte(data), now)
			require.NoError(t, jwtErr)
			parts := strings.Split(token, ".")
			require.Len(t, parts, 3)
			header, decodeErr := base64.RawURLEncoding.DecodeString(parts[0])
			require.NoError(t, decodeErr)
			require.JSONEq(t, `{"alg":"RS256","typ":"JWT"}`, string(header))
			payload, decodeErr := base64.RawURLEncoding.DecodeString(parts[1])
			require.NoError(t, decodeErr)
			var claims struct {
				Issuer  string `json:"iss"`
				Issued  int64  `json:"iat"`
				Expires int64  `json:"exp"`
			}
			require.NoError(t, json.Unmarshal(payload, &claims))
			require.Equal(t, "1234", claims.Issuer)
			require.Equal(t, now.Add(-time.Minute).Unix(), claims.Issued)
			require.Greater(t, claims.Expires, now.Unix())
			require.Less(t, claims.Expires-claims.Issued, int64(600))
			signature, decodeErr := base64.RawURLEncoding.DecodeString(parts[2])
			require.NoError(t, decodeErr)
			digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
			require.NoError(t, rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, digest[:], signature))
		}
	}
	for _, invalid := range []string{"", string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("bad")})), string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: []byte("abc")}))} {
		_, err = JWT("id", []byte(invalid), now)
		require.Error(t, err)
	}
	ec, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(ec)
	require.NoError(t, err)
	_, err = JWT("id", pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), now)
	require.Error(t, err)
}
