package internal

import (
	"bufio"
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
	"github.com/valyala/fasthttp"
	"github.com/yosida95/uritemplate"
)

var (
	subscriptionTopic = "/.well-known/mercure/subscriptions/topic/subscriber"
	pingPeriod        = 30 * time.Second
)

type server struct {
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
	server         *fasthttp.Server
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
		cfg:        cfg,
		clock:      clock.New(),
		httpClient: &http.Client{Timeout: 5 * time.Second},
		metrics:    m,
		hub:        newHub(m),
	}
}

func (s *server) Start(ctx context.Context) (err error) {
	if s.ctx != nil {
		return
	}
	s.ctx, s.ctxCancel = context.WithCancel(ctx)
	s.pubKeys = jwtKeys(s.cfg.PUBLISHER.JWT_ALG, s.cfg.PUBLISHER.JWT_KEY)
	s.pubKeysJwks, s.pubJwksRefresh = jwksKeys(s.httpClient, s.cfg.PUBLISHER.JWKS_URL)
	if len(s.allPubKeys()) == 0 {
		return fmt.Errorf("No publish keys available")
	}
	s.subKeys = jwtKeys(s.cfg.SUBSCRIBER.JWT_ALG, s.cfg.SUBSCRIBER.JWT_KEY)
	s.subKeysJwks, s.subJwksRefresh = jwksKeys(s.httpClient, s.cfg.SUBSCRIBER.JWKS_URL)
	if len(s.allSubKeys()) == 0 {
		return fmt.Errorf("No subscriber keys available")
	}
	s.done = make(chan bool)
	s.startJwksRefresh()
	go s.hub.Run(s.ctx)
	s.server = &fasthttp.Server{
		Handler: s.handleFastHTTP,
	}
	go func() {
		log.Printf("Starting server on %s", s.cfg.LISTEN)
		if err := s.server.ListenAndServe(s.cfg.LISTEN); err != nil {
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
	close(s.done)
	timeout, cancel := context.WithTimeout(s.ctx, time.Second)
	defer cancel()
	s.server.ShutdownWithContext(timeout)
	s.metrics.Stop()
	s.ctxCancel()
	s.ctx = nil
}

func (s *server) handleFastHTTP(ctx *fasthttp.RequestCtx) {
	var (
		uri  = ctx.Request.URI()
		path = string(uri.Path())
	)
	if !strings.HasPrefix(path, "/.well-known/mercure") {
		ctx.SetStatusCode(404)
		return
	}
	switch path {
	case "/.well-known/mercure":
		switch strings.ToUpper(string(ctx.Method())) {
		case "POST":
			s.publish(ctx)
		case "OPTIONS":
			s.options(ctx)
		case "GET":
			s.subscribe(ctx)
		default:
			ctx.SetStatusCode(405)
		}
	case "/.well-known/mercure/subscriptions":
		switch strings.ToUpper(string(ctx.Method())) {
		case "GET":
			s.list(ctx)
		default:
			ctx.SetStatusCode(405)
		}
	default:
		ctx.SetStatusCode(404)
	}
}

func (s *server) publish(ctx *fasthttp.RequestCtx) {
	args := ctx.PostArgs()
	msg := newMessage(
		string(args.Peek("type")),
		s.verifyPublish(ctx, argTopics(args)),
		args.Peek("data"),
	)
	if len(msg.Topics) == 0 {
		ctx.SetStatusCode(403)
		return
	}
	s.hub.Broadcast(msg)
	ctx.SetContentType("application/ld+json")
	ctx.Write([]byte(msg.ID))
	s.metrics.Publish()
}

func (s *server) options(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Access-Control-Allow-Origin", s.cfg.CORS_ORIGINS)
	ctx.Response.Header.Set("Access-Control-Allow-Headers", "Cache-Control")
	ctx.Response.Header.Set("Access-Control-Allow-Credentials", "true")
}

func (s *server) subscribe(ctx *fasthttp.RequestCtx) {
	topics := argTopics(ctx.Request.URI().QueryArgs())
	topics = s.verifySubscribe(ctx, topics)
	if len(topics) < 1 {
		return
	}
	topics, err := s.normalize(topics)
	if err != nil {
		log.Print(err)
		ctx.SetStatusCode(400)
		return
	}
	ctx.SetContentType("text/event-stream")
	ctx.Response.Header.Set("Cache-Control", "no-cache")
	ctx.Response.Header.Set("Connection", "keep-alive")
	ctx.Response.Header.Set("Transfer-Encoding", "chunked")
	ctx.Response.Header.Set("Access-Control-Allow-Origin", s.cfg.CORS_ORIGINS)
	ctx.Response.Header.Set("Access-Control-Allow-Headers", "Cache-Control")
	ctx.Response.Header.Set("Access-Control-Allow-Credentials", "true")
	conn := newConnection(topics)
	conn.Announce(s.hub, true)
	s.hub.Register(conn)
	ctx.SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		defer s.hub.Unregister(conn)
		defer conn.Announce(s.hub, false)
		defer s.metrics.Disconnect()
		defer s.metrics.Unsubscribe(len(conn.topics))
		w.Write([]byte(":\n"))
		if err := w.Flush(); err != nil {
			return
		}
		pinger := time.NewTicker(pingPeriod)
		defer pinger.Stop()
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
				msg.WriteTo(w)
				if err := w.Flush(); err != nil {
					return
				}
				last = msg.ID
				s.metrics.Send()
			case <-pinger.C:
				w.Write([]byte(":\n"))
				if err := w.Flush(); err != nil {
					return
				}
			}
		}
	}))
	s.metrics.Connect()
	s.metrics.Subscribe(len(conn.topics))
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

func (s *server) verifySubscribe(ctx *fasthttp.RequestCtx, topics []string) (res []string) {
	claims := jwtTokenClaims(ctx, s.allSubKeys())
	if claims == nil {
		return
	}
	for _, t := range topics {
		if slices.Contains(claims.Mercure.Subscribe, t) {
			res = append(res, t)
		}
	}
	return
}

func (s *server) verifyPublish(ctx *fasthttp.RequestCtx, topics []string) (res []string) {
	claims := jwtTokenClaims(ctx, s.allPubKeys())
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

func (s *server) list(ctx *fasthttp.RequestCtx) {
	ctx.SetContentType("application/ld+json")
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
	ctx.Write(b)
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
			select {
			case <-t.C:
				keys, maxage := jwksKeys(s.httpClient, s.cfg.SUBSCRIBER.JWKS_URL)
				if maxage != s.subJwksRefresh && maxage > 0 {
					s.subJwksRefresh = maxage
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
		}()
	}
	if s.pubJwksRefresh > 0 {
		go func() {
			t := s.clock.Ticker(s.pubJwksRefresh)
			defer t.Stop()
			select {
			case <-t.C:
				keys, maxage := jwksKeys(s.httpClient, s.cfg.PUBLISHER.JWKS_URL)
				if maxage != s.pubJwksRefresh && maxage > 0 {
					s.pubJwksRefresh = maxage
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
		}()
	}
}
