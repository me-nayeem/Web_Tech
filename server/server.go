package main

import (
	"encoding/json"
	"net/http"
	"log"
)

func healthHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}


func main() {
	http.HandleFunc("/health", healthHandler)
  log.Fatal(http.ListenAndServe(":8080", nil))
}


//assaignment: what is the differnce between two HandleFunc