package main

import (
	"net/http"

	_ "github.com/Elena-S/Chat/db/migrations-go"
	"github.com/Elena-S/Chat/pkg/database"
	"github.com/Elena-S/Chat/pkg/hydra"
	"github.com/Elena-S/Chat/pkg/logger"
	"github.com/Elena-S/Chat/pkg/routs"
	"github.com/Elena-S/Chat/pkg/vault"
)

func main() {
	defer finish()

	ctxLogger := logger.Logger.With(logger.EventField("Start of the server"))
	ctxLogger.Info("")

	db := database.DB()
	vault.Client()

	defer func() {
		err := db.Close()
		if err != nil {
			ctxLogger.Error(err.Error())
		}
		err = hydra.StatesStorage.Close()
		if err != nil {
			ctxLogger.Error(err.Error())
		}
	}()

	routs.SetupRouts()

	//needs config file
	if err := http.ListenAndServeTLS(":8000", "../../cert/certificate.crt", "../../cert/privateKey.key", nil); err != nil {
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
