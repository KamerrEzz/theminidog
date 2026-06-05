package storage_test

import (
	"testing"
	"time"

	"github.com/kamerrezz/theminidog/internal/server/storage"
)

func TestQueryParams_Validate(t *testing.T) {
	base := storage.QueryParams{
		Host:   "host1",
		Name:   "cpu.usage_pct",
		From:   time.Now().Add(-time.Hour),
		To:     time.Now(),
		Bucket: "1m",
		Agg:    "avg",
	}
	t.Run("valid", func(t *testing.T) {
		if err := base.Validate(); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
	})
	t.Run("empty host", func(t *testing.T) {
		p := base
		p.Host = ""
		if err := p.Validate(); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty name", func(t *testing.T) {
		p := base
		p.Name = ""
		if err := p.Validate(); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("from equals to", func(t *testing.T) {
		p := base
		p.From = p.To
		if err := p.Validate(); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("from after to", func(t *testing.T) {
		p := base
		p.From = p.To.Add(time.Second)
		if err := p.Validate(); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("range over 30 days", func(t *testing.T) {
		p := base
		p.From = time.Now().Add(-31 * 24 * time.Hour)
		p.To = time.Now()
		if err := p.Validate(); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("invalid bucket", func(t *testing.T) {
		p := base
		p.Bucket = "2m"
		if err := p.Validate(); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("invalid agg", func(t *testing.T) {
		p := base
		p.Agg = "sum"
		if err := p.Validate(); err == nil {
			t.Fatal("expected error")
		}
	})
	// All valid bucket values
	for _, b := range []string{"1m", "5m", "15m", "1h", "1d"} {
		b := b
		t.Run("bucket_"+b, func(t *testing.T) {
			p := base
			p.Bucket = b
			if err := p.Validate(); err != nil {
				t.Fatalf("bucket %s: %v", b, err)
			}
		})
	}
	// All valid agg values
	for _, a := range []string{"avg", "max", "min"} {
		a := a
		t.Run("agg_"+a, func(t *testing.T) {
			p := base
			p.Agg = a
			if err := p.Validate(); err != nil {
				t.Fatalf("agg %s: %v", a, err)
			}
		})
	}
}
