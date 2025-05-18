package internal

import (
	"context"
	"hash/crc32"
	"maps"
)

type hubMulti struct {
	hubs []*hub
}

func newHubMulti(hubCount int, m *metrics) (h *hubMulti) {
	hubCount = max(hubCount, 1)
	h = &hubMulti{
		hubs: make([]*hub, hubCount),
	}
	for i := range h.hubs {
		h.hubs[i] = newHub(m)
	}
	return
}
func (h *hubMulti) Run(ctx context.Context) {
	for _, h := range h.hubs {
		go h.Run(ctx)
	}
	return
}

func (h *hubMulti) Register(c *connection) {
	for _, topic := range c.topics {
		h.hubs[h.hash(topic)].Register(c)
	}
	return
}

func (h *hubMulti) Unregister(c *connection) {
	for _, topic := range c.topics {
		h.hubs[h.hash(topic)].Unregister(c)
	}
	return
}

func (h *hubMulti) Broadcast(m *message) {
	for _, topic := range m.Topics {
		h.hubs[h.hash(topic)].Broadcast(m)
	}
	return
}

func (h *hubMulti) Connections() map[*connection]bool {
	m2 := make(map[*connection]bool)
	for _, h := range h.hubs {
		maps.Copy(m2, h.Connections())
	}
	return m2
}

func (h *hubMulti) hash(topic string) int {
	return int(crc32.ChecksumIEEE([]byte(topic))) % len(h.hubs)
}
