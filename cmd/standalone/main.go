package main

import (
	"github.com/caarlos0/env/v11"
	"github.com/pantopic/mercure-lite/internal"
)

func main() {
	cfg := internal.Config{
		LISTEN:             ":8001",
		PUBLISHER_JWT_KEY:  "SECRET",
		PUBLISHER_JWT_ALG:  "HS256",
		SUBSCRIBER_JWT_KEY: "SECRET",
		SUBSCRIBER_JWT_ALG: "HS256",
		CORS_ORIGINS:       "*",
	}
	if err := env.ParseWithOptions(&cfg, env.Options{
		UseFieldNameByDefault: true,
		Prefix:                "MERCURE_LITE_",
	}); err != nil {
		panic(err)
	}
	srv := internal.NewServer(cfg)
	srv.Start()
}
