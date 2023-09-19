package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/Elena-S/Chat/pkg/logger"
	"github.com/lib/pq"
	"github.com/pressly/goose/v3"
	"go.uber.org/fx"
)

var Module = fx.Module("database",
	fx.Provide(
		NewRepository,
	),
	fx.Invoke(registerFunc),
)

type Repository struct {
	*sql.DB
	migrationPath    string
	dialect          string
	versionTableName string
	dsn              string
	logger           *logger.Logger
}

type RepositoryParams struct {
	fx.In
	Logger *logger.Logger
}

func NewRepository(p RepositoryParams) (repo *Repository, err error) {
	repo = &Repository{}
	repo.logger = p.Logger
	//TODO: need config
	repo.dialect = "postgres"
	repo.dsn = fmt.Sprintf(
		"host=db user=%s password=%s dbname=%s port=5432 sslmode=disable TimeZone=Europe/Moscow",
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_NAME"),
	)
	repo.versionTableName = "db_version"
	repo.migrationPath = "../../db/migrations"
	repo.DB, err = sql.Open(repo.dialect, repo.dsn)
	if err != nil {
		return nil, err
	}
	return
}

func registerFunc(lc fx.Lifecycle, r *Repository) {
	lc.Append(fx.Hook{
		OnStart: r.connect,
		OnStop:  func(context.Context) error { return r.DB.Close() },
	})
}

func (r *Repository) connect(ctx context.Context) (err error) {
	ctxLogger := r.logger.WithEventField("connection to the database").With("data source name", r.dsn)
	ctxLogger.Info("start")
	defer ctxLogger.OnDefer("database", err, nil, "finish")

	if err = r.ping(ctx); err != nil {
		return
	}

	return r.migrate()
}

func (r *Repository) ping(ctx context.Context) (err error) {
	for i := 0; i < 3; i++ {
		err = r.DB.Ping()
		if errors.Is(err, syscall.ECONNREFUSED) && ctx.Err() == nil {
			time.Sleep(time.Second * 2)
			continue
		} else {
			break
		}
	}
	return
}

func (r *Repository) migrate() (err error) {
	if err = goose.SetDialect(r.dialect); err != nil {
		return
	}
	goose.SetTableName(r.versionTableName)
	return goose.Up(r.DB, r.migrationPath)
}

func (r *Repository) RollbackTx(tx *sql.Tx, err error) error {
	errTx := tx.Rollback()
	if err == nil && errTx != sql.ErrTxDone {
		return errTx
	}
	return err
}

func (r *Repository) SerializationFailureError(err error) bool {
	if err == nil {
		return false
	}
	if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "40001" {
		return true
	}
	return false
}
