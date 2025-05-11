package internal

import (
	"bufio"
	"encoding/json"
	"log"
	"strings"
	"time"

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
		switch string(ctx.Method()) {
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
		switch string(ctx.Method()) {
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
		argTopics(args),
		args.Peek("data"),
	)
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
