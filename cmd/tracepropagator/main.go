package main

import (
	"context"
	"log"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/provider/fileprovider"
	"go.opentelemetry.io/collector/confmap/provider/yamlprovider"
	"go.opentelemetry.io/collector/otelcol"

	tracepropagatorprocessor "github.com/bhushan-amit/otel-traceprop-processor/processor/tracepropagatorprocessor"
)

func main() {
	ctx := context.Background()

	// Build config provider
	configProviderSettings := otelcol.ConfigProviderSettings{
		ResolverSettings: confmap.ResolverSettings{
			ProviderFactories: map[string]confmap.ProviderFactory{
				"file": fileprovider.NewFactory(),
				"yaml": yamlprovider.NewFactory(),
			},
		},
	}

	// Prepare the Collector Settings
	settings := otelcol.CollectorSettings{
		// Factories must be a function returning (Factories, error)
		Factories: func() (otelcol.Factories, error) {
			factories, err := otelcol.DefaultFactories()
			if err != nil {
				return otelcol.Factories{}, err
			}

			// Add custom processor
			factories.Processors[component.MustNewType("tracepropagator")] = tracepropagatorprocessor.NewFactory()
			return factories, nil
		},
		ConfigProviderSettings: configProviderSettings,
	}

	// Build and run the collector
	cmd := otelcol.NewCommand(settings)
	if err := cmd.ExecuteContext(ctx); err != nil {
		log.Fatalf("Failed to run collector: %v", err)
	}
}
