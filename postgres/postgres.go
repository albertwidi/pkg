// Postgres is a compatibility layer between pgx and database/sql.
// This package also provide a pure pgx object if pgx is used.

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	_ "github.com/lib/pq"
	"go.opentelemetry.io/otel/trace"
)

const (
	defaultMaxOpenConns = 10
)

// ConnectConfig stores the configuration to create a new connection to PostgreSQL database.
type ConnectConfig struct {
	Driver       string
	Username     string
	Password     string
	Host         string
	Port         string
	DBName       string
	SearchPath   string
	SSLMode      string
	MaxOpenConns int
	Tracer       trace.Tracer
}

func (c *ConnectConfig) copy() ConnectConfig {
	return *c
}

func (c *ConnectConfig) validate() error {
	if c.Username == "" {
		return errors.New("postgres: username cannot be empty")
	}
	if c.Password == "" {
		return errors.New("postgres: password cannot be empty")
	}
	if c.Host == "" {
		return errors.New("postgres: host cannot be empty")
	}
	if c.Port == "" {
		return errors.New("postgres: port cannot be empty")
	}

	switch c.Driver {
	case "postgres", "libpq", "pgx":
	default:
		return fmt.Errorf("postgres: driver %s is not supported", c.Driver)
	}
	// Normalize the driver from 'libpq' and other drivers, because we will only support 'postgres'.
	if c.Driver == "libpq" {
		c.Driver = "postgres"
	}
	if c.SSLMode == "" {
		c.SSLMode = "disable"
	}

	// Overrides values.
	if c.MaxOpenConns == 0 {
		c.MaxOpenConns = defaultMaxOpenConns
	}
	return nil
}

// DSN returns the PostgreSQL DSN.
//
//	For example: postgres://username:password@localhost:5432/mydb?sslmode=false.
func (c *ConnectConfig) DSN() (url string, dsn map[string]string, err error) {
	url = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s", c.Username, c.Password, c.Host, c.Port, c.DBName, c.SSLMode)
	if c.SearchPath != "" {
		url = url + "&searchPath=" + c.SearchPath
	}
	dsn, err = PostgresDSN(url)
	return
}

type Postgres struct {
	config *ConnectConfig

	db  *sql.DB
	pgx *pgxpool.Pool
	tx  Transaction

	// searchPathMu protects set of the searchpath.
	searchPathMu sync.RWMutex
	searchPath   string // default schema separated by comma.
	// closeMu protects closing postgres connection concurrently.
	closeMu sync.Mutex
	closed  bool

	// originConn is the origin connection of a fork. This connection will be used in the case
	// of closing a connection and destroying the new schema.
	originConn *Postgres
	// cloneOnce is used to ensure the clone.sql to be applied once in a database.
	// The forked connection will use the same cloneOnce from previous so it won't
	// apply the function again.
	cloneOnce *sync.Once
}

// InTransaction returns whether the postgres object is currently in transaction or not. The information need to be
// exposed via a function because we don't want to expose the 'tx' object.
func (p *Postgres) InTransaction() bool {
	return p.tx != nil
}

// Config returns the copy of connection configuration.
func (p *Postgres) Config() ConnectConfig {
	return *p.config
}

// Connect returns connected Postgres object.
func Connect(ctx context.Context, connConfig ConnectConfig) (*Postgres, error) {
	config := &connConfig
	if err := config.validate(); err != nil {
		return nil, err
	}

	var (
		db    *sql.DB
		pgxdb *pgxpool.Pool
		err   error
	)

	url, _, err := config.DSN()
	if err != nil {
		return nil, err
	}

	switch config.Driver {
	case "postgres":
		db, err = sql.Open(config.Driver, url)
		if err != nil {
			return nil, err
		}
		db.SetMaxOpenConns(config.MaxOpenConns)
	case "pgx":
		poolConfig, err := pgxpool.ParseConfig(url)
		if err != nil {
			return nil, err
		}
		poolConfig.MaxConns = int32(config.MaxOpenConns)
		// Use the open telemetry tracer if tracer is not empty.
		if config.Tracer != nil {
			poolConfig.ConnConfig.Tracer = &PgxQueryTracer{
				dbInfo: struct {
					user    string
					host    string
					port    string
					dbName  string
					sslMode string
				}{
					user:    config.Username,
					host:    config.Host,
					port:    config.Port,
					dbName:  config.DBName,
					sslMode: config.SSLMode,
				},
			}
		}

		pgxdb, err = pgxpool.NewWithConfig(context.Background(), poolConfig)
		if err != nil {
			return nil, err
		}
	}

	p := &Postgres{
		config:     config,
		db:         db,
		pgx:        pgxdb,
		searchPath: "public",
		cloneOnce:  &sync.Once{},
	}
	return p, nil
}

func (p *Postgres) Query(ctx context.Context, query string, params ...any) (*RowsCompat, error) {
	return p.query(ctx, query, params...)
}

func (p *Postgres) query(ctx context.Context, query string, params ...any) (*RowsCompat, error) {
	if p.pgx != nil {
		rows, err := p.pgx.Query(ctx, query, params...)
		if err != nil {
			return nil, err
		}
		return &RowsCompat{pgxRows: rows}, err
	}
	rows, err := p.db.QueryContext(ctx, query, params...)
	if err != nil {
		return nil, err
	}
	return &RowsCompat{rows: rows}, nil
}

