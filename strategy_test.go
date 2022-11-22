package balancer

import (
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/agiledragon/gomonkey/v2"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sync/errgroup"
)

func TestNewRoundRobinStrategy(t *testing.T) {
	tests := []struct {
		name  string
		nodes Nodes
		want  *RoundRobinStrategy
		err   assert.ErrorAssertionFunc
	}{
		{
			name:  "it fails if empty nodes",
			nodes: Nodes{},
			want:  nil,
			err:   assert.Error,
		},
		{
			name: "it creates a new strategy",
			nodes: []*Node{
				{
					Address: "root:Mastermaster123@tcp(127.0.0.1:3306)/mysql",
					Name:    "master",
				},
			},
			want: &RoundRobinStrategy{
				nodes: []*Node{
					{
						Address: "root:Mastermaster123@tcp(127.0.0.1:3306)/mysql",
						Name:    "master",
					},
				},
				next: 0,
				eg:   &errgroup.Group{},
				lock: sync.RWMutex{},
			},
			err: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := NewRoundRobinStrategy(tt.nodes)
			tt.err(t, err)
			assert.Equal(t, got, tt.want)
		})
	}
}

func TestRoundRobinStrategy_Balance(t *testing.T) {
	t.Parallel()
	nodes := []*Node{
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
	}
	strategy, strategyErr := NewRoundRobinStrategy(nodes)
	assert.NoError(t, strategyErr)

	node1, err := strategy.Balance()
	assert.NoError(t, err)
	assert.Equal(t, node1, nodes[0])

	node2, err := strategy.Balance()
	assert.NoError(t, err)
	assert.Equal(t, node2, nodes[1])

	node3, err := strategy.Balance()
	assert.NoError(t, err)
	assert.Equal(t, node3, nodes[2])

	node4, err := strategy.Balance()
	assert.NoError(t, err)
	assert.Equal(t, node4, nodes[0])
}

func TestRoundRobinStrategy_BalanceErr(t *testing.T) {
	t.Parallel()
	node := Node{
		Address: "root:Mastermaster123@tcp(127.0.0.1:3306)/mysql",
		Name:    "master",
	}

	nodes := []*Node{&node}

	strategy, strategyErr := NewRoundRobinStrategy(nodes)
	assert.NoError(t, strategyErr)

	node1, err := strategy.Balance()
	assert.NoError(t, err)
	strategy.RemoveNode(node1)

	_, err = strategy.Balance()

	assert.EqualError(t, err, ErrNoNodesLeft.Error())
}

func TestRoundRobinStrategy_Remove_AddNode(t *testing.T) {
	t.Parallel()
	nodes := []*Node{
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
	}
	cnodes := make([]*Node, len(nodes))
	for i, p := range nodes {
		v := *p
		cnodes[i] = &v
	}

	strategy, strategyErr := NewRoundRobinStrategy(cnodes)
	assert.NoError(t, strategyErr)

	strategy.RemoveNode(nodes[1])

	assert.Equal(t, len(strategy.nodes), 2)
	//assert.Equal(t, []*Node{nodes[0], nodes[2]}, strategy.nodes)

	strategy.RemoveNode(nodes[2])
	assert.Equal(t, len(strategy.nodes), 1)

	strategy.AddNode(nodes[2])
	assert.Equal(t, len(strategy.nodes), 2)
}

func TestBalancer_CloseErr(t *testing.T) {
	eg := errgroup.Group{}
	patch := gomonkey.ApplyMethodFunc(reflect.TypeOf(&eg), "Wait", func() error {
		return errors.New("kabum")
	})
	defer patch.Reset()

	strategy, strategyErr := NewRoundRobinStrategy(Nodes{
		{
			Address: "root:Mastermaster123@tcp(127.0.0.1:3306)/mysql",
			Name:    "master",
		},
	})
	assert.NoError(t, strategyErr)

	strategy.eg = &eg

	err := strategy.Close()
	assert.Error(t, err)
}

func TestBalancer_Close(t *testing.T) {
	strategy, strategyErr := NewRoundRobinStrategy(Nodes{
		{
			Address: "root:Mastermaster123@tcp(127.0.0.1:3306)/mysql",
			Name:    "master",
		},
	})
	assert.NoError(t, strategyErr)

	err := strategy.Close()
	assert.NoError(t, err)
}
