package main

import (
	"fmt"
	"net/http"

	_ "github.com/Elena-S/Chat/db/migrations-go"
	"github.com/Elena-S/Chat/pkg/broker"
	"github.com/Elena-S/Chat/pkg/database"
	"github.com/Elena-S/Chat/pkg/handlers"
	"github.com/Elena-S/Chat/pkg/kafka"
	"github.com/Elena-S/Chat/pkg/logger"
	"github.com/Elena-S/Chat/pkg/redis"
	"github.com/Elena-S/Chat/pkg/srcmng"
	"github.com/Elena-S/Chat/pkg/vault"
)

func main() {
	defer func() {
		ctxLogger := logger.ChatLogger.WithEventField("Stop of the server")
		if data := recover(); data != nil {
			ctxLogger.Error(fmt.Sprintf("main: panic raised, %v", data))
		} else {
			ctxLogger.Info("")
		}
		logger.ChatLogger.Sync()
	}()

	ctxLogger := logger.ChatLogger.WithEventField("Start of the server")
	ctxLogger.Info("")

	initComponenets()
	defer srcmng.SourceKeeper.CloseAll()

	handlers.SetupRouts()

	//needs config file
	if err := http.ListenAndServeTLS(":8000", "../../cert/certificate.crt", "../../cert/privateKey.key", nil); err != nil {
		ctxLogger.Fatal(err.Error())
	}
}

func initComponenets() {
	broker.MustAssign(kafka.Client)

	srcmng.SourceKeeper.Add(database.DBI)
	srcmng.SourceKeeper.Add(kafka.Client)
	srcmng.SourceKeeper.Add(redis.Client)
	srcmng.SourceKeeper.Add(vault.SecretStorage)
	srcmng.SourceKeeper.MustLaunchAll()
}
