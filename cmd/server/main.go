package main

import (
	"net/http"

	_ "github.com/Elena-S/Chat/db/migrations-go"
	"github.com/Elena-S/Chat/pkg/database"
	"github.com/Elena-S/Chat/pkg/logger"
	"github.com/Elena-S/Chat/pkg/routs"
)

func main() {
	defer finish()

	ctxLogger := logger.Logger.With(logger.EventField("Start of the server"))
	ctxLogger.Info("")

	db := database.DB()
	defer db.Close()

	routs.SetupRouts()

	//needs config file
	if err := http.ListenAndServe(":8000", nil); err != nil {
		ctxLogger.Fatal(err.Error())
	}
}

func finish() {
	ctxLogger := logger.Logger.With(logger.EventField("Stop of the server"))
	if data := recover(); data != nil {
		logger.ErrorPanic(ctxLogger, data)
	} else {
		ctxLogger.Info("")
	}
	logger.Logger.Sync()
}
