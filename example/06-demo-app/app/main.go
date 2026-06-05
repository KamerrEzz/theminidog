package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Task represents a to-do item.
type Task struct {
	ID        int    `json:"id"`
	Title     string `json:"title"`
	CreatedAt string `json:"created_at"`
}

// store is the in-memory task repository.
type store struct {
	mu      sync.RWMutex
	tasks   map[int]Task
	counter int
}

func newStore() *store {
	return &store{tasks: make(map[int]Task)}
}

func (s *store) create(title string) Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counter++
	t := Task{ID: s.counter, Title: title, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	s.tasks[s.counter] = t
	return t
}

func (s *store) list() []Task {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		out = append(out, t)
	}
	return out
}

func (s *store) get(id int) (Task, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tasks[id]
	return t, ok
}

func (s *store) delete(id int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.tasks[id]
	if ok {
		delete(s.tasks, id)
	}
	return ok
}

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// writeJSON encodes v as JSON and writes it with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes a plain JSON error message.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// fib computes the nth Fibonacci number (non-memoized, intentionally slow).
func fib(n int) int {
	if n <= 1 {
		return n
	}
	return fib(n-1) + fib(n-2)
}

func main() {
	// Set up log output: both stdout and a file.
	logDir := "/var/log/demoapp"
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create log dir: %v\n", err)
		os.Exit(1)
	}
	logFile, err := os.OpenFile(logDir+"/app.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open log file: %v\n", err)
		os.Exit(1)
	}
	defer logFile.Close()

	multi := io.MultiWriter(os.Stdout, logFile)
	logger := slog.New(slog.NewJSONHandler(multi, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slog.SetDefault(logger)

	db := newStore()
	mux := http.NewServeMux()

	// Request logging middleware.
	logged := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
			next(rw, r)
			durMs := time.Since(start).Milliseconds()
			logger.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rw.status,
				"duration_ms", durMs,
			)
		}
	}

	// GET /healthz
	mux.HandleFunc("GET /healthz", logged(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}))

	// POST /tasks
	mux.HandleFunc("POST /tasks", logged(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Title string `json:"title"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Title) == "" {
			writeError(w, http.StatusBadRequest, "title is required")
			return
		}
		task := db.create(strings.TrimSpace(body.Title))
		writeJSON(w, http.StatusCreated, task)
	}))

	// GET /tasks — list all tasks
	mux.HandleFunc("GET /tasks", logged(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, db.list())
	}))

	// GET /tasks/{id} — get one task
	mux.HandleFunc("GET /tasks/{id}", logged(func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid task id")
			return
		}
		task, ok := db.get(id)
		if !ok {
			writeError(w, http.StatusNotFound, "task not found")
			return
		}
		writeJSON(w, http.StatusOK, task)
	}))

	// DELETE /tasks/{id}
	mux.HandleFunc("DELETE /tasks/{id}", logged(func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid task id")
			return
		}
		if !db.delete(id) {
			writeError(w, http.StatusNotFound, "task not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	// GET /slow — intentionally slow endpoint
	mux.HandleFunc("GET /slow", logged(func(w http.ResponseWriter, r *http.Request) {
		// Sleep between 100ms and 500ms to generate interesting latency metrics.
		delay := time.Duration(100+rand.Intn(400)) * time.Millisecond
		time.Sleep(delay)
		writeJSON(w, http.StatusOK, map[string]any{
			"message":   "slow response",
			"delay_ms":  delay.Milliseconds(),
		})
	}))

	// GET /cpu — fibonacci to push CPU usage up
	mux.HandleFunc("GET /cpu", logged(func(w http.ResponseWriter, r *http.Request) {
		result := fib(35)
		writeJSON(w, http.StatusOK, map[string]any{
			"message": "cpu work done",
			"fib_35":  result,
		})
	}))

	addr := ":9000"
	logger.Info("demo app started on " + addr)
	srv := &http.Server{Addr: addr, Handler: mux}
	if err := srv.ListenAndServe(); err != nil {
		logger.Error("server stopped", "err", err)
		os.Exit(1)
	}
}
