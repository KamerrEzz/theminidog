package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/kamerrezz/theminidog/internal/server/storage"
)

// handlePrometheus serves the latest metric values in Prometheus text format.
// Public endpoint — no JWT required. Compatible with any Prometheus scrape_config.
func handlePrometheus(repo storage.MetricRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		now := time.Now().UTC()
		from := now.Add(-2 * time.Minute)

		// Discover active hosts within the last 2 minutes.
		hosts, err := repo.Hosts(ctx, 2*time.Minute)
		if err != nil || len(hosts) == 0 {
			w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
			return
		}

		metricNames := []string{
			"cpu.usage_pct",
			"mem.used_pct", "mem.used_bytes", "mem.total_bytes",
			"disk.used_pct", "disk.used_bytes", "disk.total_bytes",
			"net.bytes_in", "net.bytes_out",
		}

		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

		for _, name := range metricNames {
			promName := promMetricName(name)

			// Collect the latest point per host for this metric name.
			type hostPoint struct {
				host  string
				point storage.QueryPoint
			}
			var rows []hostPoint

			for _, host := range hosts {
				pts, err := repo.Query(ctx, storage.QueryParams{
					Host:   host,
					Name:   name,
					From:   from,
					To:     now,
					Bucket: "1m",
					Agg:    "avg",
				})
				if err != nil || len(pts) == 0 {
					continue
				}
				// Query returns points ordered DESC by bucket; index 0 is the most recent.
				rows = append(rows, hostPoint{host: host, point: pts[0]})
			}

			if len(rows) == 0 {
				continue
			}

			fmt.Fprintf(w, "# HELP %s MiniObserv metric: %s\n", promName, name)
			fmt.Fprintf(w, "# TYPE %s gauge\n", promName)

			for _, row := range rows {
				tsMs := row.point.Time.UnixMilli()
				fmt.Fprintf(w, "%s{host=%q} %g %d\n", promName, row.host, row.point.Value, tsMs)
			}

			fmt.Fprintln(w)
		}
	}
}

// promMetricName converts a MiniObserv metric name to a valid Prometheus metric
// name: dots and hyphens become underscores and the result is prefixed with
// "miniobserv_".
func promMetricName(name string) string {
	result := make([]byte, len(name))
	for i := range name {
		if name[i] == '.' || name[i] == '-' {
			result[i] = '_'
		} else {
			result[i] = name[i]
		}
	}
	return "miniobserv_" + string(result)
}
