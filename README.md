[![MIT License](https://img.shields.io/badge/License-MIT-green.svg)](https://choosealicense.com/licenses/mit/)

# Golang SQL Load Balancer

This project aims to give support to a client to load balance connections
between multiple sql instances.

#### What this project not aim to do

- Do automatic split of Read and Write traffic

## Installation

Install with go mod modules

```bash
  go get github.com/Espina2/go-sql-connection-balancer

  # Also import the driver you are using like for example
  go get github.com/go-sql-driver/mysql
```

## Usage/Examples

```go
package main

import (
	"context"
	_ "github.com/go-sql-driver/mysql"
	"time"
)

func main() {
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

	defer balancer.Close()

	ctx := context.Background()

	_, err = balancer.ExecContext(ctx, "SHOW PROCESSLIST;")
	if err != nil {
		panic(err)
	}
}
```

```go
package main

import (
	"context"
	_ "github.com/go-sql-driver/mysql"
	"time"
)

func main() {
	writerConnection, writerConnectionErr := NewBalancer(&Config{
		StrategyType: RoundRobin,
		Nodes: []*Node{
			{
				address: "root:Mastermaster123@tcp(docker.for.mac.localhost:3306)/mysql",
				name:    "master",
			},
		},
		Connection: Connection{
			MaxOpenConnections: 50,
			MaxIdleConns:       5,
			ConnMaxLifetime:    time.Hour,
			Driver:             "mysql",
		},
	})
	if writerConnectionErr != nil {
		panic(writerConnectionErr)
	}
	defer writerConnection.Close()

	readerConnection, readerConnectionErr := NewBalancer(&Config{
		StrategyType: RoundRobin,
		Nodes: []*Node{
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
	if readerConnectionErr != nil {
		panic(readerConnectionErr)
	}
	defer readerConnection.Close()

	ctx := context.Background()
	writerConnection.ExecContext(ctx, "SHOW PROCESSLIST;")
	readerConnection.QueryContext(ctx, "SELECT * FROM db;")
}
```

## Authors

- [@Espina2](https://www.github.com/Espina2)

## Used By

This project is used in production by the following companies:

- [Easypay](https://www.easypay.pt/)

