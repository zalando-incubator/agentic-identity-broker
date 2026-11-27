package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"math/rand/v2"
	"slices"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
)

// ShuffledMetadataJSON encodes a metadata member value with the keys of both
// maps in a random order. encoding/json would always sort them.
func ShuffledMetadataJSON(labels, annotations map[string]string, random *rand.Rand) string {
	members := []string{
		`"labels":` + shuffledObject(labels, random),
		`"annotations":` + shuffledObject(annotations, random),
	}
	random.Shuffle(len(members), func(i, j int) { members[i], members[j] = members[j], members[i] })
	return "{" + strings.Join(members, ",") + "}"
}

func shuffledObject(entries map[string]string, random *rand.Rand) string {
	keys := slices.Collect(maps.Keys(entries))
	slices.Sort(keys)
	random.Shuffle(len(keys), func(i, j int) { keys[i], keys[j] = keys[j], keys[i] })
	parts := make([]string, len(keys))
	for i, key := range keys {
		parts[i] = jsonString(key) + ":" + jsonString(entries[key])
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func jsonString(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

// TelemetryCapture holds the three signal recorders a broker under test writes to.
type TelemetryCapture struct {
	Logs    *bootstrap.BufferedLogCapture
	Spans   *tracetest.SpanRecorder
	Metrics *sdkmetric.ManualReader
}

// SentinelLeaks returns a description of every place a sentinel appears: a log
// record, a span name, attribute, or event, or a metric attribute. It returns
// nil when no signal carries any sentinel.
func (c TelemetryCapture) SentinelLeaks(sentinels []string) []string {
	var leaks []string
	check := func(where, text string) {
		for _, sentinel := range sentinels {
			if strings.Contains(text, sentinel) {
				leaks = append(leaks, fmt.Sprintf("%s contains %q", where, sentinel))
			}
		}
	}
	if c.Logs != nil {
		check("log output", c.Logs.Raw())
	}
	if c.Spans != nil {
		for _, span := range c.Spans.Ended() {
			check("span name "+span.Name(), span.Name())
			check("span status "+span.Name(), span.Status().Description)
			for _, attribute := range span.Attributes() {
				check("span attribute "+string(attribute.Key), string(attribute.Key)+"="+attribute.Value.String())
			}
			for _, event := range span.Events() {
				check("span event "+event.Name, event.Name)
				for _, attribute := range event.Attributes {
					check("span event attribute "+string(attribute.Key), string(attribute.Key)+"="+attribute.Value.String())
				}
			}
		}
	}
	if c.Metrics != nil {
		var collected metricdata.ResourceMetrics
		if err := c.Metrics.Collect(context.Background(), &collected); err != nil {
			return append(leaks, "metric collection failed: "+err.Error())
		}
		for _, scope := range collected.ScopeMetrics {
			for _, metric := range scope.Metrics {
				for _, set := range metricAttributeSets(metric.Data) {
					for _, attribute := range set.ToSlice() {
						check("metric "+metric.Name+" attribute "+string(attribute.Key), string(attribute.Key)+"="+attribute.Value.String())
					}
				}
			}
		}
	}
	return leaks
}

func metricAttributeSets(data metricdata.Aggregation) []attribute.Set {
	var sets []attribute.Set
	switch data := data.(type) {
	case metricdata.Sum[int64]:
		for _, point := range data.DataPoints {
			sets = append(sets, point.Attributes)
		}
	case metricdata.Sum[float64]:
		for _, point := range data.DataPoints {
			sets = append(sets, point.Attributes)
		}
	case metricdata.Gauge[int64]:
		for _, point := range data.DataPoints {
			sets = append(sets, point.Attributes)
		}
	case metricdata.Gauge[float64]:
		for _, point := range data.DataPoints {
			sets = append(sets, point.Attributes)
		}
	case metricdata.Histogram[int64]:
		for _, point := range data.DataPoints {
			sets = append(sets, point.Attributes)
		}
	case metricdata.Histogram[float64]:
		for _, point := range data.DataPoints {
			sets = append(sets, point.Attributes)
		}
	case metricdata.ExponentialHistogram[int64]:
		for _, point := range data.DataPoints {
			sets = append(sets, point.Attributes)
		}
	case metricdata.ExponentialHistogram[float64]:
		for _, point := range data.DataPoints {
			sets = append(sets, point.Attributes)
		}
	}
	return sets
}
