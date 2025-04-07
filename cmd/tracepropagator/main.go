package main

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/otelcol/otelcoltest"

	tracepropagatorprocessor "github.com/bhushan-amit/otel-traceprop-processor/processor/tracepropagatorprocessor"
)

func main() {
	factories, err := otelcoltest.NopFactories()
	if err != nil {
		panic(err)
	}

	factories.Processors[component.MustNewType("tracepropagator")] = tracepropagatorprocessor.NewFactory()

	params := otelcol.CollectorSettings{
		Factories: func() (otelcol.Factories, error) {
			return factories, nil
		},
	}

	collector, err := otelcol.NewCollector(params)
	if err != nil {
		panic(err)
	}

	if err := collector.Run(context.Background()); err != nil {
		panic(err)
	}
}
