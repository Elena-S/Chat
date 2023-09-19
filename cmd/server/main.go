package main

import (
	"context"
	_ "net/http/pprof"

	_ "github.com/Elena-S/Chat/db/migrations-go"
	"github.com/Elena-S/Chat/pkg/auth"
	"github.com/Elena-S/Chat/pkg/broker"
	"github.com/Elena-S/Chat/pkg/chats"
	"github.com/Elena-S/Chat/pkg/chats/messages"
	"github.com/Elena-S/Chat/pkg/conns"
	"github.com/Elena-S/Chat/pkg/database"
	"github.com/Elena-S/Chat/pkg/handlers"
	"github.com/Elena-S/Chat/pkg/httpsrv"
	"github.com/Elena-S/Chat/pkg/kafka"
	"github.com/Elena-S/Chat/pkg/logger"
	"github.com/Elena-S/Chat/pkg/redis"
	"github.com/Elena-S/Chat/pkg/secretsmng"
	"github.com/Elena-S/Chat/pkg/users"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
)

func main() {
	NewApp().Run()
}

func NewApp() *fx.App {
	return fx.New(
		auth.Module,
		secretsmng.Module,
		database.Module,
		kafka.Module,
		redis.Module,
		broker.Module,
		handlers.Module,
		httpsrv.Module,
		fx.Provide(
			logger.NewZapLogger,
			logger.New,
			context.Background,
			conns.NewManager,
			users.NewManager,
			chats.NewManager,
			messages.NewManager,
		),
		fx.WithLogger(func(logger *zap.Logger) fxevent.Logger {
			return &fxevent.ZapLogger{Logger: logger}
		}),
	)
}
