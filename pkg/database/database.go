package database

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/Elena-S/Chat/pkg/logger"
	"github.com/Elena-S/Chat/pkg/srcmng"
	"github.com/lib/pq"
	"github.com/pressly/goose/v3"
)

var _ srcmng.SourceManager = (*dbInstance)(nil)
var DBI *dbInstance = new(dbInstance)

type dbInstance struct {
	db   *sql.DB
	once sync.Once
}

func (dbi *dbInstance) MustLaunch() {
	dbi.once.Do(func() {
		dbi.connect()
		dbi.init()
	})
}

func (dbi *dbInstance) Close() error {
	if dbi.db == nil {
		return nil
	}
	return dbi.db.Close()
}

func (dbi *dbInstance) connect() {
	var err error

	dsn := fmt.Sprintf(
		//needs config file
		"host=db user=%s password=%s dbname=%s port=5432 sslmode=disable TimeZone=Europe/Moscow",
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_NAME"),
	)

	ctxLogger := logger.ChatLogger.WithEventField("connection to the database").With("data source name", dsn)
	ctxLogger.Info("Start")

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		ctxLogger.Fatal(err.Error())
	}

	for i := 0; i < 3; i++ {
		err = db.Ping()
		if errors.Is(err, syscall.ECONNREFUSED) {
			time.Sleep(time.Second * 2)
			continue
		} else {
			break
		}
	}

	if err != nil {
		ctxLogger.Fatal(err.Error())
	}

	ctxLogger.Info("Finish")

	dbi.db = db
}

func (dbi *dbInstance) init() {

	//needs config file
	path := "../../db/migrations"

	ctxLogger := logger.ChatLogger.WithEventField("Init of the database").With("path", path)
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

func DB() *sql.DB {
	DBI.MustLaunch()
	return DBI.db
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
