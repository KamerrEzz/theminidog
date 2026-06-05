package api

import (
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// JWTMiddleware validates Bearer JWT tokens using HS256.
// It enforces jwt.WithValidMethods([]string{"HS256"}) to block alg=none and alg-substitution attacks.
func JWTMiddleware(secret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
				writeError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
			if tokenStr == "" {
				writeError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			_, err := jwt.ParseWithClaims(
				tokenStr,
				&jwt.RegisteredClaims{},
				func(t *jwt.Token) (any, error) { return secret, nil },
				jwt.WithValidMethods([]string{"HS256"}), // MANDATORY — blocks alg=none and RS256
			)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
