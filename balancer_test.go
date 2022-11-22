package balancer

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	_ "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
)

func TestNewBalancer(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		prepare func()
		want    *Balancer
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "it fails if no nodes are provided",
			config: &Config{
				Nodes:      nil,
				Strategy:   nil,
				Connection: Connection{},
			},
			want:    nil,
			wantErr: assert.Error,
		},
		{
			name: "it fails if invalid connection are provided",
			config: &Config{
				Nodes: Nodes{{
					Address: "root:Mastermaster123@tcp(127.0.0.1:3306)/mysql",
					Name:    "master",
				}},
				Strategy:   nil,
				Connection: Connection{},
			},
			want:    nil,
			wantErr: assert.Error,
		},
		{
			name: "it fails if invalid driver are provided",
			config: &Config{
				Nodes: Nodes{{
					Address: "root:Mastermaster123@tcp(127.0.0.1:3306)/mysql",
					Name:    "master",
				}},
				Strategy: nil,
				Connection: Connection{
					MaxOpenConnections: 10,
					MaxIdleConns:       0,
					ConnMaxLifetime:    1 * time.Hour,
					Driver:             "",
				},
			},
			want:    nil,
			wantErr: assert.Error,
		},
		{
			name: "it fails if can't connect to any node",
			config: &Config{
				Nodes: Nodes{{
					Address: "root:Mastermaster123@tcp(127.0.0.1:3306)/mysql",
					Name:    "master",
				}},
				Strategy: nil,
				Connection: Connection{
					MaxOpenConnections: 10,
					MaxIdleConns:       0,
					ConnMaxLifetime:    1 * time.Hour,
					Driver:             "mysql",
				},
			},
			want:    nil,
			wantErr: assert.Error,
		},
		{
			name: "it fails if no strategy is provided",
			config: &Config{
				Nodes: Nodes{{
					Address: "root:Mastermaster123@tcp(127.0.0.1:3306)/mysql",
					Name:    "master",
				}},
				Strategy: nil,
				Connection: Connection{
					MaxOpenConnections: 10,
					MaxIdleConns:       0,
					ConnMaxLifetime:    1 * time.Hour,
					Driver:             "mysql",
				},
			},
			prepare: func() {
				_ = gomonkey.ApplyMethodFunc(&sql.DB{}, "Ping", func() error {
					return nil
				})
				//defer patch.Reset()
			},
			want:    nil,
			wantErr: assert.Error,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.prepare != nil {
				tt.prepare()
			}
			got, err := NewBalancer(tt.config)
			if !tt.wantErr(t, err, fmt.Sprintf("NewBalancer(%v)", tt.config)) {
				return
			}
			assert.Equalf(t, tt.want, got, "NewBalancer(%v)", tt.config)
		})
	}
}
