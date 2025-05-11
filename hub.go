package mercurelite

import (
	"maps"
	"sync"
)

type Hub interface {
	Run()
	Broadcast(message)
	Register(*connection)
	Unregister(*connection)
	Connections() map[*connection]bool
}

func newHub() *hub {
	return &hub{
		subscriptions: make(map[string]map[*connection]bool),
		broadcast:     make(chan message),
		register:      make(chan *connection),
		unregister:    make(chan *connection),
	}
}

type hub struct {
	mutex         sync.RWMutex
	subscriptions map[string]map[*connection]bool
	broadcast     chan message
	register      chan *connection
	unregister    chan *connection
}

func (h *hub) Run() {
	for {
		select {
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
			close(conn.send)
			h.mutex.Unlock()
		case msg := <-h.broadcast:
			h.mutex.RLock()
			for _, t := range msg.Topics {
				if connections, ok := h.subscriptions[t]; ok {
					for conn := range connections {
						select {
						case conn.send <- msg:
						default:
							for _, topic := range conn.topics {
								delete(h.connections[topic], conn)
							}
							close(conn.send)
							// go broadcast inactive subscription
						}
					}
				}
			}
			h.mutex.RUnlock()
		}
	}
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

func (h *hub) Broadcast(msg message) {
	if msg.ID == "" {
		msg.ID = uuidv7()
	}
	h.broadcast <- msg
}

func (h *hub) Register(conn *connection) {
	h.register <- conn
}

func (h *hub) Unregister(conn *connection) {
	h.unregister <- conn
}
