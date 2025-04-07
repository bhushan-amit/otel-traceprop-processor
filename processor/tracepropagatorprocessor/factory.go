// file: processor/tracepropagatorprocessor/factory.go
package tracepropagatorprocessor

import (
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/processor"
)

// NewFactory creates a new processor factory.
func NewFactory() component.ProcessorFactory {
	return processor.NewFactory(
		"tracepropagator",
		createDefaultConfig,
		processor.WithTraces(createTracesProcessor),
	)
}
