package internal

import (
	"encoding/json"
	"fmt"
	"io"
)

type message struct {
	ID     string
	Type   string
	Topics []string
	Data   string
}

func newMessage(msgType string, topics []string, data string) (m *message) {
	return &message{
		ID:     uuidv7(),
		Type:   msgType,
		Topics: topics,
		Data:   data,
	}
}

func (msg *message) WriteTo(w io.Writer) (in int64, err error) {
	var out []byte
	if len(msg.ID) > 0 {
		out = fmt.Appendf(out, "id: %v\n", msg.ID)
	}
	if len(msg.Type) > 0 {
		out = fmt.Appendf(out, "type: %v\n", msg.Type)
	}
	if len(msg.Data) > 0 {
		out = fmt.Appendf(out, "data: %s\n", msg.Data)
	}
	if len(out) == 0 {
		return
	}
	n, err := w.Write(append(out, []byte("\n")...))
	return int64(n), err
}

func (msg *message) ToJson() (out []byte) {
	out, _ = json.Marshal(msg)
	return
}

func (msg *message) FromJson(in []byte) {
	json.Unmarshal(in, msg)
}

func (msg *message) timestamp() uint64 {
	return msgIDtimestamp(msg.ID)
}
