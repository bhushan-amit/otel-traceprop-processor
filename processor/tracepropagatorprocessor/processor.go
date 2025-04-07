package tracepropagatorprocessor

import (
	"context"
	"go.opentelemetry.io/collector/component"

	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor"
)

func createTracesProcessor(
	_ context.Context,
	_ processor.CreateSettings,
	cfg component.Config,
	nextConsumer processor.Traces,
) (processor.Traces, error) {
	return &tracePropProcessor{
		next: nextConsumer,
	}, nil
}

type tracePropProcessor struct {
	next processor.Traces
}

func (p *tracePropProcessor) Capabilities() processor.Capabilities {
	return processor.Capabilities{MutatesData: true}
}

func (p *tracePropProcessor) Start(_ context.Context, _ component.Host) error {
	return nil
}

func (p *tracePropProcessor) Shutdown(_ context.Context) error {
	return nil
}

func (p *tracePropProcessor) ConsumeTraces(ctx context.Context, td ptrace.Traces) error {
	// No-op logic for now
	return p.next.ConsumeTraces(ctx, td)
}
