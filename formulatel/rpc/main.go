package main

import (
	"context"
	"log"
	"net"
	"time"

	formulatel "github.com/isnor/formulatel/server"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
)

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

	rpcExporter, err := otlpmetricgrpc.New(ctx)
	if err != nil {
		log.Fatalf("new otlp metric grpc exporter failed: %v", err)
	}

	// stdoutExporter, err := stdoutmetric.New()
	// if err != nil {
	// 	log.Fatalf("failed creating stdout exporter %v", err)
	// }
	metrics := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(rpcExporter)),
		// sdkmetric.WithReader(sdkmetric.NewPeriodicReader(stdoutExporter)),
		sdkmetric.WithResource(resource),
	)
	otel.SetMeterProvider(metrics)
	return metrics
}

func main() {
	ctx := context.Background()

	meterProvider := initMeterProvider()
	defer func() {
		if err := meterProvider.Shutdown(ctx); err != nil {
			log.Printf("Error shutting down meter provider: %v", err)
		}
	}()
	server := formulatel.FormulaTelServer(otel.Meter("formulatelrpc"))

	err := runtime.Start(runtime.WithMinimumReadMemStatsInterval(time.Second))
	if err != nil {
		log.Fatal(err)
	}

	listener, err := net.Listen("tcp", "0.0.0.0:29292")
	if err != nil {
		panic(err)
	}

	defer listener.Close()

	println("formulatel-rpc listening on 29292")
	server.Serve(listener)
}
