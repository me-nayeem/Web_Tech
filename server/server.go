package main

import (
	"crypto/rand"
	"encoding/json"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
)

const codeLength = 6
const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

var (
	mu   sync.RWMutex
	urls = map[string]string{}
)

func randomCode() string {
	b := make([]byte, codeLength)
	max := big.NewInt(int64(len(charset)))
	for i := range b {
		n, _ := rand.Int(rand.Reader, max)
		b[i] = charset[n.Int64()]
	}
	return string(b)
}

func isValidURL(raw string) bool {
	u, err := url.ParseRequestURI(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https")
}

func handleShorten(baseURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "use POST", http.StatusMethodNotAllowed)
			return
		}

		var req struct{ URL string `json:"url"` }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}

		req.URL = strings.TrimSpace(req.URL)
		if req.URL == "" || !isValidURL(req.URL) {
			http.Error(w, "url must be a valid absolute URL", http.StatusBadRequest)
			return
		}

		mu.Lock()
		var code string
		for i := 0; i < 5; i++ {
			code = randomCode()
			if _, exists := urls[code]; !exists {
				break
			}
		}
		urls[code] = req.URL
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{
			"short_url": baseURL + "/" + code,
			"code":      code,
		})
	}
}

func handleRedirect(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimPrefix(r.URL.Path, "/")
	mu.RLock()
	longURL, ok := urls[code]
	mu.RUnlock()

	if code == "" || !ok {
		http.Error(w, "short link not found", http.StatusNotFound)
		return
	}
	http.Redirect(w, r, longURL, http.StatusFound)
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:" + port
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/shorten", handleShorten(baseURL))
	mux.HandleFunc("/", handleRedirect)

	log.Printf("listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}