func (p *Postgres) RunQuery(ctx context.Context, query string, f func(*RowsCompat) error, params ...any) error {
	rows, err := p.query(ctx, query, params...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := f(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (p *Postgres) QueryRow(ctx context.Context, query string, params ...any) *RowCompat {
	if p.pgx != nil {
		row := p.pgx.QueryRow(ctx, query, params...)
		return &RowCompat{pgxRow: row}
	}
	row := p.db.QueryRowContext(ctx, query, params...)
	return &RowCompat{row: row}
}

func (p *Postgres) Exec(ctx context.Context, query string, params ...any) (*ExecResultCompat, error) {
	if p.pgx != nil {
		tag, err := p.pgx.Exec(ctx, query, params...)
		if err != nil {
			return nil, err
		}
		return &ExecResultCompat{pgxResult: tag}, nil
	}
	result, err := p.db.ExecContext(ctx, query, params...)
	if err != nil {
		return nil, err
	}
	return &ExecResultCompat{result: result}, nil
}

// Transaction interface ensure the pgx and sql/db tx object is compatible so we can use them both inside
// the Postgres object.
type Transaction interface {
	Rollback() error
	Commit() error
}

func (p *Postgres) Transact(ctx context.Context, iso sql.IsolationLevel, txFunc func(*Postgres) error) error {
	err := p.transact(ctx, iso, txFunc)
	return err
}

func (p *Postgres) transact(ctx context.Context, iso sql.IsolationLevel, txFunc func(*Postgres) error) error {
	if p.InTransaction() {
		return errors.New("a DB Transact function was called on a DB already in a transaction")
	}

	var tx Transaction
	var err error

	if p.pgx != nil {
		var pgxTx pgx.Tx
		pgxTx, err = p.pgx.BeginTx(ctx, pgx.TxOptions{IsoLevel: sqlIsoLevelToPgxIsoLevel(iso)})
		if err != nil {
			return err
		}
		tx = &TransactCompat{pgxTx: pgxTx}
	} else {
		tx, err = p.db.BeginTx(ctx, &sql.TxOptions{Isolation: iso})
		if err != nil {
			return err
		}
	}

	// Create a new copy of Postgres and add the transaction object inside the object. This will make InTransaction() check to be true.
	newPG := &Postgres{
		config:     p.config,
		closed:     p.closed,
		searchPath: p.searchPath,
		db:         p.db,
		pgx:        p.pgx,
		tx:         tx,
	}
	err = txFunc(newPG)
	if err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			// Rollback error is a different error, join the error with the actual error.
			err = errors.Join(err, rollbackErr)
		}
		return fmt.Errorf("failed to rollback: %w", err)
	}
	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}
	return nil
}

func (p *Postgres) Prepare(ctx context.Context, query string) (*StmtCompat, error) {
	if p.pgx != nil {
		return &StmtCompat{sql: query, pgxdb: p.pgx}, nil
	}
	stmt, err := p.db.PrepareContext(ctx, query)
	if err != nil {
		return nil, err
	}
	return &StmtCompat{stmt: stmt}, nil
}

func (p *Postgres) Ping(ctx context.Context) error {
	if p.pgx != nil {
		return p.pgx.Ping(ctx)
	}
	return p.db.PingContext(ctx)
}

// setDefaultSearchPath sets the default schema for the current connection.
func (p *Postgres) setDefaultSearchPath(ctx context.Context, schemaName string) error {
	query := fmt.Sprintf("SET search_path TO %s;", schemaName)
	_, err := p.Exec(context.Background(), query)
	if err != nil {
		return err
	}
	p.searchPathMu.Lock()
	p.searchPath = schemaName
	p.searchPathMu.Unlock()
	return nil
}

// SearchPath returns list of search path.
func (p *Postgres) SearchPath() []string {
	p.searchPathMu.RLock()
	paths := strings.Split(p.searchPath, ",")
	p.searchPathMu.RUnlock()
	return paths
}

func (p *Postgres) Close() (err error) {
	p.closeMu.Lock()
	if p.closed {
		p.closeMu.Unlock()
		return nil
	}

	defer func() {
		if err != nil {
			p.closeMu.Unlock()
			return
		}

		p.closed = true
		clean(p)
		p.closeMu.Unlock()
	}()

	if p.pgx != nil {
		p.pgx.Close()
		return
	}
	err = p.db.Close()
	return
}

// clean cleans the schema by deleting the schema if we know the current connection is forked.
// The reason of why we clean the schema here is, usually the new schema is used in a
// local function test and not via global variable. So this means when the connection is closed
// we don't need the schema anymore as our test have been completed.
func clean(p *Postgres) {
	if !testing.Testing() {
		return
	}
	if p.originConn == nil {
		return
	}

	var connErr error
	var newConn bool

	conn := p.originConn
	if conn.closed {
		conf := *p.originConn.config
		conn, connErr = Connect(context.Background(), conf)
		if connErr != nil {
			return
		}
		newConn = true
	}
	// Do best effort attempt to drop the schema.
	dropSchemaQuery := fmt.Sprintf("DROP SCHEMA %s CASCADE;", p.searchPath)
	conn.Exec(context.Background(), dropSchemaQuery)
	if newConn {
		_ = conn.Close()
	}
}

// Sometimes other libraries require us to use the stdlib database. So we provide a function to do so.
func (p *Postgres) StdlibDB() *sql.DB {
	if p.db != nil {
		return p.db
	}
	// The pgx version will create a whole new connection instead of using the current one.
	copyConf := p.pgx.Config().Copy()
	return stdlib.OpenDB(*copyConf.ConnConfig)
}
