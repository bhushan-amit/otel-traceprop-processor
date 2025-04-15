package tracepropagatorprocessor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

func newTestProcessor(t *testing.T) *tracePropagatorProcessor {
	logger, err := zap.NewDevelopment()
	assert.NoError(t, err)

	return &tracePropagatorProcessor{
		logger: logger,
	}
}

func TestProcessTraces_RootAndChildSpanPropagation(t *testing.T) {
	p := newTestProcessor(t)

	// Build test trace
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()
	spans := ss.Spans()

	// Root span
	rootSpan := spans.AppendEmpty()
	rootSpan.SetName("RootSpan")
	rootSpan.SetSpanID([8]byte{0x01})
	rootSpan.SetTraceID([16]byte{0x01})

	// Child span
	childSpan := spans.AppendEmpty()
	childSpan.SetName("ChildSpan")
	childSpan.SetSpanID([8]byte{0x02})
	childSpan.SetParentSpanID(rootSpan.SpanID())
	childSpan.SetTraceID(rootSpan.TraceID())

	// Orphan span (no matching parent)
	orphanSpan := spans.AppendEmpty()
	orphanSpan.SetName("OrphanChild")
	orphanSpan.SetSpanID([8]byte{0x03})
	orphanSpan.SetParentSpanID([8]byte{0xAA}) // Unknown parent
	orphanSpan.SetTraceID(rootSpan.TraceID())

	// Process the trace
	processed, err := p.processTraces(context.Background(), td)
	assert.NoError(t, err)

	processedSpans := processed.ResourceSpans().At(0).ScopeSpans().At(0).Spans()

	// Validate TraceName on root span
	val, _ := processedSpans.At(0).Attributes().Get("TraceName")
	assert.Equal(t, "RootSpan", val.Str())

	// Validate TraceParentName on child span
	val, _ = processedSpans.At(1).Attributes().Get("TraceParentName")
	assert.Equal(t, "RootSpan", val.Str())

	// Validate Orphan span has no TraceParentName
	_, exists := processedSpans.At(2).Attributes().Get("TraceParentName")
	assert.False(t, exists, "Orphan span should not have TraceParentName")
}

func TestProcessTraces_OnlyRootSpan(t *testing.T) {
	p := newTestProcessor(t)

	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()
	spans := ss.Spans()

	// Only root span
	rootSpan := spans.AppendEmpty()
	rootSpan.SetName("OnlyRoot")
	rootSpan.SetSpanID([8]byte{0x10})
	rootSpan.SetTraceID([16]byte{0x10})

	processed, err := p.processTraces(context.Background(), td)
	assert.NoError(t, err)

	processedSpan := processed.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)

	val, _ := processedSpan.Attributes().Get("TraceName")
	assert.Equal(t, "OnlyRoot", val.Str())

	_, exists := processedSpan.Attributes().Get("TraceParentName")
	assert.False(t, exists)

}

//package tracepropagatorprocessor
//
//import (
//	"context"
//	"testing"
//
//	"go.opentelemetry.io/collector/consumer/consumertest"
//	"go.opentelemetry.io/collector/pdata/ptrace"
//	"go.uber.org/zap/zaptest"
//)
//
//func TestProcessTraces_AddsHelloWorldAttribute(t *testing.T) {
//	// Setup a fake logger
//	logger := zaptest.NewLogger(t)
//
//	// Create dummy config (can be empty for now)
//	cfg := &Config{}
//
//	// Create a dummy next consumer that does nothing
//	next := consumertest.NewNop()
//
//	// Initialize your processor
//	p := newTracePropagatorProcessor(logger, cfg, next)
//
//	// Build a sample trace
//	td := ptrace.NewTraces()
//	rs := td.ResourceSpans().AppendEmpty()
//	ss := rs.ScopeSpans().AppendEmpty()
//	span := ss.Spans().AppendEmpty()
//
//	span.SetName("test-span")
//	span.SetSpanID([8]byte{1, 2, 3, 4, 5, 6, 7, 8})
//
//	// Call your logic
//	newTd, err := p.processTraces(context.Background(), td)
//	if err != nil {
//		t.Fatalf("unexpected error: %v", err)
//	}
//
//	// Verify that hello=world was added to the span
//	attrVal, found := newTd.ResourceSpans().At(0).
//		ScopeSpans().At(0).
//		Spans().At(0).
//		Attributes().Get("hello")
//
//	if !found {
//		t.Errorf("expected 'hello' attribute to be present")
//	}
//	if attrVal.Str() != "world" {
//		t.Errorf("expected 'hello' attribute to have value 'world', got: %v", attrVal.Str())
//	}
//}
