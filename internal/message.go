package internal

import (
	"fmt"
	"io"
	"sync"
	"sync/atomic"
)

var messagePool = sync.Pool{
	New: func() any {
		return &message{
			ID: uuidv7(),
			n:  &atomic.Uint64{},
		}
	},
}

type message struct {
	ID     string
	Type   string
	Topics []string
	Data   string
	n      *atomic.Uint64
}

func newMessage(msgType string, topics []string, data string) (m *message) {
	m = messagePool.Get().(*message)
	m.Type = msgType
	m.Topics = topics
	m.Data = data
	return
}

func (msg *message) WriteTo(w io.Writer) (n int64, err error) {
	out := ""
	if len(msg.ID) > 0 {
		out += fmt.Sprintf("id: %v\n", msg.ID)
	}
	if len(msg.Type) > 0 {
		out += fmt.Sprintf("type: %v\n", msg.Type)
	}
	if len(msg.Data) > 0 {
		out += fmt.Sprintf("data: %s\n", msg.Data)
	}
	if len(out) == 0 {
		return
	}
	out += "\n"
	if _, err = w.Write([]byte(out)); err != nil {
		return
	}
	return int64(len(out)), nil
}

func (msg *message) release() {
	// msg must be released by both publisher and subscriber
	if msg.n.Add(1)%2 == 0 {
		msg.ID = uuidv7()
		messagePool.Put(msg)
	}
}
