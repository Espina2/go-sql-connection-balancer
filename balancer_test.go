package balancer

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/agiledragon/gomonkey/v2"
	_ "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
)

func TestNewBalancer(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		prepare func(*testing.T) (*sql.DB, sqlmock.Sqlmock, *gomonkey.Patches)
		verify  func(*testing.T, sqlmock.Sqlmock)
		want    assert.ValueAssertionFunc
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "it fails if no nodes are provided",
			config: &Config{
				Nodes:      nil,
				Strategy:   nil,
				Connection: Connection{},
			},
			prepare: nopPrepare,
			verify:  nopVerify,
			want:    assert.Nil,
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
			prepare: nopPrepare,
			verify:  nopVerify,
			want:    assert.Nil,
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
			prepare: nopPrepare,
			verify:  nopVerify,
			want:    assert.Nil,
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
			prepare: nopPrepare,
			verify:  nopVerify,
			want:    assert.Nil,
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
					MaxIdleConns:       1,
					ConnMaxLifetime:    1 * time.Hour,
					Driver:             "mysql",
				},
			},
			prepare: func(t *testing.T) (*sql.DB, sqlmock.Sqlmock, *gomonkey.Patches) {
				db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
				assert.NoError(t, err)

				patch := gomonkey.ApplyFunc(sql.Open, func(driverName, dataSourceName string) (*sql.DB, error) {
					return db, nil
				})

				e := mock.ExpectPing()
				e.WillReturnError(fmt.Errorf("could not ping"))
				return db, mock, patch
			},
			verify: func(t *testing.T, mock sqlmock.Sqlmock) {
				assert.NoError(t, mock.ExpectationsWereMet())
			},
			want:    assert.Nil,
			wantErr: assert.Error,
		},
		{
			name: "it fails if strategyfunc returns an error",
			config: &Config{
				Nodes: Nodes{{
					Address: "root:Mastermaster123@tcp(127.0.0.1:3306)/mysql",
					Name:    "master",
				}},
				Strategy: func(Nodes) (Strategy, error) {
					return nil, fmt.Errorf("failed to bootstrap strategy")
				},
				Connection: Connection{
					MaxOpenConnections: 10,
					MaxIdleConns:       1,
					ConnMaxLifetime:    1 * time.Hour,
					Driver:             "mysql",
				},
			},
			prepare: func(t *testing.T) (*sql.DB, sqlmock.Sqlmock, *gomonkey.Patches) {
				db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
				assert.NoError(t, err)

				patch := gomonkey.ApplyFunc(sql.Open, func(driverName, dataSourceName string) (*sql.DB, error) {
					return db, nil
				})

				mock.ExpectPing()
				return db, mock, patch
			},
			verify: func(t *testing.T, mock sqlmock.Sqlmock) {
				assert.NoError(t, mock.ExpectationsWereMet())
			},
			want:    assert.Nil,
			wantErr: assert.Error,
		},
		{
			name: "creates new balancer",
			config: &Config{
				Nodes: Nodes{{
					Address: "root:Mastermaster123@tcp(127.0.0.1:3306)/mysql",
					Name:    "master",
				}},
				Strategy: func(n Nodes) (Strategy, error) { return NewRoundRobinStrategy(n) },
				Connection: Connection{
					MaxOpenConnections: 10,
					MaxIdleConns:       1,
					ConnMaxLifetime:    1 * time.Hour,
					Driver:             "mysql",
				},
			},
			prepare: func(*testing.T) (*sql.DB, sqlmock.Sqlmock, *gomonkey.Patches) {
				db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
				assert.NoError(t, err)

				patch := gomonkey.ApplyFunc(sql.Open, func(driverName, dataSourceName string) (*sql.DB, error) {
					return db, nil
				})

				e := mock.ExpectPing()
				e.WillReturnError(nil)
				return db, mock, patch
			},
			verify: func(t *testing.T, mock sqlmock.Sqlmock) {
				assert.NoError(t, mock.ExpectationsWereMet())
			},
			want:    assert.NotNil,
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, patches := tt.prepare(t)
			defer func() {
				if db != nil {
					db.Close()
				}
				if patches != nil {
					patches.Reset()
				}
			}()

			got, err := NewBalancer(tt.config)
			tt.wantErr(t, err)
			tt.want(t, got)
			tt.verify(t, mock)
		})
	}
}

func nopPrepare(*testing.T) (*sql.DB, sqlmock.Sqlmock, *gomonkey.Patches) {
	return nil, nil, nil
}

func nopVerify(*testing.T, sqlmock.Sqlmock) {
}
