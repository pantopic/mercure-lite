package internal

import (
	"context"
	"maps"
	"sync"
)

type Hub interface {
	Run(context.Context)
	Register(*connection)
	Unregister(*connection)
	Broadcast(message)
	Connections() map[*connection]bool
}

type hub struct {
	metrics       *metrics
	subscriptions map[string]map[*connection]bool
	register      chan *connection
	unregister    chan *connection
	broadcast     chan message

	mutex sync.RWMutex
}

func newHub(m *metrics) *hub {
	return &hub{
		metrics:       m,
		subscriptions: make(map[string]map[*connection]bool),
		register:      make(chan *connection),
		unregister:    make(chan *connection),
		broadcast:     make(chan message),
	}
}

func (h *hub) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case conn := <-h.register:
			h.mutex.Lock()
			for _, topic := range conn.topics {
				if _, ok := h.subscriptions[topic]; !ok {
					h.subscriptions[topic] = make(map[*connection]bool)
				}
				h.subscriptions[topic][conn] = true
			}
			h.mutex.Unlock()
		case conn := <-h.unregister:
			h.mutex.Lock()
			for _, topic := range conn.topics {
				delete(h.subscriptions[topic], conn)
			}
			h.mutex.Unlock()
			conn.closed = true
			close(conn.send)
		case msg := <-h.broadcast:
			h.mutex.RLock()
			for _, t := range msg.Topics {
				if connections, ok := h.subscriptions[t]; ok {
					for conn := range connections {
						select {
						case conn.send <- msg:
						default:
							if conn.close() {
								h.metrics.Terminate()
							}
						}
					}
				}
			}
			h.mutex.RUnlock()
		}
	}
}

func (h *hub) Broadcast(msg message) {
	h.broadcast <- msg
}

func (h *hub) Register(conn *connection) {
	h.register <- conn
}

func (h *hub) Unregister(conn *connection) {
	h.unregister <- conn
}

func (h *hub) Connections() map[*connection]bool {
	m2 := make(map[*connection]bool)
	h.mutex.RLock()
	for _, m := range h.subscriptions {
		maps.Copy(m2, m)
	}
	h.mutex.RUnlock()
	return m2
}
