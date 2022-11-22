package balancer

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"time"

	"github.com/sethvargo/go-retry"
)

const (
	RetryReconnectDelayInSeconds = 5
)

type StrategyType string

const (
	RoundRobin StrategyType = "ROUND_ROBIN"
)

type (
	// Node represents a SQL node
	Node struct {
		conn    *sql.DB
		Name    string
		Address string
	}

	Nodes []*Node

	// Connection represents the Connection config for the Nodes
	Connection struct {
		MaxOpenConnections int
		MaxIdleConns       int
		ConnMaxLifetime    time.Duration
		Driver             string
	}

	// Config holds the Balancer configuration
	Config struct {
		Nodes        Nodes
		StrategyType StrategyType
		Connection   Connection
	}

	// Balancer represents SQL Load Balancer and wraps sql.ExecerContext and sql.QueryerContext compatible methods
	// All methods exposed are balanced following the Strategy provided
	Balancer struct {
		nodes    Nodes
		strategy Strategy
	}
)

func NewStrategy(nodes Nodes, strategyType StrategyType) (Strategy, error) {
	if strategyType == RoundRobin {
		return NewRoundRobinStrategy(nodes)
	}

	return nil, fmt.Errorf("invalid strategy provided %v", strategyType)
}

// NewBalancer create a new SQL Load Balancer
func NewBalancer(config *Config) (*Balancer, error) {
	var conns []*Node
	var failedNodes []*Node
	balancer := Balancer{nodes: conns}

	for _, node := range config.Nodes {
		sqlConn, errSQL := sql.Open(config.Connection.Driver, node.Address)
		if errSQL != nil {
			return nil, errSQL
		}

		sqlConn.SetMaxOpenConns(config.Connection.MaxOpenConnections)
		sqlConn.SetMaxIdleConns(config.Connection.MaxIdleConns)
		sqlConn.SetConnMaxLifetime(config.Connection.ConnMaxLifetime)

		if errPing := sqlConn.Ping(); errPing != nil {
			node.conn = sqlConn
			failedNodes = append(failedNodes, node)
			continue
		}

		node.conn = sqlConn
		balancer.nodes = append(balancer.nodes, node)
	}

	pol, polErr := NewStrategy(balancer.nodes, config.StrategyType)
	if polErr != nil {
		return nil, polErr
	}

	balancer.strategy = pol

	for i := range failedNodes {
		go watchNodeForReconnect(failedNodes[i], pol)
	}

	return &balancer, nil
}

// BeginTx wrap a *sql.BeginTx
func (m Balancer) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return m.beginTx(ctx, opts)
}

func (m Balancer) beginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	var tx *sql.Tx
	err := retry.Constant(ctx, 1*time.Millisecond, func(ctx context.Context) error {
		node, err := m.strategy.Balance()
		if err != nil {
			return err
		}

		txt, err := node.conn.BeginTx(ctx, opts)
		if err != nil {
			if err == driver.ErrBadConn {
				m.strategy.RemoveNode(node)
				return retry.RetryableError(err)
			}
			return err
		}

		tx = txt
		return nil
	})

	if err != nil {
		return nil, err
	}

	return tx, nil
}

// ExecContext wrap a *sql.ExecContext
func (m Balancer) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return m.execContext(ctx, query, args...)
}

func (m Balancer) execContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	var res sql.Result
	err := retry.Constant(ctx, 1*time.Millisecond, func(ctx context.Context) error {
		node, err := m.strategy.Balance()
		if err != nil {
			return err
		}

		result, err := node.conn.ExecContext(ctx, query, args...)

		if err != nil {
			if err == driver.ErrBadConn {
				m.strategy.RemoveNode(node)
				return retry.RetryableError(err)
			}
			return err
		}

		res = result
		return nil
	})

	if err != nil {
		return nil, err
	}

	return res, nil
}

// QueryContext wrap a *sql.QueryContext
func (m Balancer) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return m.queryContext(ctx, query, args...)
}

func (m Balancer) queryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	var res *sql.Rows
	err := retry.Constant(ctx, 1*time.Millisecond, func(ctx context.Context) error {
		node, err := m.strategy.Balance()
		if err != nil {
			return err
		}

		rows, err := node.conn.QueryContext(ctx, query, args...)

		if err != nil {
			if err == driver.ErrBadConn {
				m.strategy.RemoveNode(node)
				return retry.RetryableError(err)
			}
			return err
		}

		res = rows
		return nil
	})

	if err != nil {
		return nil, err
	}

	return res, nil
}

// QueryRowContext wrap a *sql.QueryRowContext
func (m Balancer) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return m.queryRowContext(ctx, query, args...)
}

func (m Balancer) queryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	var res *sql.Row
	_ = retry.Constant(ctx, 1*time.Millisecond, func(ctx context.Context) error {
		node, errBalance := m.strategy.Balance()
		if errBalance != nil {
			return errBalance
		}

		row := node.conn.QueryRowContext(ctx, query, args...)
		err := row.Err()
		res = row

		if err != nil {
			if err == driver.ErrBadConn {
				m.strategy.RemoveNode(node)
				return retry.RetryableError(err)
			}
			return err
		}

		return nil
	})

	return res
}

func watchNodeForReconnect(node *Node, strategy Strategy) {
	t := time.NewTicker(time.Second * RetryReconnectDelayInSeconds)

	for range t.C {
		err := node.conn.Ping()
		if err != nil {
			continue
		}

		strategy.AddNode(node)
		break
	}
}

// Close the underlaying resources for all Nodes
func (m Balancer) Close() []error {
	var errs []error
	for i := range m.nodes {
		err := m.nodes[i].conn.Close()

		if err != nil {
			errs = append(errs, err)
		}
	}

	err := m.strategy.Close()
	if err != nil {
		errs = append(errs, err)
	}

	return errs
}
