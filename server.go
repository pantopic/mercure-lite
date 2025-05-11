package mercurelite

import (
	"bufio"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/valyala/fasthttp"
)

type server struct {
	cfg Config
}

func NewServer(cfg Config) *server {
	return &server{cfg: cfg}
}

type Config struct {
	LISTEN             string
	PUBLISHER_JWT_KEY  string
	PUBLISHER_JWT_ALG  string
	SUBSCRIBER_JWT_KEY string
	SUBSCRIBER_JWT_ALG string
	CORS_ORIGINS       string
}

func (s *server) Start() {
	log.Printf("Listening on %s", s.cfg.LISTEN)
	if err := fasthttp.ListenAndServe(s.cfg.LISTEN, s.HandleFastHTTP); err != nil {
		log.Fatalf("Error in ListenAndServe: %s", err)
	}
}

func (s *server) HandleFastHTTP(ctx *fasthttp.RequestCtx) {
	var (
		uri  = ctx.Request.URI()
		path = string(uri.Path())
	)
	if !strings.HasPrefix(path, "/.well-known/mercure") {

	}
	switch {
	case strings.HasPrefix(path, "/.well-known/mercure/subscriptions"):
	}
	ctx.SetContentType("text/event-stream")
	ctx.Response.Header.Set("Cache-Control", "no-cache")
	ctx.Response.Header.Set("Connection", "keep-alive")
	ctx.Response.Header.Set("Transfer-Encoding", "chunked")
	ctx.Response.Header.Set("Access-Control-Allow-Origin", s.cfg.CORS_ORIGINS)
	ctx.Response.Header.Set("Access-Control-Allow-Headers", "Cache-Control")
	ctx.Response.Header.Set("Access-Control-Allow-Credentials", "true")
	ctx.SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		var i int
		for {
			i++
			fmt.Fprintf(w, "data: Message: %d - %v\n\n", i, time.Now())
			if err := w.Flush(); err != nil {
				log.Printf("Flush error: %s", err.Error())
				return
			}
			time.Sleep(time.Second)
		}
	}))
}
