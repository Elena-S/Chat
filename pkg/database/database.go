package database

import (
	"database/sql"
	"fmt"
	"os"
	"sync"

	"github.com/Elena-S/Chat/pkg/logger"
	"github.com/lib/pq"
	"github.com/pressly/goose/v3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	once sync.Once
	dbi  *dbInstance
)

func DB() *sql.DB {
	once.Do(func() {
		dbi = new(dbInstance)
		dbi.connect()
		dbi.init()
	})
	return dbi.db
}

type dbInstance struct {
	db *sql.DB
}

func (dbi *dbInstance) connect() {
	dsn := fmt.Sprintf(
		//needs config file
		"host=db user=%s password=%s dbname=%s port=5432 sslmode=disable TimeZone=Europe/Moscow",
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_NAME"),
	)

	ctxLogger := logger.Logger.With(
		logger.EventField("connection to the database"),
		zap.Field{
			Key:    "data source name",
			Type:   zapcore.StringType,
			String: dsn,
		})
	ctxLogger.Info("Start")

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		ctxLogger.Fatal(err.Error())
	}

	ctxLogger.Info("Finish")

	dbi.db = db
}

func (dbi *dbInstance) init() {

	//needs config file
	path := "../../db/migrations"

	ctxLogger := logger.Logger.With(
		logger.EventField("init of the database"),
		zap.Field{
			Key:    "path",
			Type:   zapcore.StringType,
			String: path,
		})

	ctxLogger.Info("Start")

	if err := goose.SetDialect("postgres"); err != nil {
		ctxLogger.Fatal(err.Error())
	}

	//needs config file
	goose.SetTableName("db_version")
	if err := goose.Up(dbi.db, path); err != nil {
		ctxLogger.Fatal(err.Error())
	}

	ctxLogger.Info("Finish")
}

func SerializationFailureError(err error) bool {
	if err == nil {
		return false
	}
	if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "40001" {
		return true
	}
	return false
}
