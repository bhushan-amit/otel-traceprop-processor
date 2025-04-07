package main

import (
	"github.com/bhushan-amit/otel-traceprop-processor/processor/tracepropagatorprocessor"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/service"
)

func main() {
	factories, err := service.DefaultComponents()
	if err != nil {
		panic(err)
	}

	// Register our custom processor
	factories.Processors["tracepropagator"] = component.ProcessorFactory(
		tracepropagatorprocessor.NewFactory(),
	)

	cmd := service.NewCommand(service.Settings{
		Factories: factories,
	})

	if err := cmd.Execute(); err != nil {
		panic(err)
	}
}
