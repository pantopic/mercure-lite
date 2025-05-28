package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/logbn/expset"
	"github.com/logbn/mvfifo"
	"github.com/yosida95/uritemplate"
)

var (
	subscriptionTopic = "/.well-known/mercure/subscriptions/topic/subscriber"
	pingPeriod        = 30 * time.Second
)

type server struct {
	cache          *mvfifo.Cache
	cfg            Config
	clock          clock.Clock
	ctx            context.Context
	ctxCancel      context.CancelFunc
	done           chan bool
	httpClient     *http.Client
	hub            Hub
	metrics        *metrics
	mutex          sync.RWMutex
	pubJwksRefresh time.Duration
	pubKeys        []any
	pubKeysJwks    []any
	recentTopics   *expset.Set[string]
	server         *http.Server
	subJwksRefresh time.Duration
	subKeys        []any
	subKeysJwks    []any
}

func NewServer(cfg Config) *server {
	var m *metrics
	if len(cfg.METRICS) > 0 {
		m = NewMetrics(cfg.METRICS)
	}
	return &server{
		cache:        mvfifo.NewCache(mvfifo.WithMaxSizeBytes(max(cfg.CACHE_SIZE_MB, 16) << 20)),
		cfg:          cfg,
		clock:        clock.New(),
		httpClient:   &http.Client{Timeout: 5 * time.Second},
		hub:          newHubMulti(cfg.HUB_COUNT, m),
		metrics:      m,
		recentTopics: expset.New[string](),
	}
}

func (s *server) Start(ctx context.Context) (err error) {
	if s.ctx != nil {
		return
	}
	var maxage time.Duration
	s.ctx, s.ctxCancel = context.WithCancel(ctx)
	s.pubKeys = jwtKeys(s.cfg.PUBLISHER.JWT_ALG, s.cfg.PUBLISHER.JWT_KEY)
	s.pubKeysJwks, maxage = jwksKeys(s.httpClient, s.cfg.PUBLISHER.JWKS_URL)
	if len(s.allPubKeys()) == 0 {
		return fmt.Errorf("No publish keys available")
	}
	s.pubJwksRefresh = time.Duration(max(int(maxage), 60)) * time.Second
	s.subKeys = jwtKeys(s.cfg.SUBSCRIBER.JWT_ALG, s.cfg.SUBSCRIBER.JWT_KEY)
	s.subKeysJwks, maxage = jwksKeys(s.httpClient, s.cfg.SUBSCRIBER.JWKS_URL)
	if len(s.allSubKeys()) == 0 {
		return fmt.Errorf("No subscriber keys available")
	}
	s.subJwksRefresh = time.Duration(max(int(maxage), 60)) * time.Second
	s.done = make(chan bool)
	s.startJwksRefresh()
	go s.hub.Run(s.ctx)
	s.server = &http.Server{
		Addr:    s.cfg.LISTEN,
		Handler: s,
	}
	go func() {
		log.Printf("Starting server on %s", s.cfg.LISTEN)
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Error in ListenAndServe: %s", err)
		}
	}()
	go func() {
		select {
		case <-ctx.Done():
			s.Stop()
		case <-s.done:
		}
	}()
	s.metrics.Start(ctx)
	return nil
}

func (s *server) Stop() {
	if s.ctx == nil {
		return
	}
	close(s.done)
	timeout, cancel := context.WithTimeout(s.ctx, time.Second)
	defer cancel()
	s.server.Shutdown(timeout)
	s.metrics.Stop()
	s.ctxCancel()
	s.ctx = nil
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, "/.well-known/mercure") {
		w.WriteHeader(404)
		return
	}
	switch r.URL.Path {
	case "/.well-known/mercure":
		switch strings.ToUpper(r.Method) {
		case "POST":
			s.publish(w, r)
		case "OPTIONS":
			s.options(w, r)
		case "GET":
			s.subscribe(w, r)
		default:
			w.WriteHeader(405)
		}
	case "/.well-known/mercure/subscriptions":
		switch strings.ToUpper(r.Method) {
		case "GET":
			s.list(w, r)
		default:
			w.WriteHeader(405)
		}
	default:
		w.WriteHeader(404)
	}
}

func (s *server) publish(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	msg := newMessage(
		r.Form.Get("type"),
		s.verifyPublish(r, r.Form["topic"]),
		r.Form.Get("data"),
	)
	if len(msg.Topics) == 0 {
		w.WriteHeader(403)
		return
	}
	for _, topic := range msg.Topics {
		if s.recentTopics.Has(topic) {
			s.cache.Add(topic, msg.timestamp(), msg.ToJson())
		}
	}
	s.hub.Broadcast(msg)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(msg.ID))
	s.metrics.Publish()
}

func (s *server) options(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", s.cfg.CORS_ORIGINS)
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Last-Event-ID, Cache-Control")
	w.Header().Set("Access-Control-Allow-Credentials", "true")
}

