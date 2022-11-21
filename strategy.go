package balancer

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"golang.org/x/sync/errgroup"
)

var (
	ErrNoNodesProvided = errors.New("empty slices of nodes")
	ErrNoNodesLeft     = errors.New("no nodes left for balancing")
)

type (
	// Strategy represents the methods a strategy need's to implement
	Strategy interface {
		Balance() (*Node, error)
		RemoveNode(node *Node)
		AddNode(node *Node)
		Close() error
	}

	// RoundRobinStrategy implements a Round Robin strategy
	RoundRobinStrategy struct {
		nodes Nodes
		next  uint32
		eg    *errgroup.Group
		lock  sync.RWMutex
	}
)

// NewRoundRobinStrategy serves as factory for the RoundRobinStrategy
// return an error if an empty slice of Nodes is provided
func NewRoundRobinStrategy(nodes Nodes) (*RoundRobinStrategy, error) {
	if len(nodes) == 0 {
		return nil, ErrNoNodesProvided
	}

	return &RoundRobinStrategy{
		nodes,
		0,
		&errgroup.Group{},
		sync.RWMutex{},
	}, nil
}

// Balance return the Node to query
func (p *RoundRobinStrategy) Balance() (*Node, error) {
	p.lock.Lock()
	defer p.lock.Unlock()
	if len(p.nodes) == 0 {
		return nil, ErrNoNodesLeft
	}

	n := atomic.AddUint32(&p.next, 1)
	node := p.nodes[(int(n)-1)%len(p.nodes)]

	return node, nil
}

// RemoveNode remove the specific node from strategy
func (p *RoundRobinStrategy) RemoveNode(node *Node) {
	p.lock.Lock()

	for i := range p.nodes {
		if p.nodes[i].Name != node.Name {
			continue
		}

		copy(p.nodes[i:], p.nodes[i+1:])
		p.nodes[len(p.nodes)-1] = &Node{}
		p.nodes = p.nodes[:len(p.nodes)-1]

		p.eg.Go(func() error {
			watchNodeForReconnect(node, p)
			return nil
		})

		break
	}

	p.lock.Unlock()
}

// AddNode adds the specific node to the strategy
func (p *RoundRobinStrategy) AddNode(node *Node) {
	p.lock.Lock()
	defer p.lock.Unlock()
	p.nodes = append(p.nodes, node)
}

// Close cancel all the go routines the check for reconnection
// of disconnected nodes
func (p *RoundRobinStrategy) Close() error {
	err := p.eg.Wait()
	if err != nil {
		return fmt.Errorf("failed to close strategy with error: %v", err)
	}

	return nil
}
