package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	testSecret = "ajksfhdjheihajkfjksfjksfhjksfjshfht"
	testSub    = "123e4567-e89b-12d3-a456-426614174000"
)

func claims(sub, iss, aud string, exp time.Time) jwt.MapClaims {
	return jwt.MapClaims{"sub": sub, "iss": iss, "aud": aud, "exp": exp.Unix(), "email": "a@b.com", "role": "authenticated"}
}

func signHS(t *testing.T, secret string, c jwt.MapClaims) string {
	t.Helper()
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestHS256(t *testing.T) {
	const base = "https://proj.supabase.co"
	v := NewVerifier("", testSecret, nil) 
	future := time.Now().Add(time.Hour)

	cl, err := v.Verify(context.Background(), signHS(t, testSecret, claims(testSub, "x", "authenticated", future)))
	if err != nil || cl.Subject != testSub || cl.Email != "a@b.com" {
		t.Fatalf("valid token rejected: %v %+v", err, cl)
	}

	bad := map[string]string{
		"wrong secret":   signHS(t, "another-secret-another-secret-xx", claims(testSub, "x", "authenticated", future)),
		"expired":        signHS(t, testSecret, claims(testSub, "x", "authenticated", time.Now().Add(-time.Hour))),
		"wrong audience": signHS(t, testSecret, claims(testSub, "x", "other", future)),
		"subject not id": signHS(t, testSecret, claims("not-a-uuid", "x", "authenticated", future)),
		"garbage":        "abc.def.ghi",
	}
	none, _ := jwt.NewWithClaims(jwt.SigningMethodNone, claims(testSub, "x", "authenticated", future)).SignedString(jwt.UnsafeAllowNoneSignatureType)
	bad["alg none"] = none
	for name, tok := range bad {
		if _, err := v.Verify(context.Background(), tok); err == nil {
			t.Errorf("%s: token was accepted", name)
		}
	}

	
	v2 := NewVerifier(base, testSecret, nil)
	if _, err := v2.Verify(context.Background(), signHS(t, testSecret, claims(testSub, base+"/auth/v1", "authenticated", future))); err != nil {
		t.Errorf("right issuer rejected: %v", err)
	}
	if _, err := v2.Verify(context.Background(), signHS(t, testSecret, claims(testSub, "https://evil.example/auth/v1", "authenticated", future))); err == nil {
		t.Error("wrong issuer accepted")
	}
}

func TestHS256RefusedWithoutSecret(t *testing.T) {
	v := NewVerifier("https://proj.supabase.co", "", nil)
	tok := signHS(t, "", claims(testSub, "https://proj.supabase.co/auth/v1", "authenticated", time.Now().Add(time.Hour)))
	if _, err := v.Verify(context.Background(), tok); err == nil {
		t.Fatal("HS256 token accepted although no secret is configured")
	}
}

func TestES256ViaJWKS(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	enc := base64.RawURLEncoding.EncodeToString
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Path != "/auth/v1/.well-known/jwks.json" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "EC", "crv": "P-256", "kid": "k1", "alg": "ES256",
			"x": enc(key.X.FillBytes(make([]byte, 32))), "y": enc(key.Y.FillBytes(make([]byte, 32))),
		}}})
	}))
	defer srv.Close()

	v := NewVerifier(srv.URL, "", srv.Client())
	mk := func(kid string, k *ecdsa.PrivateKey) string {
		tok := jwt.NewWithClaims(jwt.SigningMethodES256, claims(testSub, srv.URL+"/auth/v1", "authenticated", time.Now().Add(time.Hour)))
		tok.Header["kid"] = kid
		s, err := tok.SignedString(k)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}

	if _, err := v.Verify(context.Background(), mk("k1", key)); err != nil {
		t.Fatalf("valid ES256 token rejected: %v", err)
	}
	if _, err := v.Verify(context.Background(), mk("k1", key)); err != nil || hits != 1 {
		t.Fatalf("keys should be cached (hits=%d, err=%v)", hits, err)
	}
	other, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if _, err := v.Verify(context.Background(), mk("k1", other)); err == nil {
		t.Error("token signed with a different key was accepted")
	}
	if _, err := v.Verify(context.Background(), mk("unknown", key)); err == nil {
		t.Error("token with unknown key id was accepted")
	}
}
