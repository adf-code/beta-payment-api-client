package repository

import (
	"encoding/json"
	"net/http"
)

func redactHeaders(h http.Header) http.Header {
	cloned := make(http.Header, len(h))
	for k, v := range h {
		// copy slice
		cp := append([]string(nil), v...)
		// redact Authorization
		if http.CanonicalHeaderKey(k) == "Authorization" {
			for i := range cp {
				// Bentuk umum: "Bearer xxxxx"
				cp[i] = "Bearer ***redacted***"
			}
		}
		cloned[k] = cp
	}
	return cloned
}

func marshalHeaders(h http.Header) ([]byte, error) {
	return json.Marshal(h)
}
