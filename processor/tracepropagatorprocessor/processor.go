package tracepropagatorprocessor

import (
	"context"
	"go.uber.org/zap"

	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

type tracePropagatorProcessor struct {
	logger *zap.Logger
	config *Config
	next   consumer.Traces
}

func newTracePropagatorProcessor(logger *zap.Logger, cfg *Config, next consumer.Traces) *tracePropagatorProcessor {
	return &tracePropagatorProcessor{
		logger: logger,
		config: cfg,
		next:   next,
	}
}
func (t *tracePropagatorProcessor) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: true}
}

func (t *tracePropagatorProcessor) ConsumeTraces(ctx context.Context, td ptrace.Traces) error {
	// Insert your trace propagation logic here
	return t.next.ConsumeTraces(ctx, td)
}

func (t *tracePropagatorProcessor) processTraces(ctx context.Context, td ptrace.Traces) (ptrace.Traces, error) {
	// You can mutate or inspect the trace data here before forwarding
	return td, nil
}

func (t *tracePropagatorProcessor) Shutdown(ctx context.Context) error {
	return nil
}
