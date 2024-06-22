package postgres

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"testing"
	"time"
)

const (
	testTimeout = time.Second * 10
	testPGName  = "postgres_test"
)

var (
	testPG     *Postgres
	testDriver string
)

func TestMain(m *testing.M) {
	for _, driver := range []string{"postgres", "pgx"} {
		fmt.Println("==========")
		fmt.Printf("Driver: %s\n", driver)
		fmt.Println("==========")
		testDriver = driver

		var err error
		config := ConnectConfig{
			Driver:   driver,
			Username: "postgres",
			Password: "postgres",
			Host:     "localhost",
			Port:     "5432",
		}

		testPG, err = createDatabase(context.Background(), config, testPGName)
		if err != nil {
			log.Fatal(err)
		}

		code := m.Run()
		if err := dropDatabase(context.Background(), config, testPGName); err != nil {
			log.Fatal(err)
		}
		if code != 0 {
			os.Exit(code)
		}
	}
}

func TestGenerateURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		config ConnectConfig
		expect string
	}{
		{
			name: "simple configuration",
			config: ConnectConfig{
				Driver:   "postgres",
				Username: "username",
				Password: "password",
				Host:     "localhost",
				Port:     "5432",
				DBName:   "testing",
				SSLMode:  "disable",
			},
			expect: "postgres://username:password@localhost:5432/testing?sslmode=disable",
		},
		{
			name: "with search path",
			config: ConnectConfig{
				Driver:     "postgres",
				Username:   "username",
				Password:   "password",
				Host:       "localhost",
				Port:       "5432",
				DBName:     "testing",
				SSLMode:    "disable",
				SearchPath: "public",
			},
			expect: "postgres://username:password@localhost:5432/testing?sslmode=disable&search_path=public",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			url, _, err := test.config.DSN()
			if err != nil {
				t.Fatal(err)
			}
			if url != test.expect {
				t.Fatalf("expecting:\n%s\nbut got\n%s", test.expect, url)
			}
		})
	}
}

func TestConnect(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		config ConnectConfig
		dbName string
		setup  func(context.Context, ConnectConfig, string, *testing.T)
	}{
		{
			name: "without schema",
			config: ConnectConfig{
				Driver:   testDriver,
				Username: "postgres",
				Password: "postgres",
				Host:     "localhost",
				Port:     "5432",
			},
			dbName: "test_connect_1",
			setup: func(ctx context.Context, config ConnectConfig, name string, t *testing.T) {
				t.Helper()
				pg, err := createDatabase(ctx, config, name)
				if err != nil {
					t.Fatal(err)
				}
				if err := pg.Close(); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "with schema",
			config: ConnectConfig{
				Driver:     testDriver,
				Username:   "postgres",
				Password:   "postgres",
				Host:       "localhost",
				Port:       "5432",
				SearchPath: "testing2",
			},
			dbName: "test_connect_2",
			setup: func(ctx context.Context, config ConnectConfig, name string, t *testing.T) {
				t.Helper()
				pg, err := createDatabase(ctx, config, name)
				if err != nil {
					t.Fatal(err)
				}
				_, err = pg.Exec(ctx, "CREATE SCHEMA testing2")
				if err != nil {
					t.Fatal(err)
				}
				if err := pg.Close(); err != nil {
					t.Fatal(err)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			config := test.config
			config.DBName = test.dbName

			test.setup(context.Background(), test.config, test.dbName, t)
			pg, err := Connect(context.Background(), config)
			if err != nil {
				t.Fatal(err)
			}
			if err := pg.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func createDatabase(ctx context.Context, config ConnectConfig, name string) (*Postgres, error) {
	config.DBName = ""
	pg, err := Connect(ctx, config)
	if err != nil {
		return nil, err
	}
	defer pg.Close()

	query := fmt.Sprintf("CREATE DATABASE %s", name)
	_, err = pg.Exec(ctx, query)
	if err != nil && err != context.Canceled {
		// If we got an error it might be because the database is still exist.
		fmt.Println("DROPPING DATABASE")
		errDrop := dropDatabase(ctx, config, name)
		if errDrop != nil {
			return nil, errors.Join(err, errDrop)
		}
		_, err = pg.Exec(ctx, query)
		fmt.Println("GOt ERROR", err)
	}
	if err != nil {
		err = fmt.Errorf("failed to create database: %w", err)
		return nil, err
	}

	newConnConfig := config
	newConnConfig.DBName = name
	newConn, err := Connect(ctx, newConnConfig)
	if err != nil {
		return nil, err
	}
	return newConn, nil
}

func dropDatabase(ctx context.Context, config ConnectConfig, name string) error {
	config.DBName = ""
	pg, err := Connect(ctx, config)
	if err != nil {
		return err
	}
	defer pg.Close()

	query := fmt.Sprintf("DROP DATABASE %s", name)
	_, err = pg.Exec(ctx, query)
	if err != nil {
		err = fmt.Errorf("failed to drop database: %w", err)
	}
	return err
}
