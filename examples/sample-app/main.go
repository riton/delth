package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

func handler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)

	for _, k := range os.Environ() {
		fmt.Fprintf(w, "%s: %s\n", k, os.Getenv(k))
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func main() {

	mux := http.NewServeMux()
	mux.Handle("/health", http.HandlerFunc(healthHandler))
	mux.Handle("/", http.HandlerFunc(handler))

	if err := http.ListenAndServe(":80", mux); err != nil {
		log.Fatal(err)
	}
}
