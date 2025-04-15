package tracepropagatorprocessor // import "github.com/bhushan-amit/otel-traceprop-processor/processor/tracepropagatorprocessor"

import (
	"context"
	"go.uber.org/zap"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/processor/processorhelper"
)

var processorCapabilities = consumer.Capabilities{MutatesData: true}

const typeStr = "tracepropagator"

// NewFactory returns a new factory for the Trace Propagator processor.
func NewFactory() processor.Factory {
	return processor.NewFactory(
		component.MustNewType(typeStr),
		createDefaultConfig,
		processor.WithTraces(createTracesProcessor, component.StabilityLevelDevelopment),
	)
}

func createDefaultConfig() component.Config {
	return &Config{}
}

func createTracesProcessor(
	ctx context.Context,
	set processor.Settings,
	cfg component.Config,
	nextConsumer consumer.Traces,
) (processor.Traces, error) {
	logger := set.Logger
	logger.Info("⚡ Creating TracePropagatorProcessor",
		zap.String("component", "tracepropagator"),
	)

	pCfg := cfg.(*Config)
	p := newTracePropagatorProcessor(logger, pCfg, nextConsumer)

	logger.Info("✅ TracePropagatorProcessor created successfully")

	return processorhelper.NewTraces(
		ctx,
		set,
		cfg,
		nextConsumer,
		p.processTraces,
		processorhelper.WithCapabilities(processorCapabilities),
	)
}