func (s *server) subscribe(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	topics := r.Form["topic"]
	topics, jwtExpires := s.verifySubscribe(r, topics)
	if len(topics) < 1 {
		return
	}
	if jwtExpires < 1 {
		jwtExpires = time.Hour * 1e6
	}
	topics, err := s.normalize(topics)
	if err != nil {
		log.Print(err)
		w.WriteHeader(400)
		return
	}
	for _, topic := range topics {
		s.recentTopics.Add(topic, time.Hour)
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("Access-Control-Allow-Origin", s.cfg.CORS_ORIGINS)
	w.Header().Set("Access-Control-Allow-Headers", "Authorization")
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	lastEventID := r.Header.Get("Last-Event-ID")
	lastEventCursor := msgIDtimestamp(lastEventID)
	if lastEventCursor > 0 {
		var msg = &message{}
		for _, topic := range topics {
			for _, data := range s.cache.IterAfter(topic, lastEventCursor) {
				msg.FromJson(data)
				msg.WriteTo(w)
			}
		}
	}
	if _, err := w.Write([]byte(":\n")); err != nil {
		return
	}
	conn := newConnection(topics)
	conn.Announce(s.hub, true)
	s.hub.Register(conn)
	defer s.hub.Unregister(conn)
	defer conn.Announce(s.hub, false)
	defer s.metrics.Disconnect()
	defer s.metrics.Unsubscribe(len(conn.topics))
	s.metrics.Connect()
	s.metrics.Subscribe(len(conn.topics))
	flush := w.(http.Flusher).Flush
	flush()
	ping := time.NewTicker(pingPeriod)
	defer ping.Stop()
	var last string
	for {
		select {
		case msg, ok := <-conn.send:
			if !ok {
				return
			}
			if msg.ID == last {
				break
			}
			if _, err := msg.WriteTo(w); err != nil {
				return
			}
			flush()
			last = msg.ID
			s.metrics.Send()
		case <-ping.C:
			if _, err := w.Write([]byte(":\n")); err != nil {
				return
			}
			flush()
			for _, topic := range topics {
				s.recentTopics.Add(topic, time.Hour)
			}
		case <-r.Context().Done():
			return
		case <-s.clock.After(jwtExpires):
			return
		}
	}
}

func (s *server) normalize(topics []string) ([]string, error) {
	for i := range topics {
		t, err := uritemplate.New(topics[i])
		if err != nil {
			return topics, fmt.Errorf("Invalid topic: %s", topics[i])
		}
		if t.Match(subscriptionTopic) != nil {
			topics[i] = subscriptionTopic
		}
	}
	return topics, nil
}

func (s *server) verifySubscribe(r *http.Request, topics []string) (res []string, jwtExpires time.Duration) {
	claims := jwtTokenClaims(r, s.allSubKeys(), s.cfg.DEBUG)
	if claims == nil {
		return
	}
	if claims.RegisteredClaims.ExpiresAt != nil {
		jwtExpires = s.clock.Until(claims.RegisteredClaims.ExpiresAt.Truncate(time.Second))
	}
	all := slices.Contains(claims.Mercure.Subscribe, "*")
	for _, t := range topics {
		if all || slices.Contains(claims.Mercure.Subscribe, t) {
			res = append(res, t)
		}
	}
	return
}

func (s *server) verifyPublish(r *http.Request, topics []string) (res []string) {
	claims := jwtTokenClaims(r, s.allPubKeys(), s.cfg.DEBUG)
	if claims == nil {
		return
	}
	all := slices.Contains(claims.Mercure.Publish, "*")
	for _, t := range topics {
		if all || slices.Contains(claims.Mercure.Publish, t) {
			res = append(res, t)
		}
	}
	return
}

func (s *server) list(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/ld+json")
	list := subscriptionList{
		Context:     "github.com/pantopic/mercure-lite",
		ID:          subscriptionTopic,
		Type:        "Subscriptions",
		LastEventID: uuidv7(),
	}
	for c := range s.hub.Connections() {
		for _, topic := range c.topics {
			list.Subscriptions = append(list.Subscriptions, c.toSubscription(topic, true))
		}
	}
	b, _ := json.Marshal(list)
	w.Write(b)
}

func (s *server) allPubKeys() []any {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return append(s.pubKeys, s.pubKeysJwks...)
}

func (s *server) allSubKeys() []any {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return append(s.subKeys, s.subKeysJwks...)
}

func (s *server) startJwksRefresh() {
	if s.subJwksRefresh > 0 {
		go func() {
			t := s.clock.Ticker(s.subJwksRefresh)
			defer t.Stop()
			for {
				select {
				case <-t.C:
					keys, maxage := jwksKeys(s.httpClient, s.cfg.SUBSCRIBER.JWKS_URL)
					if maxage != s.subJwksRefresh && maxage > 0 {
						s.subJwksRefresh = maxage * time.Second
						t.Reset(s.subJwksRefresh)
					}
					if len(keys) < 1 {
						break
					}
					s.mutex.Lock()
					s.subKeysJwks = keys
					s.mutex.Unlock()
				case <-s.done:
					return
				}
			}
		}()
	}
	if s.pubJwksRefresh > 0 {
		go func() {
			t := s.clock.Ticker(s.pubJwksRefresh)
			defer t.Stop()
			for {
				select {
				case <-t.C:
					keys, maxage := jwksKeys(s.httpClient, s.cfg.PUBLISHER.JWKS_URL)
					if maxage != s.pubJwksRefresh && maxage > 0 {
						s.pubJwksRefresh = maxage * time.Second
						t.Reset(s.pubJwksRefresh)
					}
					if len(keys) < 1 {
						break
					}
					s.mutex.Lock()
					s.pubKeysJwks = keys
					s.mutex.Unlock()
				case <-s.done:
					return
				}
			}
		}()
	}
}
