package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/client_golang/prometheus/push"

	"github.com/drand/drand/client"
	"github.com/drand/drand/client/basic"
	"github.com/drand/drand/cmd/client/lib"
	"github.com/drand/drand/log"
	"github.com/urfave/cli/v2"
)

// Automatically set through -ldflags
// Example: go install -ldflags "-X main.version=`git describe --tags` -X main.buildDate=`date -u +%d/%m/%Y@%H:%M:%S` -X main.gitCommit=`git rev-parse HEAD`"
var (
	version   = "master"
	gitCommit = "none"
	buildDate = "unknown"
)

var watchFlag = &cli.BoolFlag{
	Name:  "watch",
	Usage: "stream new values as they become available",
}

var roundFlag = &cli.IntFlag{
	Name:  "round",
	Usage: "request randomness for a specific round",
}

// client metric flags

var clientMetricsAddressFlag = &cli.StringFlag{
	Name:  "client-metrics-address",
	Usage: "Server address for Prometheus metrics.",
	Value: ":8080",
}

var clientMetricsGatewayFlag = &cli.StringFlag{
	Name:  "client-metrics-gateway",
	Usage: "Push gateway for Prometheus metrics.",
}

var clientMetricsPushIntervalFlag = &cli.Int64Flag{
	Name:  "client-metrics-push-interval",
	Usage: "Push interval in seconds for Prometheus gateway.",
}

var clientMetricsIDFlag = &cli.StringFlag{
	Name:  "client-metrics-id",
	Usage: "Unique identifier for the client instance, used by the metrics system.",
}

func main() {
	app := cli.NewApp()
	app.Name = "drand-client"
	app.Version = version
	app.Usage = "CDN Drand client for loading randomness from an HTTP endpoint"
	app.Flags = append(lib.ClientFlags,
		watchFlag, roundFlag,
		clientMetricsAddressFlag, clientMetricsGatewayFlag, clientMetricsIDFlag,
		clientMetricsPushIntervalFlag)
	app.Action = Client
	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Printf("drand client %v (date %v, commit %v)\n", version, buildDate, gitCommit)
	}

	err := app.Run(os.Args)
	if err != nil {
		panic(err)
	}
}

// Client loads randomness from a server
func Client(c *cli.Context) error {
	opts := []basic.Option{}

	if c.IsSet(clientMetricsIDFlag.Name) {
		clientID := c.String(clientMetricsIDFlag.Name)
		if !c.IsSet(clientMetricsAddressFlag.Name) && !c.IsSet(clientMetricsGatewayFlag.Name) {
			return fmt.Errorf("missing prometheus address or push gateway")
		}
		metricsAddr := c.String(clientMetricsAddressFlag.Name)
		metricsGateway := c.String(clientMetricsGatewayFlag.Name)
		metricsPushInterval := c.Int64(clientMetricsPushIntervalFlag.Name)
		bridge := newPrometheusBridge(metricsAddr, metricsGateway, metricsPushInterval)
		bridgeWithID := basic.WithPrometheus(prometheus.WrapRegistererWith(
			prometheus.Labels{"client_id": clientID},
			bridge))
		opts = append(opts, bridgeWithID)
	}

	client, err := lib.Create(c, opts...)
	if err != nil {
		return err
	}

	if c.IsSet(watchFlag.Name) {
		return Watch(c, client)
	}

	round := uint64(0)
	if c.IsSet(roundFlag.Name) {
		round = uint64(c.Int(roundFlag.Name))
	}
	rand, err := client.Get(context.Background(), round)
	if err != nil {
		return err
	}
	fmt.Printf("%v\n", rand)
	return nil
}

// Watch streams randomness from a client
func Watch(c *cli.Context, client client.Client) error {
	results := client.Watch(context.Background())
	for r := range results {
		fmt.Printf("%d\t%x\n", r.Round(), r.Randomness())
	}
	return nil
}

func newPrometheusBridge(address string, gateway string, pushIntervalSec int64) prometheus.Registerer {
	b := &prometheusBridge{
		address:         address,
		pushIntervalSec: pushIntervalSec,
		Registry:        prometheus.NewRegistry(),
	}
	if gateway != "" {
		b.pusher = push.New(gateway, "drand_client_observations_push").Gatherer(b.Registry)
		go b.pushLoop()
	}
	if address != "" {
		http.Handle("/metrics", promhttp.HandlerFor(b.Registry, promhttp.HandlerOpts{
			Timeout: 10 * time.Second,
		}))
		go func() {
			log.DefaultLogger.Fatal("client", http.ListenAndServe(address, nil))
		}()
	}
	return b
}

type prometheusBridge struct {
	*prometheus.Registry
	address         string
	pushIntervalSec int64
	pusher          *push.Pusher
}

func (b *prometheusBridge) pushLoop() {
	for {
		time.Sleep(time.Second * time.Duration(b.pushIntervalSec))
		if err := b.pusher.Push(); err != nil {
			log.DefaultLogger.Info("client_metrics", "prometheus gateway push (%v)", err)
		}
	}
}