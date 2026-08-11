package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

type contextKey string

const (
	requestIDKey contextKey = "mindova-request-id"
	requestIDLen            = 16
)

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		requestID := r.Header.Get("X-Request-ID")

		if requestID == "" {
			requestID = genarateRequestID()
		}

		ctx := context.WithValue(r.Context(), requestIDKey, requestID)
		r = r.WithContext(ctx)

		w.Header().Set("X-Request-ID", requestID)

		next.ServeHTTP(w, r)
	})
}

func GetRequestID(ctx context.Context) string {
	id, ok := ctx.Value(requestIDKey).(string)

	if !ok {
		return ""
	}

	return id
}

func genarateRequestID() string {
	b := make([]byte, requestIDLen)

	_, err := rand.Read(b)
	if err != nil {
		return "error-" + hex.EncodeToString([]byte{0})
	}

	return hex.EncodeToString(b)
}
