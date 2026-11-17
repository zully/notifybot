package bot

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type metrics struct {
	registry           *prometheus.Registry
	connected          prometheus.Gauge
	online             prometheus.Gauge
	connectionAttempts prometheus.Counter
	connectionErrors   prometheus.Counter
	isonRequests       prometheus.Counter
	isonResponses      prometheus.Counter
	lastISON           prometheus.Gauge
	presenceChanges    *prometheus.CounterVec
	emails             *prometheus.CounterVec
	emailDuration      prometheus.Histogram
}

func newMetrics(watched int) *metrics {
	registry := prometheus.NewRegistry()
	gauge := func(name, help string) prometheus.Gauge {
		metric := prometheus.NewGauge(prometheus.GaugeOpts{Name: "notifybot_" + name, Help: help})
		registry.MustRegister(metric)
		return metric
	}
	counter := func(name, help string) prometheus.Counter {
		metric := prometheus.NewCounter(prometheus.CounterOpts{Name: "notifybot_" + name, Help: help})
		registry.MustRegister(metric)
		return metric
	}
	m := &metrics{
		registry:           registry,
		connected:          gauge("irc_connected", "Whether IRC registration is complete."),
		online:             gauge("online_nicknames", "Number of watched nicknames online; zero while disconnected."),
		connectionAttempts: counter("connection_attempts_total", "TCP connection attempts, including retries."),
		connectionErrors:   counter("connection_errors_total", "Failed connection attempts and unexpected disconnects."),
		isonRequests:       counter("ison_requests_total", "Successfully written ISON requests."),
		isonResponses:      counter("ison_responses_total", "Valid ISON responses received."),
		lastISON:           gauge("last_ison_response_timestamp_seconds", "Unix timestamp of the last valid ISON response; zero before the first."),
		presenceChanges:    prometheus.NewCounterVec(prometheus.CounterOpts{Name: "notifybot_presence_changes_total", Help: "Observed presence changes."}, []string{"state"}),
		emails:             prometheus.NewCounterVec(prometheus.CounterOpts{Name: "notifybot_email_attempts_total", Help: "SES send attempts by result."}, []string{"result"}),
		emailDuration:      prometheus.NewHistogram(prometheus.HistogramOpts{Name: "notifybot_email_duration_seconds", Help: "SES send duration, including failures.", Buckets: prometheus.DefBuckets}),
	}
	gauge("watched_nicknames", "Number of configured watched nicknames.").Set(float64(watched))
	registry.MustRegister(m.presenceChanges, m.emails, m.emailDuration)
	for _, state := range []string{"online", "offline"} {
		m.presenceChanges.WithLabelValues(state)
	}
	for _, result := range []string{"success", "error"} {
		m.emails.WithLabelValues(result)
	}
	return m
}

// MetricsHandler exposes this bot's metrics without global registry state.
func (b *NotifyBot) MetricsHandler() http.Handler {
	return promhttp.HandlerFor(b.metrics.registry, promhttp.HandlerOpts{})
}
