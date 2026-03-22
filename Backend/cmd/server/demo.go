package main

import (
	_ "embed"
	"net/http"
)

//go:embed demo.html
var demoHTML []byte

// demoHandler serves the self-contained demo e-commerce store.
// The page has the BehaviourLens tracker pre-installed — every user
// interaction fires real events to the backend, which the dashboard
// visualises in real time.
func demoHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(demoHTML)
}
