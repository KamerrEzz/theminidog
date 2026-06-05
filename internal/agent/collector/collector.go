package collector

import (
	"context"
	"fmt"

	"github.com/kamerrezz/theminidog/internal/model"
)

// Collector gathers a set of metrics from the host.
type Collector interface {
	Name() string
	Collect(ctx context.Context) ([]model.Metric, error)
}

// Registry holds a set of Collectors and runs them together.
type Registry struct {
	collectors []Collector
}

// NewRegistry returns a Registry pre-loaded with the given collectors.
func NewRegistry(collectors ...Collector) *Registry {
	return &Registry{collectors: collectors}
}

// Add appends a Collector to the registry.
func (r *Registry) Add(c Collector) {
	r.collectors = append(r.collectors, c)
}

// CollectAll runs every collector and returns all metrics plus any errors.
// A single collector error does not stop the others.
func (r *Registry) CollectAll(ctx context.Context) ([]model.Metric, []error) {
	var metrics []model.Metric
	var errs []error
	for _, c := range r.collectors {
		m, err := c.Collect(ctx)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", c.Name(), err))
		}
		metrics = append(metrics, m...)
	}
	return metrics, errs
}
