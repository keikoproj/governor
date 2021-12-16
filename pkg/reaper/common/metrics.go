package common

<<<<<<< HEAD
<<<<<<< HEAD
type MetricsAPI interface {

	// Set Metric value on metric
	SetMetricValue(metricName string, tags map[string]string, value float64) error
=======
import (
	"context"
)

// API for Query Metrics
type MetricsAPI interface {

	// Set Metric value on metric
	SetMetricValue(ctx context.Context, metricName string, tags map[string]string, value float64) error
>>>>>>> 7a298b8 (Pushgateway API)
=======
type MetricsAPI interface {

	// Set Metric value on metric
	SetMetricValue(metricName string, tags map[string]string, value float64) error
>>>>>>> 25d4af5 (Add reason metrics)
}
