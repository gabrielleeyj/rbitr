package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"
)

type response struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

func main() {
	http.HandleFunc("/refund", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, response{Status: "ok", Message: "refund processed"})
	})

	http.HandleFunc("/export_customer_data", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, response{Status: "ok", Message: "export ready"})
	})

	http.HandleFunc("/change_role", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, response{Status: "ok", Message: "role change queued"})
	})

	const (
		readHeaderTimeout = 5 * time.Second
		readTimeout       = 10 * time.Second
		writeTimeout      = 10 * time.Second
		idleTimeout       = 30 * time.Second
	)

	server := &http.Server{
		Addr:              ":8090",
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	log.Println("mock tool listening on :8090")
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("mock tool failed: %v", err)
	}
}

func respondJSON(w http.ResponseWriter, status int, payload response) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
