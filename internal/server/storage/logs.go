package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kamerrezz/theminidog/internal/model"
)

var validLogLevels = map[string]struct{}{
	"info": {}, "warn": {}, "error": {}, "debug": {},
}

// LogQueryParams defines filters and pagination for log queries.
type LogQueryParams struct {
	Host   string
	Level  string
	From   time.Time
	To     time.Time
	Q      string // ILIKE substring
	Limit  int    // default 100, max 1000
	Cursor int64  // 0 = first page; keyset: WHERE id < Cursor
}

// Validate applies defaults and validates constraints.
func (p *LogQueryParams) Validate() error {
	if p.Limit <= 0 {
		p.Limit = 100
	}
	if p.Limit > 1000 {
		p.Limit = 1000
	}
	if p.Level != "" {
		if _, ok := validLogLevels[p.Level]; !ok {
			return fmt.Errorf("invalid level %q: must be one of info, warn, error, debug", p.Level)
		}
	}
	return nil
}

// LogQueryResult is one row returned from the logs query.
type LogQueryResult struct {
	ID      int64     `json:"id"`
	Time    time.Time `json:"time"`
	Host    string    `json:"host"`
	Path    string    `json:"path"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
}

// LogRepository defines storage for logs.
type LogRepository interface {
	Insert(ctx context.Context, batch model.LogBatch) (int, error)
	Query(ctx context.Context, params LogQueryParams) ([]LogQueryResult, int64, error)
	Ping(ctx context.Context) error
}

type pgLogRepository struct {
	pool *pgxpool.Pool
}

// NewLogRepository creates a LogRepository backed by a pgxpool.
func NewLogRepository(pool *pgxpool.Pool) LogRepository {
	return &pgLogRepository{pool: pool}
}

const insertLogSQL = `INSERT INTO logs (time, host, path, level, message) VALUES ($1,$2,$3,$4,$5)`

func (r *pgLogRepository) Insert(ctx context.Context, batch model.LogBatch) (int, error) {
	b := &pgx.Batch{}
	for _, e := range batch.Entries {
		b.Queue(insertLogSQL, e.Time, e.Host, e.Path, string(e.Level), e.Message)
	}
	br := r.pool.SendBatch(ctx, b)
	defer br.Close() // CRITICAL: releases pool connection
	for i := 0; i < b.Len(); i++ {
		if _, err := br.Exec(); err != nil {
			return i, fmt.Errorf("insert log[%d]: %w", i, err)
		}
	}
	return b.Len(), nil
}

func (r *pgLogRepository) Query(ctx context.Context, params LogQueryParams) ([]LogQueryResult, int64, error) {
	// Dynamic WHERE builder — only Go structure is built; all user values are bound params.
	var conds []string
	var args []any
	n := 0
	add := func(cond string, val any) {
		n++
		conds = append(conds, fmt.Sprintf(cond, n))
		args = append(args, val)
	}

	if params.Host != "" {
		add("host = $%d", params.Host)
	}
	if params.Level != "" {
		add("level = $%d", params.Level)
	}
	if !params.From.IsZero() {
		add("time >= $%d", params.From)
	}
	if !params.To.IsZero() {
		add("time <= $%d", params.To)
	}
	if params.Q != "" {
		add("message ILIKE $%d", "%"+params.Q+"%") // value bound, never interpolated
	}
	if params.Cursor > 0 {
		add("id < $%d", params.Cursor) // keyset pagination
	}

	n++
	limitArg := n
	args = append(args, params.Limit+1) // fetch LIMIT+1 to detect next page

	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}

	q := fmt.Sprintf(`SELECT id, time, host, path, level, message FROM logs %s ORDER BY id DESC LIMIT $%d`, where, limitArg)

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query logs: %w", err)
	}
	defer rows.Close()

	var results []LogQueryResult
	for rows.Next() {
		var row LogQueryResult
		if err := rows.Scan(&row.ID, &row.Time, &row.Host, &row.Path, &row.Level, &row.Message); err != nil {
			return nil, 0, fmt.Errorf("scan log row: %w", err)
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate log rows: %w", err)
	}

	// Keyset pagination: if we got LIMIT+1 results, there's a next page.
	var nextCursor int64
	if len(results) > params.Limit {
		results = results[:params.Limit]        // drop probe row
		nextCursor = results[params.Limit-1].ID // cursor = id of LAST returned row
	}
	return results, nextCursor, nil
}

func (r *pgLogRepository) Ping(ctx context.Context) error {
	_, err := r.pool.Exec(ctx, "SELECT 1")
	return err
}
