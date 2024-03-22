package main

import (
	"context"
	"log"
	"net"

	formulatel "github.com/isnor/formulatel/server"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
)

type FormulaTelConfig struct {
	ExporterHost string `envconfig:"OTEL_EXPORTER_ADDRESS"`
}

// I largely copied this from the otel-demo just to get started producing metrics
func initMeterProvider() *sdkmetric.MeterProvider {
	ctx := context.Background()

	basicMetrics, err := sdkresource.New(
		context.Background(), // TODO: should this be background?
		sdkresource.WithOS(),
		sdkresource.WithProcess(),
		sdkresource.WithContainer(),
		sdkresource.WithHost(),
	)
	if err != nil {
		panic(err) // TODO: remove this
	}
	resource, _ := sdkresource.Merge(
		sdkresource.Default(),
		basicMetrics,
	)

	exporter, err := otlpmetricgrpc.New(ctx)
	if err != nil {
		log.Fatalf("new otlp metric grpc exporter failed: %v", err)
	}

	metrics := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)),
		sdkmetric.WithResource(resource),
	)
	otel.SetMeterProvider(metrics)
	return metrics
}

func main() {
	server := formulatel.FormulaTelServer()
	ctx := context.Background()

	meterProvider := initMeterProvider()
	defer func() {
		if err := meterProvider.Shutdown(ctx); err != nil {
			log.Printf("Error shutting down meter provider: %v", err)
		}
	}()

	listener, err := net.Listen("tcp", "0.0.0.0:29292")
	if err != nil {
		panic(err)
	}

	defer listener.Close()

	println("formulatel-rpc listening on 29292")
	server.Serve(listener)
}
