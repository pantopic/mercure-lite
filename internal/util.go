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
