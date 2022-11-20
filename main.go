package main

import (
	"context"
	_ "github.com/go-sql-driver/mysql"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	balancer, err := NewBalancer(&Config{
		StrategyType: RoundRobin,
		Nodes: []*Node{
			{
				address: "root:Mastermaster123@tcp(docker.for.mac.localhost:3306)/mysql",
				name:    "master",
			},
			{
				address: "root:slaveslave123@tcp(docker.for.mac.localhost:3307)/mysql",
				name:    "slave",
			},
			{
				address: "root:slaveslave123@tcp(docker.for.mac.localhost:3308)/mysql",
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
				panic(err)
			}

		case <-stop:
			balancer.Close()
			break
		}
	}
}
