package common

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"
	log "github.com/sirupsen/logrus"
)

// API for Query Metrics
type PrometheusAPI struct {
	//Base URL
	Pushgateway string
}

func NewPrometheusAPI(pushgateway string) *PrometheusAPI {
	return &PrometheusAPI{Pushgateway: pushgateway}
}

func (a *PrometheusAPI) SetMetricValue(metricName string, tags map[string]string, value float64) error {
	newMetric := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: metricName,
		Help: "new metric generated by governor",
	})
	newMetric.Set(value)
	var pusher = push.New(a.Pushgateway, "governor").Collector(newMetric)
	// Copy from the original map to the target map
	for key, value := range tags {
		pusher.Grouping(key, value)
	}

	if err := pusher.Push(); err != nil {
		log.Warnf("failed to push metric to pushgateway: %s, %v", metricName, tags)
		return err
	}
	return nil
}
