package storage_test

import (
	"testing"

	"github.com/kamerrezz/theminidog/internal/server/storage"
)

func TestLogQueryParams_Validate(t *testing.T) {
	tests := []struct {
		name      string
		input     storage.LogQueryParams
		wantErr   bool
		wantLimit int
	}{
		{
			name:      "zero limit defaults to 100",
			input:     storage.LogQueryParams{Limit: 0},
			wantErr:   false,
			wantLimit: 100,
		},
		{
			name:      "negative limit defaults to 100",
			input:     storage.LogQueryParams{Limit: -5},
			wantErr:   false,
			wantLimit: 100,
		},
		{
			name:      "limit 5000 clamped to 1000",
			input:     storage.LogQueryParams{Limit: 5000},
			wantErr:   false,
			wantLimit: 1000,
		},
		{
			name:      "limit 1001 clamped to 1000",
			input:     storage.LogQueryParams{Limit: 1001},
			wantErr:   false,
			wantLimit: 1000,
		},
		{
			name:      "limit 50 unchanged",
			input:     storage.LogQueryParams{Limit: 50},
			wantErr:   false,
			wantLimit: 50,
		},
		{
			name:      "limit 1000 unchanged",
			input:     storage.LogQueryParams{Limit: 1000},
			wantErr:   false,
			wantLimit: 1000,
		},
		{
			name:      "empty level is valid",
			input:     storage.LogQueryParams{Level: "", Limit: 10},
			wantErr:   false,
			wantLimit: 10,
		},
		{
			name:      "level info is valid",
			input:     storage.LogQueryParams{Level: "info", Limit: 10},
			wantErr:   false,
			wantLimit: 10,
		},
		{
			name:      "level warn is valid",
			input:     storage.LogQueryParams{Level: "warn", Limit: 10},
			wantErr:   false,
			wantLimit: 10,
		},
		{
			name:      "level error is valid",
			input:     storage.LogQueryParams{Level: "error", Limit: 10},
			wantErr:   false,
			wantLimit: 10,
		},
		{
			name:      "level debug is valid",
			input:     storage.LogQueryParams{Level: "debug", Limit: 10},
			wantErr:   false,
			wantLimit: 10,
		},
		{
			name:    "level critical is invalid",
			input:   storage.LogQueryParams{Level: "critical", Limit: 10},
			wantErr: true,
		},
		{
			name:    "level warning is invalid",
			input:   storage.LogQueryParams{Level: "warning", Limit: 10},
			wantErr: true,
		},
		{
			name:    "level fatal is invalid",
			input:   storage.LogQueryParams{Level: "fatal", Limit: 10},
			wantErr: true,
		},
		{
			name:    "level uppercase INFO is invalid",
			input:   storage.LogQueryParams{Level: "INFO", Limit: 10},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			p := tt.input
			err := p.Validate()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("expected nil, got %v", err)
			}
			if p.Limit != tt.wantLimit {
				t.Fatalf("expected Limit=%d after Validate(), got %d", tt.wantLimit, p.Limit)
			}
		})
	}
}

// TestLogQueryParams_Validate_MutatesPointer confirms that Validate() mutates the
// receiver (pointer receiver), so callers see the corrected Limit value.
func TestLogQueryParams_Validate_MutatesPointer(t *testing.T) {
	p := storage.LogQueryParams{Limit: 0}
	if err := p.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Limit != 100 {
		t.Fatalf("expected Limit=100 after Validate() on pointer, got %d", p.Limit)
	}
}
