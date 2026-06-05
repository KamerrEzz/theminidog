package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kamerrezz/theminidog/internal/model"
)

// validBuckets maps user-facing bucket strings to safe SQL interval literals.
var validBuckets = map[string]string{
	"1m":  "1 minute",
	"5m":  "5 minutes",
	"15m": "15 minutes",
	"1h":  "1 hour",
	"1d":  "1 day",
}

// validAggs maps user-facing agg strings to safe SQL function names.
var validAggs = map[string]string{
	"avg": "avg",
	"max": "max",
	"min": "min",
}

// validMetricNames mirrors the canonical metric names from internal/model.
// Both sets must stay in sync when new metrics are added.
var validMetricNames = map[string]struct{}{
	"cpu.usage_pct":    {},
	"mem.used_pct":     {},
	"mem.used_bytes":   {},
	"mem.total_bytes":  {},
	"disk.used_pct":    {},
	"disk.used_bytes":  {},
	"disk.total_bytes": {},
	"net.bytes_in":     {},
	"net.bytes_out":    {},
}

// QueryParams defines the parameters for a metric time-bucket query.
type QueryParams struct {
	Host   string
	Name   string
	From   time.Time
	To     time.Time
	Bucket string // "1m","5m","15m","1h","1d"
	Agg    string // "avg","max","min"
}

// Validate checks all QueryParams fields.
func (p QueryParams) Validate() error {
	if strings.TrimSpace(p.Host) == "" {
		return fmt.Errorf("host is required")
	}
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if _, ok := validMetricNames[p.Name]; !ok {
		return fmt.Errorf("unknown metric name %q", p.Name)
	}
	if _, ok := validBuckets[p.Bucket]; !ok {
		return fmt.Errorf("invalid bucket %q: must be one of 1m,5m,15m,1h,1d", p.Bucket)
	}
	if _, ok := validAggs[p.Agg]; !ok {
		return fmt.Errorf("invalid agg %q: must be one of avg,max,min", p.Agg)
	}
	if !p.From.Before(p.To) {
		return fmt.Errorf("from must be before to")
	}
	if p.To.Sub(p.From) > 30*24*time.Hour {
		return fmt.Errorf("time range must not exceed 30 days")
	}
	return nil
}

// QueryPoint is a single time-bucketed value.
type QueryPoint struct {
	Time  time.Time `json:"time"`
	Value float64   `json:"value"`
}

// MetricRepository defines storage operations for metrics.
type MetricRepository interface {
	Insert(ctx context.Context, batch model.MetricBatch) (int, error)
	Query(ctx context.Context, params QueryParams) ([]QueryPoint, error)
	Ping(ctx context.Context) error
}

type pgxMetricRepository struct {
	pool *pgxpool.Pool
}

// NewMetricRepository creates a MetricRepository backed by a pgxpool.
func NewMetricRepository(pool *pgxpool.Pool) MetricRepository {
	return &pgxMetricRepository{pool: pool}
}

func (r *pgxMetricRepository) Insert(ctx context.Context, batch model.MetricBatch) (int, error) {
	b := &pgx.Batch{}
	const q = `INSERT INTO metrics (time, host, name, value, labels) VALUES ($1,$2,$3,$4,$5)`
	for _, m := range batch.Metrics {
		var labels []byte
		if len(m.Labels) > 0 {
			var err error
			labels, err = json.Marshal(m.Labels)
			if err != nil {
				return 0, fmt.Errorf("marshal labels: %w", err)
			}
		}
		b.Queue(q, m.Time, m.Host, m.Name, m.Value, labels)
	}
	br := r.pool.SendBatch(ctx, b)
	defer br.Close() // CRITICAL: must close to release pool connection
	for i := 0; i < b.Len(); i++ {
		if _, err := br.Exec(); err != nil {
			return i, fmt.Errorf("insert metric[%d]: %w", i, err)
		}
	}
	return b.Len(), nil
}

func (r *pgxMetricRepository) Query(ctx context.Context, params QueryParams) ([]QueryPoint, error) {
	bucketLiteral := validBuckets[params.Bucket] // safe: validated before use
	aggFn := validAggs[params.Agg]               // safe: validated before use

	// Option B: allowlist interpolation — avoids $1::interval prepared-statement cache issue.
	// bucket and aggFn come exclusively from the validBuckets/validAggs maps, never raw user input.
	q := fmt.Sprintf(`
        SELECT time_bucket('%s', time) AS bucket,
               %s(value) AS value
        FROM metrics
        WHERE host = $1
          AND name = $2
          AND time >= $3
          AND time <= $4
        GROUP BY bucket
        ORDER BY bucket DESC`,
		bucketLiteral, aggFn,
	)

	rows, err := r.pool.Query(ctx, q, params.Host, params.Name, params.From, params.To)
	if err != nil {
		return nil, fmt.Errorf("query metrics: %w", err)
	}
	defer rows.Close()

	var points []QueryPoint
	for rows.Next() {
		var p QueryPoint
		if err := rows.Scan(&p.Time, &p.Value); err != nil {
			return nil, fmt.Errorf("scan metric row: %w", err)
		}
		points = append(points, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate metric rows: %w", err)
	}
	return points, nil
}

func (r *pgxMetricRepository) Ping(ctx context.Context) error {
	_, err := r.pool.Exec(ctx, "SELECT 1")
	return err
}
