package tracepropagatorprocessor

import (
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap"
)

// Config defines the processor config.
type Config struct {
	config.ProcessorSettings `mapstructure:",squash"`
	// We can add processor-specific config fields here later.
}
