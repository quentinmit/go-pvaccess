package main

import (
	"context"

	pvaccess "github.com/quentinmit/go-pvaccess"
)

func main() {
	s := &pvaccess.Server{}
	s.ListenAndServe(context.Background())
}
