package balancer_test

import (
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	balancer "github.com/Espina2/go-sql-connection-balancer"
	mock_strategy "github.com/Espina2/go-sql-connection-balancer/internal/mocks"
	"github.com/agiledragon/gomonkey/v2"
	_ "github.com/go-sql-driver/mysql"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestNewBalancer(t *testing.T) {
	tests := []struct {
		name    string
		config  *balancer.Config
		prepare func(*testing.T) (*sql.DB, sqlmock.Sqlmock, *gomonkey.Patches)
		verify  func(*testing.T, sqlmock.Sqlmock)
		want    assert.ValueAssertionFunc
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "it fails if no nodes are provided",
			config: &balancer.Config{
				Nodes:      nil,
				Strategy:   nil,
				Connection: balancer.Connection{},
			},
			prepare: nopPrepare,
			verify:  nopVerify,
			want:    assert.Nil,
			wantErr: assert.Error,
		},
		{
			name: "it fails if invalid connection are provided",
			config: &balancer.Config{
				Nodes: balancer.Nodes{{
					Address: "root:Mastermaster123@tcp(127.0.0.1:3306)/mysql",
					Name:    "master",
				}},
				Strategy:   nil,
				Connection: balancer.Connection{},
			},
			prepare: nopPrepare,
			verify:  nopVerify,
			want:    assert.Nil,
			wantErr: assert.Error,
		},
		{
			name: "it fails if invalid driver are provided",
			config: &balancer.Config{
				Nodes: balancer.Nodes{{
					Address: "root:Mastermaster123@tcp(127.0.0.1:3306)/mysql",
					Name:    "master",
				}},
				Strategy: nil,
				Connection: balancer.Connection{
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
			config: &balancer.Config{
				Nodes: balancer.Nodes{{
					Address: "root:Mastermaster123@tcp(127.0.0.1:3306)/mysql",
					Name:    "master",
				}},
				Strategy: nil,
				Connection: balancer.Connection{
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
			config: &balancer.Config{
				Nodes: balancer.Nodes{{
					Address: "root:Mastermaster123@tcp(127.0.0.1:3306)/mysql",
					Name:    "master",
				}},
				Strategy: nil,
				Connection: balancer.Connection{
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
			config: &balancer.Config{
				Nodes: balancer.Nodes{{
					Address: "root:Mastermaster123@tcp(127.0.0.1:3306)/mysql",
					Name:    "master",
				}},
				Strategy: func(balancer.Nodes) (balancer.Strategy, error) {
					return nil, fmt.Errorf("failed to bootstrap strategy")
				},
				Connection: balancer.Connection{
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
			config: &balancer.Config{
				Nodes: balancer.Nodes{{
					Address: "root:Mastermaster123@tcp(127.0.0.1:3306)/mysql",
					Name:    "master",
				}},
				Strategy: func(n balancer.Nodes) (balancer.Strategy, error) { return balancer.NewRoundRobinStrategy(n) },
				Connection: balancer.Connection{
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
		{
			name: "it tries to reconnect to all unavailable nodes",
			config: &balancer.Config{
				Nodes: balancer.Nodes{
					{
						Address: "root:Mastermaster123@tcp(127.0.0.1:3306)/mysql",
						Name:    "master",
					},
					{
						Address: "root:slaveslave123@tcp(127.0.0.1:3307)/mysql",
						Name:    "slave",
					},
				},
				Strategy: func(n balancer.Nodes) (balancer.Strategy, error) { return balancer.NewRoundRobinStrategy(n) },
				Connection: balancer.Connection{
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
					if dataSourceName != "root:Mastermaster123@tcp(127.0.0.1:3306)/mysql" {
						e := mock.ExpectPing()
						e.WillReturnError(nil)
					} else {
						e := mock.ExpectPing()
						e.WillReturnError(errors.New("kabum"))
					}

					return db, nil
				})

				return db, mock, patch
			},
			verify:  nopVerify,
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

			got, err := balancer.NewBalancer(tt.config)
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

func Test_watchNodeForReconnect(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	n := balancer.NewNode(db, "mocked", "root:slaveslave123@tcp(127.0.0.1:3307)/mysql")

	e := mock.ExpectPing()
	e.WillReturnError(nil)

	ctrl := gomock.NewController(t)
	st := mock_strategy.NewMockStrategy(ctrl)

	st.EXPECT().AddNode(n)
	defer ctrl.Finish()

	balancer.WatchNodeForReconnect(n, st)
	assert.NoError(t, err)
}

func TestNewNode(t *testing.T) {
	db, _, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	assert.NoError(t, err)
	defer db.Close()

	assert.NotNil(t, balancer.NewNode(db, "mocked", "root:slaveslave123@tcp(127.0.0.1:3307)/mysql"))
}
