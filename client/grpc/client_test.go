package grpc

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/drand/drand/test/mock"
)

func TestClient(t *testing.T) {
	l, server := mock.NewMockGRPCPublicServer("localhost:0", false)
	addr := l.Addr()
	go l.Start()
	defer l.Stop(context.Background())

	c, err := New(addr, "", true)
	if err != nil {
		t.Fatal(err)
	}
	result, err := c.Get(context.Background(), 1969)
	if err != nil {
		t.Fatal(err)
	}
	if result.Round() != 1969 {
		t.Fatal("unexpected round.")
	}
	r2, err := c.Get(context.Background(), 1970)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(r2.Randomness(), result.Randomness()) {
		t.Fatal("unexpected equality")
	}

	rat := c.RoundAt(time.Now())
	if rat == 0 {
		t.Fatal("round at should function")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	res := c.Watch(ctx)
	go func() {
		time.Sleep(50 * time.Millisecond)
		server.(mock.MockService).EmitRand(false)
	}()
	r3, ok := <-res
	if !ok {
		t.Fatal("watch should work")
	}
	if r3.Round() != 1971 {
		t.Fatal("unexpected round")
	}
	cancel()
}