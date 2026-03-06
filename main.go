package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/codetesla51/limitz/algorithms"
	"github.com/codetesla51/limitz/store"
	"github.com/codetesla51/logos/logos"
)

var mu sync.Mutex
type codeBody struct {
	Source string `json:"source"`
}

type runResponse struct {
	Output string `json:"output"`
	Error  string `json:"error,omitempty"`
}

func main() {
	s := store.NewMemoryStore()
	defer s.Close()
	limiter := algorithms.NewTokenBucket(50, 10, s)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /run", runHandler)

	srv := &http.Server{
		Addr:         ":8080",
		Handler:      logMiddleware(mux, limiter),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	log.Println("Server starting on :8080")

	log.Fatal(srv.ListenAndServe())

}

func logMiddleware(next http.Handler, limiter *algorithms.TokenBucket) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "https://logos-lang.vercel.app")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		// handle preflight
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		log.Printf("%s %s", r.Method, r.URL.Path)
		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		limited, err := limiter.Allow(ip)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if !limited.Allowed {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func runHandler(w http.ResponseWriter, r *http.Request) {
	var body codeBody
	r.Body = http.MaxBytesReader(w, r.Body, 1024*10) // limit to 10KB
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	
	mu.Lock()
	defer mu.Unlock()
	// capture stdout
	old := os.Stdout
	pr, pw, _ := os.Pipe()
	os.Stdout = pw

	vm := logos.NewWithConfig(logos.SandboxConfig{
		AllowFileIO:  false,
		AllowNetwork: false,
		AllowShell:   false,
		AllowExit:    false,
	})

	// run with timeout
	type result struct {
		err error
	}
	done := make(chan result, 1)

	go func() {

		done <- result{err: vm.Run(body.Source)}
	}()

	resp := runResponse{}

	select {
	case res := <-done:
		pw.Close()
		var buf bytes.Buffer
		buf.ReadFrom(pr)
		os.Stdout = old
		resp.Output = buf.String()
		if res.err != nil {
			resp.Error = res.err.Error()
		}
	case <-time.After(5 * time.Second):
		pw.Close()
		os.Stdout = old
		resp.Error = "execution timed out"
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
