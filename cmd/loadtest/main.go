package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/r3labs/sse"
)

var (
	ctx    = context.Background()
	client = &http.Client{Timeout: time.Second}
	target = flag.String("target", "http://localhost:8001", "Target")
	subs   = flag.Int("s", 256, "Number of concurrent subscribers")
	pubs   = flag.Int("c", 16, "Number of concurrent publishers")
	msgs   = flag.Int("n", 10000, "Number of requests")
	parity = flag.Bool("parity", false, "Parity")

	pubKeyHS256 = `512caae005bf589fb4d7728301205db273d55aa5030a2ab6e2acb2955063b6f1`
	subKeyHS256 = `56500e38ddc0360f0525d7545ba708d1b873aedcc2c5caca1c8077f398b2d409`

	wgPub sync.WaitGroup
	wgSub sync.WaitGroup
	err   error
)

func main() {
	flag.Parse()
	if *parity {
		*target = "http://localhost:8002"
	}
	var (
		cancel      context.CancelFunc
		publishers  []*publisher
		subscribers []*subscriber
		topics      = make([]string, *subs)
	)
	ctx, cancel = context.WithCancel(ctx)

	wgSub.Add(*subs)
	log.Printf("Starting %d subscribers", *subs)
	for i := range *subs {
		topics[i] = uuidv4()
		token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"mercure": map[string]any{
				"subscribe": []string{topics[i]},
			},
		}).SignedString([]byte(subKeyHS256))
		if err != nil {
			log.Fatal(err)
		}
		s := new(subscriber)
		subscribers = append(subscribers, s)
		s.Start(token, []string{topics[i]})
	}
	wgPub.Add(*pubs)
	log.Printf("Starting %d publishers", *pubs)
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"mercure": map[string]any{
			"publish": []string{"*"},
		},
	}).SignedString([]byte(pubKeyHS256))
	if err != nil {
		log.Fatal(err)
	}
	var jobs = make(chan string, *pubs)
	start := time.Now()
	for range *pubs {
		p := new(publisher)
		p.Start(jobs, token)
		publishers = append(publishers, p)
	}
	log.Printf("Sending %d messages", *msgs)
	for range *msgs {
		jobs <- topics[rand.Intn(*subs)]
	}

	// Close publishers
	close(jobs)
	wgPub.Wait()
	var sent int
	for _, p := range publishers {
		sent += p.sent
	}

	// Close subscribers
	cancel()
	wgSub.Wait()
	var received int
	for _, s := range subscribers {
		received += s.received
	}

	// Print results
	log.Printf("%d sent, %d received in %v", sent, received, time.Since(start))
}

type subscriber struct {
	received int
}

func (w *subscriber) Start(token string, topics []string) {
	events := make(chan *sse.Event)
	sseClient := sse.NewClient(*target + "/.well-known/mercure?topic=" + strings.Join(topics, "&topic="))
	sseClient.Headers["Authorization"] = "Bearer " + token
	if err := sseClient.SubscribeChanRawWithContext(ctx, events); err != nil {
		log.Fatal(err)
	}
	go func() {
		defer wgSub.Done()
		for {
			select {
			case <-events:
				w.received++
			case <-ctx.Done():
				return
			}
		}
	}()
}

type publisher struct {
	sent int
}

func (w *publisher) Start(topics chan string, token string) {
	go func() {
		defer wgPub.Done()
		for topic := range topics {
			req, _ := http.NewRequest("POST", *target+"/.well-known/mercure", strings.NewReader(url.Values{
				"data":  {"test-data"},
				"topic": {topic},
			}.Encode()))
			req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Add("Authorization", "Bearer "+token)
			_, err := client.Do(req)
			if err != nil {
				log.Fatalf("Publish error: %v", err)
			}
			w.sent++
		}
	}()
}

func uuidv4() string {
	uuid, _ := uuid.NewV4()
	return fmt.Sprintf("urn:uuid:%s", uuid)
}
