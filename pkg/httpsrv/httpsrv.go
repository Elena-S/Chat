package httpsrv

import (
	"context"
	"net"
	"net/http"

	"github.com/Elena-S/Chat/pkg/logger"
	"go.uber.org/fx"
)

var Module = fx.Module("httpsrv",
	fx.Invoke(New),
)

type HTTPServerParams struct {
	fx.In
	fx.Lifecycle
	Logger *logger.Logger
}

func New(p HTTPServerParams) {
	//TODO: need config
	server := &http.Server{Addr: ":8000"}
	p.Lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) (err error) {
			listener, err := net.Listen("tcp", server.Addr)
			if err != nil {
				return
			}
			ctxLogger := p.Logger.WithEventField("server").With("address", server.Addr)
			ctxLogger.Info("start")
			go func() {
				var err error
				defer func() {
					data := recover()
					ctxLogger.OnDefer("httpsrv", err, data, "stop")
				}()
				//TODO: need config
				err = server.ServeTLS(listener, "../../cert/certificate.crt", "../../cert/privateKey.key")
			}()
			return
		},
		OnStop: func(ctx context.Context) error {
			return server.Shutdown(ctx)
		},
	})
}
