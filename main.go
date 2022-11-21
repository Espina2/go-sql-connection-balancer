package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	balancer, err := NewBalancer(&Config{
		StrategyType: RoundRobin,
		Nodes: []*Node{
			{
				address: "root:Mastermaster123@tcp(127.0.0.1:3306)/mysql",
				name:    "master",
			},
			{
				address: "root:slaveslave123@tcp(127.0.0.1:3307)/mysql",
				name:    "slave",
			},
			{
				address: "root:slaveslave123@tcp(127.0.0.1:3308)/mysql",
				name:    "slave-2",
			},
		},
		Connection: Connection{
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

	ticker := time.NewTicker(time.Millisecond * 100)

	for {
		select {
		case <-ticker.C:
			_, err := balancer.ExecContext(ctx, "SHOW PROCESSLIST;")
			if err != nil {
				fmt.Printf("%w\n", err)
			}

		case <-stop:
			balancer.Close()
			break
		}
	}
}
