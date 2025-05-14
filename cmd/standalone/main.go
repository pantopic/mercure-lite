package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/caarlos0/env/v11"

	"github.com/pantopic/mercure-lite/internal"
)

func main() {
	ctx := context.Background()
	cfg := internal.Config{}
	if err := env.ParseWithOptions(&cfg, env.Options{
		Prefix: "MERCURE_LITE_",
	}); err != nil {
		panic(err)
	}
	srv := internal.NewServer(cfg)
	if err := srv.Start(ctx); err != nil {
		log.Fatal(err)
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	signal.Notify(stop, syscall.SIGTERM)
	<-stop

	srv.Stop()
}
