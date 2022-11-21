package main

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/sethvargo/go-retry"
)

type StrategyType string

const (
	RoundRobin StrategyType = "ROUND_ROBIN"
)

type (
	// Node represents a SQL node
	Node struct {
		conn    *sql.DB
		name    string
		address string
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

	// Strategy represents the methods a strategy need's to implement
	Strategy interface {
		Balance() (*Node, error)
		RemoveNode(node *Node)
		AddNode(node *Node)
	}

	// RoundRobinPolicy implements a Round Robin policy
	RoundRobinPolicy struct {
		nodes Nodes
		next  uint32
		lock  sync.RWMutex
	}

	// Balancer represents SQL Load Balancer and wraps sql.ExecerContext and sql.QueryerContext compatible methods
	// All methods exposed are balanced following the Strategy provided
	Balancer struct {
		nodes  Nodes
		policy Strategy
	}
)

func NewRoundRobinPolicy(connections Nodes) RoundRobinPolicy {
	return RoundRobinPolicy{connections, 0, sync.RWMutex{}}
}

func (p *RoundRobinPolicy) Balance() (*Node, error) {
	p.lock.Lock()
	defer p.lock.Unlock()
	if len(p.nodes) == 0 {
		return nil, fmt.Errorf("empty set of nodes")
	}
	n := atomic.AddUint32(&p.next, 1)
	node := p.nodes[(int(n)-1)%len(p.nodes)]

	spew.Dump(node.name)
	return node, nil
}

func (p *RoundRobinPolicy) RemoveNode(node *Node) {
	p.lock.Lock()

	for i := range p.nodes {
		if p.nodes[i].name == node.name {
			copy(p.nodes[i:], p.nodes[i+1:])
			p.nodes[len(p.nodes)-1] = &Node{}
			p.nodes = p.nodes[:len(p.nodes)-1]

			go watchNodeForReconnect(node, p)
		}
	}
	p.lock.Unlock()
}

func watchNodeForReconnect(node *Node, strategy Strategy) {
	t := time.NewTicker(time.Second * 5)

	for range t.C {
		err := node.conn.Ping()
		if err != nil {
			continue
		}

		strategy.AddNode(node)
		break
	}
}

func (p *RoundRobinPolicy) AddNode(node *Node) {
	p.lock.Lock()
	defer p.lock.Unlock()
	p.nodes = append(p.nodes, node)
}

func NewPolicy(nodes Nodes, policyType StrategyType) (Strategy, error) {
	switch policyType {
	case RoundRobin:
		r := NewRoundRobinPolicy(nodes)
		return &r, nil
	}

	return nil, fmt.Errorf("invalid policy provided %v", policyType)
}

// NewBalancer create a new SQL Load Balancer
func NewBalancer(config *Config) (*Balancer, error) {
	var conns []*Node
	var failedNodes []*Node
	balancer := Balancer{nodes: conns}

	for _, node := range config.Nodes {
		sqlConn, errSQL := sql.Open(config.Connection.Driver, node.address)
		if errSQL != nil {
			return nil, errSQL
		}

		sqlConn.SetMaxOpenConns(config.Connection.MaxOpenConnections)
		sqlConn.SetMaxIdleConns(config.Connection.MaxIdleConns)
		sqlConn.SetConnMaxLifetime(config.Connection.ConnMaxLifetime)

		if errPing := sqlConn.Ping(); errPing != nil {
			spew.Dump(errPing)
			node.conn = sqlConn
			failedNodes = append(failedNodes, node)
			continue
			//return nil, errPing
		}

		node.conn = sqlConn
		balancer.nodes = append(balancer.nodes, node)
	}

	pol, polErr := NewPolicy(balancer.nodes, config.StrategyType)
	if polErr != nil {
		return nil, polErr
	}

	balancer.policy = pol

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
		node, err := m.policy.Balance()
		if err != nil {
			return err
		}

		txt, err := node.conn.BeginTx(ctx, opts)
		if err != nil {
			if err == driver.ErrBadConn {
				m.policy.RemoveNode(node)
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
		node, err := m.policy.Balance()
		if err != nil {
			return err
		}

		result, err := node.conn.ExecContext(ctx, query, args...)

		if err != nil {
			if err == driver.ErrBadConn {
				m.policy.RemoveNode(node)
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
		node, err := m.policy.Balance()
		if err != nil {
			return err
		}

		rows, err := node.conn.QueryContext(ctx, query, args...)

		if err != nil {
			if err == driver.ErrBadConn {
				m.policy.RemoveNode(node)
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
		node, errBalance := m.policy.Balance()
		if errBalance != nil {
			return errBalance
		}

		row := node.conn.QueryRowContext(ctx, query, args...)
		err := row.Err()
		res = row

		if err != nil {
			if err == driver.ErrBadConn {
				m.policy.RemoveNode(node)
				return retry.RetryableError(err)
			}
			return err
		}

		return nil
	})

	return res
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

	return errs
}
