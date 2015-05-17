package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/facebookgo/dvara"
	"github.com/facebookgo/gangliamr"
	"github.com/facebookgo/inject"
	"github.com/facebookgo/startstop"
	"github.com/facebookgo/stats"
)

func main() {
	if err := Main(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var debug *bool

func Main() error {
	messageTimeout := flag.Duration("message_timeout", 2*time.Minute, "timeout for one message to be proxied")
	clientIdleTimeout := flag.Duration("client_idle_timeout", 60*time.Minute, "idle timeout for client connections")
	getLastErrorTimeout := flag.Duration("get_last_error_timeout", time.Minute, "timeout for getLastError pinning")
	maxConnections := flag.Uint("max_connections", 1024, "maximum number of connections per mongo")
	portStart := flag.Int("port_start", 6000, "start of port range")
	portEnd := flag.Int("port_end", 6010, "end of port range")
	addrs := flag.String("addrs", "localhost:27017", "comma separated list of mongo addresses")
	host := flag.String("host", "127.0.0.1", "host to listen")
	maxPerClientConnections := flag.Uint("max_per_client_connections", 1024, "maximum number of connections per client")
	minIdleConnections := flag.Uint("min_idle_connection", 64, "the number of idle server connections we'll keep around")
	serverIdleTimeout := flag.Duration("server_idle_timeout", 60*time.Minute, "idle timeout for server connections")
	serverClosePoolSize := flag.Uint("server_close_poolsize", 1024, "the number of goroutines that will handle closing server connections")
	debug = flag.Bool("debug", false, "debug mode")

	flag.Parse()

	replicaSet := dvara.ReplicaSet{
		Addrs:                   *addrs,
		HostListen:              *host,
		PortStart:               *portStart,
		PortEnd:                 *portEnd,
		MaxConnections:          *maxConnections,
		MinIdleConnections:      *minIdleConnections,
		ServerIdleTimeout:       *serverIdleTimeout,
		ServerClosePoolSize:     *serverClosePoolSize,
		ClientIdleTimeout:       *clientIdleTimeout,
		MaxPerClientConnections: *maxPerClientConnections,
		GetLastErrorTimeout:     *getLastErrorTimeout,
		MessageTimeout:          *messageTimeout,
	}

	var statsClient stats.HookClient
	var log stdLogger
	var graph inject.Graph
	err := graph.Provide(
		&inject.Object{Value: &log},
		&inject.Object{Value: &replicaSet},
		&inject.Object{Value: &statsClient},
	)
	if err != nil {
		return err
	}
	if err := graph.Populate(); err != nil {
		return err
	}
	objects := graph.Objects()

	// Temporarily setup the metrics against a test registry.
	gregistry := gangliamr.NewTestRegistry()
	for _, o := range objects {
		if rmO, ok := o.Value.(registerMetrics); ok {
			rmO.RegisterMetrics(gregistry)
		}
	}
	if err := startstop.Start(objects, &log); err != nil {
		return err
	}
	defer startstop.Stop(objects, &log)

	ch := make(chan os.Signal, 2)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
	<-ch
	signal.Stop(ch)
	return nil
}

type registerMetrics interface {
	RegisterMetrics(r *gangliamr.Registry)
}
