package tracepropagatorprocessor

import (
	"go.opentelemetry.io/collector/component"
)

type Config struct {
	component.Config // embeds the base processor config
	// Add any custom config fields here
}
