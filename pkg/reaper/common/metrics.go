package common

import (
	"context"
)

// API for Query Metrics
type MetricsAPI interface {

	// Set Metric value on metric
	SetMetricValue(ctx context.Context, metricName string, tags map[string]string, value float64) error
}
