package handler

import (
	"encoding/json"
	"net/http"
)

var serverVersion = "dev"

func SetServerVersion(v string) {
	serverVersion = v
}

func Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"version": serverVersion,
	})
}
