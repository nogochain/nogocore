// Copyright (c) 2026 NogoChain Contributors
// Use of this source code is governed by an ISC license.

// Package api provides the HTTP REST API server for NogoCore.
// It serves the block explorer web application and JSON API endpoints.
package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nogochain/nogocore/blockchain"
	"github.com/nogochain/nogocore/config"
	"github.com/nogochain/nogocore/explorer"
	"github.com/nogochain/nogocore/mempool"
)

// Server wraps the HTTP REST API server for NogoCore.
type Server struct {
	httpServer *http.Server
	mux        *http.ServeMux
	chain      *blockchain.BlockChain
	txPool     mempool.TxMempool
	cfg        *config.Config
	webDir     string
}

// NewServer creates a new REST API server.
// webDir should be the path to the web/explorer/ static files directory.
func NewServer(cfg *config.Config, chain *blockchain.BlockChain, txPool mempool.TxMempool) *Server {
	mux := http.NewServeMux()

	// Determine web directory relative to executable.
	webDir := cfg.DataDir
	if wd := os.Getenv("NOGOCORE_WEB_DIR"); wd != "" {
		webDir = wd
	}

	s := &Server{
		mux:    mux,
		chain:  chain,
		txPool: txPool,
		cfg:    cfg,
		webDir: webDir,
	}

	// Register middleware.
	var handler http.Handler = mux
	handler = corsMiddleware(handler)
	handler = loggingMiddleware(handler)

	// Choose listen address.
	addr := "127.0.0.1:8080"
	if len(cfg.RPCListeners) > 0 {
		// Use a separate port for REST (RPC port + 1000).
		rpcAddr := cfg.RPCListeners[0]
		parts := strings.Split(rpcAddr, ":")
		if len(parts) == 2 {
			addr = parts[0] + ":8080"
		}
	}

	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Register API routes (before static file handler).
	s.registerRoutes()

	return s
}

// registerRoutes sets up all HTTP routes.
func (s *Server) registerRoutes() {
	// Explorer JSON API endpoints.
	explorerAPI := explorer.NewAPI(s.chain, s.txPool)
	explorerAPI.RegisterRoutes(s.mux)

	// Health check.
	s.mux.HandleFunc("/health", s.handleHealth)

	// Static file serving for the block explorer web application.
	// Serves from the embedded web/explorer/ directory or a custom path.
	s.registerStaticFiles()
}

// registerStaticFiles serves the block explorer static web application.
func (s *Server) registerStaticFiles() {
	// Try multiple locations for the web directory.
	candidates := []string{
		"web/explorer",
		filepath.Join("..", "web", "explorer"),
		filepath.Join(s.webDir, "web", "explorer"),
		filepath.Join(os.Getenv("NOGOCORE_HOME"), "web", "explorer"),
	}

	var staticDir string
	for _, dir := range candidates {
		if dir == "" {
			continue
		}
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			staticDir = dir
			break
		}
	}

	if staticDir != "" {
		fs := http.FileServer(http.Dir(staticDir))
		// Serve static files at root path, but NOT intercept API routes.
		s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			// Let API routes take precedence.
			if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/health" {
				http.NotFound(w, r)
				return
			}
			// Serve static files; fallback to index.html for SPA routing.
			filePath := filepath.Join(staticDir, r.URL.Path)
			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				// SPA fallback: serve index.html for client-side routing.
				http.ServeFile(w, r, filepath.Join(staticDir, "index.html"))
				return
			}
			fs.ServeHTTP(w, r)
		})
		log.Printf("[API] Serving static files from: %s", staticDir)
	} else {
		log.Printf("[API] No static web directory found. Explorer UI not available.")
	}
}

// handleHealth returns a simple health check response.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	best := s.chain.BestSnapshot()
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"ok","block_height":%d,"timestamp":%d}`,
		best.Height, time.Now().Unix())
}

// Start begins listening and serving HTTP requests.
func (s *Server) Start() error {
	log.Printf("[API] REST server listening on %s", s.httpServer.Addr)
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully stops the server with a timeout.
func (s *Server) Shutdown(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	log.Printf("[API] Shutting down REST server...")
	return s.httpServer.Shutdown(ctx)
}

// Addr returns the listen address of the server.
func (s *Server) Addr() string {
	return s.httpServer.Addr
}

// ---- Middleware ----

// corsMiddleware adds CORS headers to allow browser access.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// loggingMiddleware logs each HTTP request with method, path, status, and duration.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := newResponseWriter(w)
		next.ServeHTTP(rw, r)
		duration := time.Since(start)
		log.Printf("[API] %s %s %d %s", r.Method, r.URL.Path, rw.statusCode, duration.Round(time.Microsecond))
	})
}
