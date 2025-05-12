package internal

import (
	"bufio"
	"encoding/json"
	"log"
	"slices"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/valyala/fasthttp"
	"github.com/yosida95/uritemplate"
)

var (
	subscriptionTopic = "/.well-known/mercure/subscriptions/topic/subscriber"
	pingPeriod        = 30 * time.Second
)

type server struct {
	cfg Config
	hub Hub
}

func NewServer(cfg Config) *server {
	return &server{cfg: cfg, hub: newHub()}
}

func (s *server) Start() {
	go s.hub.Run()
	log.Printf("Listening on %s", s.cfg.LISTEN)
	if err := fasthttp.ListenAndServe(s.cfg.LISTEN, s.handleFastHTTP); err != nil {
		log.Fatalf("Error in ListenAndServe: %s", err)
	}
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
		s.verifySubscribe(ctx, argTopics(args)),
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
	claims := s.getTokenClaims(ctx, s.cfg.SUBSCRIBER_JWT_KEY)
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
	claims := s.getTokenClaims(ctx, s.cfg.PUBLISHER_JWT_KEY)
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

type tokenClaims struct {
	Mercure struct {
		Publish   []string `json:"publish"`
		Subscribe []string `json:"subscribe"`
	} `json:"mercure"`
	jwt.RegisteredClaims
}

func (s *server) getTokenClaims(ctx *fasthttp.RequestCtx, key string) *tokenClaims {
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
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (any, error) {
		return []byte(key), nil
	})
	if err != nil {
		log.Println("Error parsing token: %s", err)
		return nil
	}
	if !token.Valid {
		log.Println("Invalid token")
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
