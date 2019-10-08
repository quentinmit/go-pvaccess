package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	log "github.com/sirupsen/logrus"

	pvaccess "github.com/quentinmit/go-pvaccess"
	"github.com/quentinmit/go-pvaccess/internal/ctxlog"
)

var (
	disableSearch = flag.Bool("disable_search", false, "disable UDP beacon/search support")
	verbose       = flag.Bool("v", false, "verbose mode")
)

func main() {
	flag.Parse()

	log.SetLevel(log.InfoLevel)
	if *verbose {
		log.SetLevel(log.TraceLevel)
	}
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
	s.DisableSearch = *disableSearch

	c := &pvaccess.SimpleChannel{ChannelName: "gopvtest"}
	c.Set(1)
	s.AddChannelProvider(c)

	s.ListenAndServe(ctx)
}
