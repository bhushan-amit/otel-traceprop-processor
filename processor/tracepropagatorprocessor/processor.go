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
	logger.Info("🚀 Initializing tracePropagatorProcessor")
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
	t.logger.Info("🛑 Shutting down tracePropagatorProcessor")
	return nil
}

func (t *tracePropagatorProcessor) processTraces(ctx context.Context, td ptrace.Traces) (ptrace.Traces, error) {
	rs := td.ResourceSpans()

	// Global map to store root span names keyed by their SpanID
	parentSpanMap := make(map[string]string)

	for i := 0; i < rs.Len(); i++ {
		scopeSpans := rs.At(i).ScopeSpans()

		for j := 0; j < scopeSpans.Len(); j++ {
			spans := scopeSpans.At(j).Spans()

			for k := 0; k < spans.Len(); k++ {
				span := spans.At(k)

				// Handle root span (no parent)
				if span.ParentSpanID().IsEmpty() {
					spanID := span.SpanID().String()
					spanName := span.Name()

					span.Attributes().PutStr("TraceName", spanName)
					parentSpanMap[spanID] = spanName

					// 🔍 Log root span processing
					t.logger.Info("Set TraceName on root span",
						zap.String("span_id", spanID),
						zap.String("name", spanName))
				}
			}
		}
	}

	// Second pass: set TraceParentName on child spans
	for i := 0; i < rs.Len(); i++ {
		scopeSpans := rs.At(i).ScopeSpans()

		for j := 0; j < scopeSpans.Len(); j++ {
			spans := scopeSpans.At(j).Spans()

			for k := 0; k < spans.Len(); k++ {
				span := spans.At(k)
				parentID := span.ParentSpanID().String()

				if !span.ParentSpanID().IsEmpty() {
					if parentName, ok := parentSpanMap[parentID]; ok {
						span.Attributes().PutStr("TraceParentName", parentName)

						t.logger.Info("Propagated TraceParentName to child span",
							zap.String("span_id", span.SpanID().String()),
							zap.String("parent_span_id", parentID),
							zap.String("parent_name", parentName))
					}
				}
			}
		}
	}

	return td, nil
}
