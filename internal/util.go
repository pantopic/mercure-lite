package internal

import (
	"fmt"

	"github.com/gofrs/uuid/v5"
	"github.com/valyala/fasthttp"
)

func uuidv7() string {
	uuid, _ := uuid.NewV7()
	return fmt.Sprintf("urn:uuid:%s", uuid)
}

func argTopics(args *fasthttp.Args) []string {
	bTopics := args.PeekMulti("topic")
	topics := make([]string, len(bTopics))
	for i, b := range bTopics {
		topics[i] = string(b)
	}
	return topics
}
