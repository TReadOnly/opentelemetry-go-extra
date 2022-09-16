package main

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"sync"
	"time"

	runtimemetrics "go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric/global"
	"go.opentelemetry.io/otel/metric/instrument"
	"go.opentelemetry.io/otel/metric/unit"
	"go.opentelemetry.io/otel/sdk/metric/aggregator/histogram"
	controller "go.opentelemetry.io/otel/sdk/metric/controller/basic"
	"go.opentelemetry.io/otel/sdk/metric/export/aggregation"
	processor "go.opentelemetry.io/otel/sdk/metric/processor/basic"
	selector "go.opentelemetry.io/otel/sdk/metric/selector/simple"
)

var meter = global.MeterProvider().Meter("com.example.app1")

var gaugeValue float64
var gaugeMutx sync.RWMutex

func main() {
	ctx := context.Background()
	configureOpentelemetry()

	counter, err := meter.SyncInt64().Counter(
		"test.my_counter",
		instrument.WithDescription("Just a test counter"),
	)
	if err != nil {
		panic(err)
	}

	counter2, err := meter.SyncInt64().Counter(
		"test.my_counter2",
		instrument.WithDescription("Just a test counter"),
	)
	if err != nil {
		panic(err)
	}

	histogram, err := meter.SyncInt64().Histogram(
		"test.histogram1",
		instrument.WithDescription("Test histogram metric"),
		instrument.WithUnit(unit.Milliseconds),
	)

	if err != nil {
		panic(err)
	}

	gaugeObserver(ctx)

	for {
		n := rand.Intn(1000)
		time.Sleep(time.Duration(n) * time.Millisecond)

		counter.Add(ctx, 1, attribute.String("test_attr", fmt.Sprintf("hello %d", n%5)))
		counter2.Add(ctx, 1, attribute.String("test_attr", fmt.Sprintf("world %d", n%5)),
			attribute.String("test_attr2", fmt.Sprintf("value2 %d", n%5)))

		latency := time.Duration(n) * time.Millisecond
		histogram.Record(ctx, latency.Milliseconds(), attribute.Bool("test_bool1", n%2 == 0))

		gaugeMutx.Lock()
		gaugeValue = rand.Float64()
		gaugeMutx.Unlock()
	}
}

func configureOpentelemetry() {
	exporter := configureMetrics()

	if err := runtimemetrics.Start(); err != nil {
		panic(err)
	}

	http.HandleFunc("/metrics", exporter.ServeHTTP)
	fmt.Println("listenening on http://localhost:8088/metrics")

	go func() {
		_ = http.ListenAndServe(":8088", nil)
	}()
}

func configureMetrics() *prometheus.Exporter {
	config := prometheus.Config{}

	ctrl := controller.New(
		processor.NewFactory(
			selector.NewWithHistogramDistribution(
				histogram.WithExplicitBoundaries(config.DefaultHistogramBoundaries),
			),
			aggregation.CumulativeTemporalitySelector(),
			processor.WithMemory(true),
		),
	)

	exporter, err := prometheus.New(config, ctrl)
	if err != nil {
		panic(err)
	}

	global.SetMeterProvider(exporter.MeterProvider())

	return exporter
}

// gaugeObserver demonstrates how to measure non-additive numbers that can go up and down,
// for example, cache hit rate or memory utilization.
func gaugeObserver(ctx context.Context) {
	gauge, _ := meter.AsyncFloat64().Gauge(
		"test.gauge_observer1",
		instrument.WithUnit(unit.Bytes),
		instrument.WithDescription("Gauge observer in bytes"),
	)

	if err := meter.RegisterCallback(
		[]instrument.Asynchronous{
			gauge,
		},
		func(ctx context.Context) {
			gaugeMutx.RLock()
			v := gaugeValue
			gaugeMutx.RUnlock()
			gauge.Observe(ctx, v)
		},
	); err != nil {
		panic(err)
	}
}
