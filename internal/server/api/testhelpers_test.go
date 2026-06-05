package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/kamerrezz/theminidog/internal/model"
	"github.com/kamerrezz/theminidog/internal/server/storage"
)

// fakeRepo is a test double for storage.MetricRepository.
type fakeRepo struct {
	pingErr    error
	insertN    int
	insertErr  error
	queryPts   []storage.QueryPoint
	queryErr   error
}

func (f *fakeRepo) Ping(_ context.Context) error {
	return f.pingErr
}

func (f *fakeRepo) Insert(_ context.Context, _ model.MetricBatch) (int, error) {
	return f.insertN, f.insertErr
}

func (f *fakeRepo) Query(_ context.Context, _ storage.QueryParams) ([]storage.QueryPoint, error) {
	return f.queryPts, f.queryErr
}

func (f *fakeRepo) Hosts(_ context.Context, _ time.Duration) ([]string, error) {
	return nil, nil
}

// helpers

func mustDecode(t *testing.T, body io.Reader, v any) {
	t.Helper()
	if err := json.NewDecoder(body).Decode(v); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
}

func errBody(t *testing.T, body io.Reader) string {
	t.Helper()
	var m map[string]string
	mustDecode(t, body, &m)
	return m["error"]
}

func newFakeRepo() *fakeRepo { return &fakeRepo{} }

// nowUTC returns a truncated RFC3339-friendly time for test use.
func nowUTC() time.Time { return time.Now().UTC().Truncate(time.Second) }

// makeFreshPingErr returns a non-nil error for readyz tests.
func makePingErr() error { return errors.New("db down") }

// assertStatus fails the test if the response code != want.
func assertStatus(t *testing.T, rr *http.Response, want int) {
	t.Helper()
	if rr.StatusCode != want {
		t.Fatalf("expected status %d, got %d", want, rr.StatusCode)
	}
}
