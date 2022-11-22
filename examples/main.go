package main

import (
	"context"
	"fmt"
	balancer "github.com/Espina2/go-sql-connection-balancer"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	bal, err := balancer.NewBalancer(&balancer.Config{
		StrategyType: balancer.RoundRobin,
		Nodes: []*balancer.Node{
			{
				Address: "root:Mastermaster123@tcp(127.0.0.1:3306)/mysql",
				Name:    "master",
			},
			{
				Address: "root:slaveslave123@tcp(127.0.0.1:3307)/mysql",
				Name:    "slave",
			},
			{
				Address: "root:slaveslave123@tcp(127.0.0.1:3308)/mysql",
				Name:    "slave-2",
			},
		},
		Connection: balancer.Connection{
			MaxOpenConnections: 50,
			MaxIdleConns:       5,
			ConnMaxLifetime:    time.Hour,
			Driver:             "mysql",
		},
	})
	if err != nil {
		panic(err)
	}

	ctx := context.Background()

	tickerInterval := 100
	ticker := time.NewTicker(time.Millisecond * time.Duration(tickerInterval))

	for {
		select {
		case <-ticker.C:
			_, err := bal.ExecContext(ctx, "SHOW PROCESSLIST;")
			if err != nil {
				fmt.Printf("%v\n", err)
			}

		case <-stop:
			bal.Close()
			break
		}
	}
}
