package internal

import (
	"encoding/json"
	"fmt"
	"net/url"
)

func newConnection(topics []string) *connection {
	return &connection{
		id:     uuidv7(),
		topics: topics,
		send:   make(chan message, 256),
	}
}

type connection struct {
	id     string
	send   chan message
	topics []string
	closed bool
}

func (c connection) Announce(h Hub, active bool) {
	for _, topic := range c.topics {
		b, _ := json.Marshal(c.toSubscription(topic, active))
		h.Broadcast(newMessage(
			"Subscription",
			[]string{subscriptionTopic},
			b,
		))
	}
}

func (c connection) close() bool {
	if c.closed {
		return false
	}
	c.closed = true
	close(c.send)
	return true
}

func (c connection) toSubscription(topic string, active bool) subscription {
	return subscription{
		ID:         fmt.Sprintf("/.well-known/mercure/subscriptions/%s/%s", url.QueryEscape(topic), url.QueryEscape(c.id)),
		Type:       "Subscription",
		Topic:      topic,
		Subscriber: c.id,
		Active:     active,
		Payload:    make(map[string]any),
	}
}
