package internal

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sync"
)

var connectionPool = sync.Pool{
	New: func() any {
		return &connection{}
	},
}

func newConnection(topics []string) (c *connection) {
	c = connectionPool.Get().(*connection)
	c.id = uuidv7()
	c.topics = topics
	c.closed = false
	c.send = make(chan *message, 256)
	return
}

type connection struct {
	id     string
	send   chan *message
	topics []string
	closed bool
}

func (c *connection) Announce(h Hub, active bool) {
	for _, topic := range c.topics {
		b, _ := json.Marshal(c.toSubscription(topic, active))
		h.Broadcast(newMessage(
			"Subscription",
			[]string{subscriptionTopic},
			string(b),
		))
	}
}

func (c *connection) close() bool {
	if c.closed {
		return false
	}
	c.closed = true
	close(c.send)
	connectionPool.Put(c)
	return true
}

func (c *connection) toSubscription(topic string, active bool) subscription {
	return subscription{
		ID:         fmt.Sprintf("/.well-known/mercure/subscriptions/%s/%s", url.QueryEscape(topic), url.QueryEscape(c.id)),
		Type:       "Subscription",
		Topic:      topic,
		Subscriber: c.id,
		Active:     active,
		Payload:    make(map[string]any),
	}
}
