package internal

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/golang-jwt/jwt/v5"
	"github.com/lestrrat-go/httpcc"
	"github.com/lestrrat-go/jwx/v2/jwk"
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
	return &server{
		cfg:        cfg,
		clock:      clock.New(),
		httpClient: &http.Client{Timeout: 5 * time.Second},
		hub:        newHub(),
	}
}

func (s *server) Start(ctx context.Context) (err error) {
	if s.ctx != nil {
		return
	}
	s.ctx, s.ctxCancel = context.WithCancel(ctx)
	s.pubKeys = s.getJwtKeys(s.cfg.PUBLISHER.JWT_ALG, s.cfg.PUBLISHER.JWT_KEY)
	s.pubKeysJwks, s.pubJwksRefresh = s.getJwksKeys(s.cfg.PUBLISHER.JWKS_URL)
	if len(s.allPubKeys()) == 0 {
		return fmt.Errorf("No publish keys available")
	}
	s.subKeys = s.getJwtKeys(s.cfg.SUBSCRIBER.JWT_ALG, s.cfg.SUBSCRIBER.JWT_KEY)
	s.subKeysJwks, s.subJwksRefresh = s.getJwksKeys(s.cfg.SUBSCRIBER.JWKS_URL)
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
		log.Printf("Listening on %s", s.cfg.LISTEN)
		if err := s.server.ListenAndServe(s.cfg.LISTEN); err != nil {
			log.Fatalf("Error in ListenAndServe: %s", err)
			s.Stop()
		}
	}()
	go func() {
		select {
		case <-ctx.Done():
			s.Stop()
		case <-s.done:
		}
	}()
	return nil
}
func (s *server) Stop() {
	close(s.done)
	s.server.Shutdown()
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
	// Normalize subscription notification topics
	for i := range topics {
		t, err := uritemplate.New(topics[i])
		if err != nil {
			log.Printf("Invalid topic: %s", topics[i])
			ctx.SetStatusCode(400)
			return
		}
		if t.Match(subscriptionTopic) != nil {
			topics[i] = subscriptionTopic
		}
	}
	ctx.SetContentType("text/event-stream")
	ctx.Response.Header.Set("Cache-Control", "no-cache")
	ctx.Response.Header.Set("Connection", "keep-alive")
	ctx.Response.Header.Set("Transfer-Encoding", "chunked")
	ctx.Response.Header.Set("Access-Control-Allow-Origin", s.cfg.CORS_ORIGINS)
	ctx.Response.Header.Set("Access-Control-Allow-Headers", "Cache-Control")
	ctx.Response.Header.Set("Access-Control-Allow-Credentials", "true")
	conn := newConnection(topics)
	conn.Broadcast(s.hub, true)
	s.hub.Register(conn)
	ctx.SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		defer s.hub.Unregister(conn)
		defer conn.Broadcast(s.hub, false)
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
			case <-pinger.C:
				w.Write([]byte(":\n"))
				if err := w.Flush(); err != nil {
					return
				}
			}
		}
	}))
}

func (s *server) verifySubscribe(ctx *fasthttp.RequestCtx, topics []string) (res []string) {
	claims := s.getTokenClaims(ctx, s.allSubKeys())
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
	claims := s.getTokenClaims(ctx, s.allPubKeys())
	if claims == nil {
		return
	}
	for _, t := range topics {
		if slices.Contains(claims.Mercure.Publish, t) {
			res = append(res, t)
		}
	}
	return
}

type tokenClaims struct {
	Mercure struct {
		Publish   []string `json:"publish"`
		Subscribe []string `json:"subscribe"`
	} `json:"mercure"`
	jwt.RegisteredClaims
}

func (s *server) getTokenClaims(ctx *fasthttp.RequestCtx, keys []any) *tokenClaims {
	tokenStr := string(ctx.Request.Header.Peek("Authorization"))
	if parts := strings.Split(tokenStr, " "); len(parts) == 2 {
		tokenStr = parts[1]
	} else {
		tokenStr = string(ctx.Request.Header.Cookie("mercureAuthorization"))
	}
	if tokenStr == "" {
		return nil
	}
	claims := new(tokenClaims)
	var token *jwt.Token
	var err error
	for _, k := range keys {
		token, err = jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (any, error) {
			return k, nil
		})
		if err != nil {
			continue
		}
		if token.Valid {
			break
		}
	}
	if !token.Valid {
		log.Printf("Invalid token: %v", err)
		return nil
	}
	return token.Claims.(*tokenClaims)
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

var (
	algECDSA  = []string{"ES256", "ES384", "ES512"}
	algHMAC   = []string{"HS256", "HS384", "HS512"}
	algRSA    = []string{"RS256", "RS384", "RS512"}
	algRSAPSS = []string{"PS256", "PS384", "PS512"}

	algEdDSA = []string{"EdDSA"}
)

func (s *server) getJwtKeys(alg, key string) (keys []any) {
	if alg == "" {
		return
	}
	if slices.Contains(algHMAC, alg) {
		for k := range bytes.SplitSeq([]byte(key), []byte("\n")) {
			keys = append(keys, k)
		}
		return
	}
	if slices.Contains(algECDSA, alg) ||
		slices.Contains(algRSA, alg) ||
		slices.Contains(algRSAPSS, alg) {
		var block *pem.Block
		rest := []byte(key)
		var i int
		for len(rest) > 0 {
			i++
			block, rest = pem.Decode(rest)
			if block == nil {
				log.Printf("Unable to decode %s block #%d", alg, i)
				return
			}
			pubInterface, err := x509.ParsePKIXPublicKey(block.Bytes)
			if err != nil {
				log.Printf("Unable to parse key %s #%d", alg, i)
				return
			}
			switch alg[:2] {
			case "ES":
				keys = append(keys, pubInterface.(*ecdsa.PublicKey))
			case "RS":
				keys = append(keys, pubInterface.(*rsa.PublicKey))
			case "PS":
				keys = append(keys, pubInterface.(*rsa.PublicKey))
			}
		}
		return
	}
	if slices.Contains(algEdDSA, alg) {
		log.Println("EdDSA key alg not supported")
		return
	}
	log.Printf("Unrecognized key alg: %s", alg)
	return
}

func (s *server) getJwksKeys(url string) (keys []any, maxage time.Duration) {
	if len(url) > 0 {
		resp, err := s.httpClient.Get(url)
		if err != nil {
			log.Println(err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			log.Printf("JWKS URL returned %d", resp.StatusCode)
			return
		}
		var jwks = map[string][]any{}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Println(err)
			return
		}
		if err = json.Unmarshal(body, &jwks); err != nil {
			log.Println(err)
			return
		}
		for _, k := range jwks["keys"] {
			kjson, err := json.Marshal(k)
			if err != nil {
				log.Println(err)
				return
			}
			if err := jwk.ParseRawKey(kjson, &k); err != nil {
				log.Println(err)
				return
			}
			keys = append(keys, k)
		}
		maxage = 3600
		directives, err := httpcc.ParseResponse(resp.Header.Get(`Cache-Control`))
		if err != nil {
			return
		}
		if val, present := directives.MaxAge(); present {
			maxage = time.Duration(max(int(val), 60))
		}
	}
	return
}

func (s *server) startJwksRefresh() {
	if s.subJwksRefresh > 0 {
		go func() {
			t := s.clock.Ticker(s.subJwksRefresh)
			defer t.Stop()
			select {
			case <-t.C:
				keys, maxage := s.getJwksKeys(s.cfg.SUBSCRIBER.JWKS_URL)
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
				keys, maxage := s.getJwksKeys(s.cfg.PUBLISHER.JWKS_URL)
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
