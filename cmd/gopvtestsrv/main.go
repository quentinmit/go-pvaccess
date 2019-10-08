package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	log "github.com/sirupsen/logrus"

	pvaccess "github.com/quentinmit/go-pvaccess"
	"github.com/quentinmit/go-pvaccess/internal/ctxlog"
)

func main() {
	log.SetLevel(log.TraceLevel)
	ctx, cancel := context.WithCancel(context.Background())
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigs
		ctxlog.L(ctx).Infof("received signal %s; exiting", sig)
		cancel()
	}()
	s, err := pvaccess.NewServer()
	if err != nil {
		ctxlog.L(ctx).Fatalf("creating server: %v", err)
	}
	s.ListenAndServe(ctx)
}
