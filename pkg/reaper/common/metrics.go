package common

type MetricsAPI interface {

	// Set Metric value on metric
	SetMetricValue(metricName string, tags map[string]string, value float64) error
}
