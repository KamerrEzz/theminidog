package api_test

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/kamerrezz/theminidog/internal/server/api"
)

const testSecret = "test-secret-min-16ch"

func makeHS256Token(secret []byte, exp time.Time) string {
	claims := jwt.RegisteredClaims{}
	if !exp.IsZero() {
		claims.ExpiresAt = jwt.NewNumericDate(exp)
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(secret)
	if err != nil {
		panic("makeHS256Token: " + err.Error())
	}
	return signed
}

func stubHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func TestJWTMiddleware(t *testing.T) {
	secret := []byte(testSecret)
	mw := api.JWTMiddleware(secret)
	handler := mw(http.HandlerFunc(stubHandler))

	t.Run("valid HS256 JWT passes", func(t *testing.T) {
		token := makeHS256Token(secret, time.Now().Add(time.Hour))
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
	})

	t.Run("missing Authorization header returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rr.Code)
		}
	})

	t.Run("Bearer with empty token returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer ")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rr.Code)
		}
	})

	t.Run("invalid signature returns 401", func(t *testing.T) {
		token := makeHS256Token([]byte("wrong-secret-16-chars!"), time.Now().Add(time.Hour))
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rr.Code)
		}
	})

	t.Run("expired JWT returns 401", func(t *testing.T) {
		token := makeHS256Token(secret, time.Now().Add(-time.Hour))
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rr.Code)
		}
	})

	t.Run("RS256 signed token returns 401 (alg substitution blocked)", func(t *testing.T) {
		privKey, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("generate RSA key: %v", err)
		}
		claims := jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		}
		tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		rsaToken, err := tok.SignedString(privKey)
		if err != nil {
			t.Fatalf("sign RS256 token: %v", err)
		}
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+rsaToken)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for RS256 token, got %d", rr.Code)
		}
	})
}
