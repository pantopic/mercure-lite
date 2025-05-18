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
	"github.com/tmaxmax/go-sse"
)

var (
	ctx    = context.Background()
	client = &http.Client{Timeout: 5 * time.Second}
	target = flag.String("target", "http://localhost:8001", "Target")
	parity = flag.Bool("parity", false, "Parity target")
	subs   = flag.Int("s", 256, "Number of concurrent subscribers")
	pubs   = flag.Int("c", 16, "Number of concurrent publishers")
	msgs   = flag.Int("n", 10000, "Number of requests")

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
		jobs        = make(chan string, 256)
	)
	ctx, cancel = context.WithCancel(ctx)
	// subscribers
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

	time.Sleep(2 * time.Second)

	// publishers
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
	for range *pubs {
		p := new(publisher)
		p.Start(jobs, token)
		publishers = append(publishers, p)
	}

	time.Sleep(time.Second)

	// messages
	start := time.Now()
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

	t := time.Since(start)

	// Close subscribers
	cancel()
	wgSub.Wait()
	var received int
	for _, s := range subscribers {
		received += s.received
	}

	// Print results
	log.Printf("%d sent, %d received in %v (%.2f msgs/sec)", sent, received, t.Truncate(time.Millisecond), float64(sent)/(float64(t)/float64(time.Second)))
}

type subscriber struct {
	received int
}

func (w *subscriber) Start(token string, topics []string) {
	events := make(chan sse.Event)
	url := *target + "/.well-known/mercure?topic=" + strings.Join(topics, "&topic=")
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	conn := sse.NewConnection(req)
	go func() {
		if err := conn.Connect(); err != nil && err != context.Canceled {
			log.Println(err)
		}
	}()
	conn.SubscribeToAll(func(evt sse.Event) {
		events <- evt
	})
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

func (w *publisher) Start(jobs chan string, token string) {
	go func() {
		defer wgPub.Done()
		for topic := range jobs {
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
