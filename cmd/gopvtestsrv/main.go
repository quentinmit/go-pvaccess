package main

import (
	"context"

	log "github.com/sirupsen/logrus"

	pvaccess "github.com/quentinmit/go-pvaccess"
)

func main() {
	log.SetLevel(log.TraceLevel)
	s := &pvaccess.Server{}
	s.ListenAndServe(context.Background())
}
