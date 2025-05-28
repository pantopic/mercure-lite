package internal

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/logbn/mvfifo"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type metrics struct {
	cache  *mvfifo.Cache
	ctx    context.Context
	listen string
	server *http.Server

	connections_active     prometheus.Gauge
	connections_terminated prometheus.Counter
	connections_total      prometheus.Counter
	message_cache_age      prometheus.Gauge
	message_cache_count    prometheus.Gauge
	message_cache_size     prometheus.Gauge
	messages_published     prometheus.Counter
	messages_sent          prometheus.Counter
	subscriptions_active   prometheus.Gauge
	subscriptions_total    prometheus.Counter
}

func NewMetrics(listen string, cache *mvfifo.Cache) *metrics {
	return &metrics{listen: listen, cache: cache}
}

func (m *metrics) Start(ctx context.Context) {
	if m == nil || m.listen == "" {
		return
	}
	log.Printf("Starting metrics on %s", m.listen)
	m.server = &http.Server{
		Addr:    m.listen,
		Handler: promhttp.Handler(),
	}
	m.init()
	go func() {
		if err := m.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		select {
		case <-t.C:
			cur, _ := m.cache.First()
			m.message_cache_age.Set(float64((cur*100 - uint64(time.Now().UnixNano())) / uint64(time.Second)))
			m.message_cache_count.Set(float64(m.cache.Len()))
			m.message_cache_size.Set(float64(m.cache.Size()))
		case <-ctx.Done():
			m.Stop()
			return
		}
	}()
}

func (m *metrics) Stop() {
	if m != nil {
		m.server.Shutdown(m.ctx)
	}
}
func (m *metrics) Connect() {
	if m != nil {
		m.connections_total.Inc()
		m.connections_active.Inc()
	}
}
func (m *metrics) Disconnect() {
	if m != nil {
		m.connections_active.Dec()
	}
}
func (m *metrics) Terminate() {
	if m != nil {
		m.connections_terminated.Inc()
	}
}
func (m *metrics) Subscribe(n int) {
	if m != nil {
		m.subscriptions_total.Add(float64(n))
		m.subscriptions_active.Add(float64(n))
	}
}
func (m *metrics) Unsubscribe(n int) {
	if m != nil {
		m.subscriptions_active.Sub(float64(n))
	}
}
func (m *metrics) Publish() {
	if m != nil {
		m.messages_published.Inc()
	}
}
func (m *metrics) Send() {
	if m != nil {
		m.messages_sent.Inc()
	}
}

func (m *metrics) init() {
	m.connections_active = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "mercure_lite_connections_active",
		Help: "Number of active connections",
	})
	m.connections_total = promauto.NewCounter(prometheus.CounterOpts{
		Name: "mercure_lite_connections_total",
		Help: "Total number of connections created",
	})
	m.connections_terminated = promauto.NewCounter(prometheus.CounterOpts{
		Name: "mercure_lite_connections_terminated",
		Help: "Total number of connections terminated",
	})
	m.message_cache_age = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "mercure_lite_message_cache_age",
		Help: "Age of oldest message in the cache",
	})
	m.message_cache_count = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "mercure_lite_message_cache_count",
		Help: "Number of messages presently stored in the cache",
	})
	m.message_cache_size = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "mercure_lite_message_cache_size",
		Help: "Number of bytes presently stored in the cache",
	})
	m.messages_published = promauto.NewCounter(prometheus.CounterOpts{
		Name: "mercure_lite_messages_published",
		Help: "Total number of messages published",
	})
	m.messages_sent = promauto.NewCounter(prometheus.CounterOpts{
		Name: "mercure_lite_messages_sent",
		Help: "Total number of messages sent",
	})
	m.subscriptions_active = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "mercure_lite_subscriptions_active",
		Help: "Number of active subsriptions",
	})
	m.subscriptions_total = promauto.NewCounter(prometheus.CounterOpts{
		Name: "mercure_lite_subscriptions_total",
		Help: "Total number of subscriptions created",
	})
}
