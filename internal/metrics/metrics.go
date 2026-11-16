// Package metrics exposes the Prometheus metrics of the W1nWire server and
// client.
//
// The metric names are part of the public contract of the project: the
// dashboards and the alert rules in the monitoring directory rely on them.
package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds every metric of the project.
type Metrics struct {
	Up                prometheus.Gauge
	Handshakes        *prometheus.CounterVec
	HandshakeDuration prometheus.Histogram
	SessionsActive    prometheus.Gauge
	Peers             *prometheus.GaugeVec
	LastHandshake     *prometheus.GaugeVec
	Bytes             *prometheus.CounterVec
	Packets           *prometheus.CounterVec
	ReplayDropped     prometheus.Counter
	DecryptErrors     prometheus.Counter
	SpoofedPackets    *prometheus.CounterVec
	ClientConnected   *prometheus.GaugeVec
}

// New registers all metrics on the given registerer.
//
// A nil registerer creates the metrics without registering them, which is
// useful in tests.
func New(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		Up: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "w1nwire_up",
			Help: "One when the process is running and serving requests.",
		}),
		Handshakes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "w1nwire_handshakes_total",
			Help: "Handshakes by result.",
		}, []string{"result"}),
		HandshakeDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "w1nwire_handshake_duration_seconds",
			Help:    "Duration of a successful handshake.",
			Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5},
		}),
		SessionsActive: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "w1nwire_sessions_active",
			Help: "Number of established sessions.",
		}),
		Peers: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "w1nwire_peers_total",
			Help: "Number of peers by state.",
		}, []string{"state"}),
		LastHandshake: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "w1nwire_last_handshake_seconds",
			Help: "Seconds since the last successful handshake with a peer.",
		}, []string{"peer"}),
		Bytes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "w1nwire_bytes_total",
			Help: "Bytes transferred through the tunnel.",
		}, []string{"direction"}),
		Packets: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "w1nwire_packets_total",
			Help: "Packets transferred through the tunnel.",
		}, []string{"direction", "kind"}),
		ReplayDropped: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "w1nwire_replay_dropped_total",
			Help: "Packets dropped because they were replayed.",
		}),
		DecryptErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "w1nwire_decrypt_errors_total",
			Help: "Packets that failed authentication.",
		}),
		SpoofedPackets: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "w1nwire_spoofed_packets_total",
			Help: "Packets dropped because their source address did not belong to the peer.",
		}, []string{"peer"}),
		ClientConnected: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "w1nwire_client_connected",
			Help: "One when the client has an established session.",
		}, []string{"peer"}),
	}

	if reg != nil {
		reg.MustRegister(
			m.Up,
			m.Handshakes,
			m.HandshakeDuration,
			m.SessionsActive,
			m.Peers,
			m.LastHandshake,
			m.Bytes,
			m.Packets,
			m.ReplayDropped,
			m.DecryptErrors,
			m.SpoofedPackets,
			m.ClientConnected,
		)
	}
	return m
}

// Handler returns the HTTP handler that exposes the metrics.
func (m *Metrics) Handler() http.Handler {
	return promhttp.Handler()
}

// ObserveHandshake records the result and the duration of a handshake.
func (m *Metrics) ObserveHandshake(result string, d time.Duration) {
	m.Handshakes.WithLabelValues(result).Inc()
	if result == "success" {
		m.HandshakeDuration.Observe(d.Seconds())
	}
}
