package agent

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/kamerrezz/theminidog/internal/model"
)

// registry is the interface the Agent uses to collect metrics from all collectors.
// *collector.Registry satisfies this interface.
type registry interface {
	CollectAll(ctx context.Context) ([]model.Metric, []error)
}

// senderIface is the interface the Agent uses to ship batches.
// It matches sender.Sender exactly so that package is not imported here.
type senderIface interface {
	Send(ctx context.Context, batch model.MetricBatch) error
}

// Agent coordinates metric collection and delivery.
type Agent struct {
	registry registry
	sender   senderIface
	host     string
	interval time.Duration
	batches  chan model.MetricBatch
	log      *slog.Logger
}

// Options configures an Agent.
type Options struct {
	Host     string
	Interval time.Duration
	BufSize  int // default 10
}

// New creates a new Agent.
func New(reg registry, snd senderIface, opts Options) *Agent {
	if opts.BufSize <= 0 {
		opts.BufSize = 10
	}
	return &Agent{
		registry: reg,
		sender:   snd,
		host:     opts.Host,
		interval: opts.Interval,
		batches:  make(chan model.MetricBatch, opts.BufSize),
		log:      slog.Default(),
	}
}

// Run starts the collection and sender loops, blocking until ctx is cancelled.
func (a *Agent) Run(ctx context.Context) {
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		defer close(a.batches)
		a.collectLoop(ctx)
	}()

	go func() {
		defer wg.Done()
		a.senderLoop(ctx)
	}()

	wg.Wait()
}

func (a *Agent) collectLoop(ctx context.Context) {
	ticker := time.NewTicker(a.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			metrics, errs := a.registry.CollectAll(ctx)
			for _, err := range errs {
				a.log.WarnContext(ctx, "collector error", "err", err)
			}
			if len(metrics) == 0 {
				continue
			}
			batch := model.MetricBatch{Host: a.host, Metrics: metrics}
			select {
			case a.batches <- batch:
			default:
				a.log.WarnContext(ctx, "batch channel full, dropping")
			}
		}
	}
}

func (a *Agent) senderLoop(ctx context.Context) {
	for batch := range a.batches {
		if err := a.sender.Send(ctx, batch); err != nil {
			if ctx.Err() != nil {
				return
			}
			a.log.ErrorContext(ctx, "send error", "err", err)
		}
	}
}
