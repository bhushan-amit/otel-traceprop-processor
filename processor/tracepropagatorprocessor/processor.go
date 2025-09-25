package tracepropagatorprocessor

import (
	"context"

	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
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
	return t.next.ConsumeTraces(ctx, td)
}

func (t *tracePropagatorProcessor) Shutdown(ctx context.Context) error {
	return nil
}

func (t *tracePropagatorProcessor) processTraces(ctx context.Context, td ptrace.Traces) (ptrace.Traces, error) {
	rs := td.ResourceSpans()

	// Maps to track parent relationships
	parentSpanMap := make(map[string]string) // SpanID -> TraceName (for root spans)

	for i := 0; i < rs.Len(); i++ {
		scopeSpans := rs.At(i).ScopeSpans()

		for j := 0; j < scopeSpans.Len(); j++ {
			spans := scopeSpans.At(j).Spans()

			for k := 0; k < spans.Len(); k++ {
				span := spans.At(k)
				spanID := span.SpanID().String()
				// Maintain a directory os span
				spanName := span.Name()
				parentSpanMap[spanID] = spanName
			}
		}
	}

	// Second pass: Set TraceParentName and ConsumerName
	for i := 0; i < rs.Len(); i++ {
		scopeSpans := rs.At(i).ScopeSpans()

		for j := 0; j < scopeSpans.Len(); j++ {
			spans := scopeSpans.At(j).Spans()

			for k := 0; k < spans.Len(); k++ {
				span := spans.At(k)
				parentID := span.ParentSpanID().String()

				if !span.ParentSpanID().IsEmpty() {
					// Set TraceParentName if applicable
					if parentName, ok := parentSpanMap[parentID]; ok {
						span.Attributes().PutStr("TraceParentName", parentName)
					}
				}
			}
		}
	}

	return td, nil
}
