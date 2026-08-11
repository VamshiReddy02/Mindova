package middleware

import (
	"fmt"
	"net/http"

	"github.com/vamshireddy02/mindova/packages/kernel/logger"
)

func Recovery(log *logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer recoverPanic(log, w, r)
			next.ServeHTTP(w, r)
		})
	}
}

func recoverPanic(log *logger.Logger, w http.ResponseWriter, r *http.Request) {
	err := recover()
	if err == nil {
		return
	}

	log.Error("panic recovered",
		"request_id", GetRequestID(r.Context()),
		"method", r.Method,
		"path", r.URL.Path,
		"path", fmt.Sprintf("%v", err),
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte(`{"error":"internal server error"}`))
}
