// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

// Package otelx is the OpenTelemetry exporter at the CLI edge: it maps a run's
// result to an OTLP trace (run → scenario → step, findings as span events).
// It is a projection of the run, never its source of truth; core never imports
// it.
package otelx

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/shinari-dev/shinari/core/engine"
)

// BuildSpans maps a run result onto the tracer as a span tree using the run's
// recorded timestamps: a root run span, a span per scenario, a span per step,
// and one span event per finding on its scenario span.
func BuildSpans(ctx context.Context, tracer trace.Tracer, res engine.RunResult) {
	rootCtx, root := tracer.Start(ctx, "shinari.run", trace.WithTimestamp(res.Start))
	for _, sc := range res.Scenarios {
		scCtx, scSpan := tracer.Start(rootCtx, "scenario:"+sc.Name, trace.WithTimestamp(sc.Start))
		scSpan.SetAttributes(attribute.String("shinari.verdict", string(sc.Verdict)))
		for _, st := range sc.Steps {
			_, stSpan := tracer.Start(scCtx, st.Section+"/"+st.Label(), trace.WithTimestamp(st.Start))
			stSpan.SetAttributes(attribute.String("shinari.verdict", string(st.Verdict)))
			if st.Err != "" {
				stSpan.SetAttributes(attribute.String("shinari.error", st.Err))
			}
			stSpan.End(trace.WithTimestamp(st.End))
		}
		for _, f := range sc.Findings {
			scSpan.AddEvent("finding", trace.WithAttributes(
				attribute.String("shinari.finding.id", f.ID),
				attribute.String("shinari.finding.narrative", f.Narrative),
				attribute.Bool("shinari.finding.nowPasses", f.NowPasses),
			))
		}
		scSpan.End(trace.WithTimestamp(sc.End))
	}
	root.End(trace.WithTimestamp(res.End))
}

// Export ships a run's spans to an OTLP/gRPC endpoint (insecure). It builds a
// tracer provider, maps the run, and shuts down to flush. The caller bounds it
// with a context deadline; a failure is the caller's to surface, not fatal.
func Export(ctx context.Context, endpoint string, res engine.RunResult) error {
	exp, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(resource.NewSchemaless(
			attribute.String("service.name", "shinari"),
		)),
	)
	BuildSpans(ctx, tp.Tracer("shinari"), res)
	return tp.Shutdown(ctx)
}
