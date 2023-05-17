package grpc

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	clock "github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/drand/drand/crypto"
	"github.com/drand/drand/internal/test/mock"
	"github.com/drand/drand/internal/test/testlogger"
)

func TestClient(t *testing.T) {
	lg := testlogger.New(t)
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	clk := clock.NewFakeClockAt(time.Now())
	l, server := mock.NewMockGRPCPublicServer(t, lg, "127.0.0.1:0", false, sch, clk)
	addr := l.Addr()

	go l.Start()
	defer l.Stop(context.Background())

	c, err := New(lg, addr, "", true, []byte(""))
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
		server.(mock.Service).EmitRand(false)
	}()
	r3, ok := <-res
	if !ok {
		t.Fatal("watch should work")
	}
	if r3.Round() != 1971 {
		t.Fatal("unexpected round")
	}
	cancel()
	_ = c.Close()
}

func TestClientClose(t *testing.T) {
	lg := testlogger.New(t)
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	clk := clock.NewFakeClockAt(time.Now())
	l, _ := mock.NewMockGRPCPublicServer(t, lg, "127.0.0.1:0", false, sch, clk)
	addr := l.Addr()

	go l.Start()
	defer l.Stop(context.Background())

	c, err := New(lg, addr, "", true, []byte(""))
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

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		//nolint:revive // we drain the channel
		for range c.Watch(context.Background()) {
		}
		wg.Done()
	}()

	err = c.Close()
	if err != nil {
		t.Fatal(err)
	}

	_, err = c.Get(context.Background(), 0)
	if status.Code(err) != codes.Canceled {
		t.Fatal("unexpected error from closed client", err)
	}

	wg.Wait() // wait for the watch to close
}
