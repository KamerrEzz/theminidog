package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"
)

type metric struct {
	Time   time.Time         `json:"time"`
	Host   string            `json:"host"`
	Name   string            `json:"name"`
	Value  float64           `json:"value"`
	Labels map[string]string `json:"labels,omitempty"`
}

type metricBatch struct {
	Host    string   `json:"host"`
	Metrics []metric `json:"metrics"`
}

func main() {
	addr := os.Getenv("STUB_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slog.SetDefault(logger)

	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})

	mux.HandleFunc("POST /api/v1/metrics", func(w http.ResponseWriter, r *http.Request) {
		var batch metricBatch
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
			http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
			return
		}

		slog.Info("received metrics batch",
			"host", batch.Host,
			"count", len(batch.Metrics),
		)
		for _, m := range batch.Metrics {
			slog.Debug("metric",
				"name", m.Name,
				"value", fmt.Sprintf("%.4f", m.Value),
				"labels", m.Labels,
			)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		fmt.Fprintf(w, `{"ingested":%d}`, len(batch.Metrics))
	})

	slog.Info("stub server listening", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}
