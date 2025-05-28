package internal

import (
	"fmt"

	"github.com/gofrs/uuid/v5"

	"github.com/pantopic/mercure-lite"
)

type (
	Config    = mercurelite.Config
	ConfigJWT = mercurelite.ConfigJWT
)

func uuidv7() string {
	uuid, _ := uuid.NewV7()
	return fmt.Sprintf("urn:uuid:%s", uuid)
}

func msgIDtimestamp(id string) uint64 {
	if len(id) == 0 {
		return 0
	}
	u, err := uuid.FromString(id)
	if err != nil {
		return 0
	}
	t, err := uuid.TimestampFromV7(u)
	if err != nil {
		return 0
	}
	return uint64(t)
}
