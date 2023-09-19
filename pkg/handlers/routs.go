package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/Elena-S/Chat/pkg/auth"
	"github.com/Elena-S/Chat/pkg/broker"
	"github.com/Elena-S/Chat/pkg/chats"
	"github.com/Elena-S/Chat/pkg/chats/messages"
	"github.com/Elena-S/Chat/pkg/conns"
	"github.com/Elena-S/Chat/pkg/logger"
	"github.com/Elena-S/Chat/pkg/users"
	"go.uber.org/fx"
	"golang.org/x/net/websocket"
)

var Module = fx.Module("handlers",
	fx.Provide(
		NewUserAboutHandler,
		NewChatHandler,
		NewHistoryHandler,
		NewRefTokenHandler,
		NewWSHandler,
		NewAuthHandler,
		NewRoutHandler,
		NewRouter,
	),
	fx.Invoke(registerFunc),
)

type WSHandler struct {
	oAuthManager   *auth.Manager
	connsManager   *conns.Manager
	usersManager   *users.Manager
	chatsManager   *chats.Manager
	messageManager *messages.Manager
	broker         *broker.BrokerClient
	context        context.Context
	cancelFunc     context.CancelFunc
	logger         *logger.Logger
}

type WSHandlerParams struct {
	fx.In
	fx.Lifecycle
	OAuthManager   *auth.Manager
	ConnsManager   *conns.Manager
	UsersManager   *users.Manager
	ChatsManager   *chats.Manager
	MessageManager *messages.Manager
	Broker         *broker.BrokerClient
	Logger         *logger.Logger
	Context        context.Context
}

func NewWSHandler(p WSHandlerParams) *WSHandler {
	wsh := &WSHandler{
		oAuthManager:   p.OAuthManager,
		connsManager:   p.ConnsManager,
		usersManager:   p.UsersManager,
		chatsManager:   p.ChatsManager,
		messageManager: p.MessageManager,
		broker:         p.Broker,
		logger:         p.Logger,
	}
	wsh.context, wsh.cancelFunc = context.WithCancel(p.Context)
	p.Lifecycle.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			wsh.cancelFunc()
			return nil
		},
	})
	return wsh
}

type RoutHandler struct {
	oAuthManager *auth.Manager
}

type RoutHandlerParams struct {
	fx.In
	OAuthManager *auth.Manager
}

func NewRoutHandler(p RoutHandlerParams) *RoutHandler {
	rh := &RoutHandler{
		oAuthManager: p.OAuthManager,
	}
	return rh
}

type AuthHandler struct {
	oAuthManager *auth.Manager
	usersManager *users.Manager
}

type AuthHandlerParams struct {
	fx.In
	OAuthManager *auth.Manager
	UsersManager *users.Manager
}

func NewAuthHandler(p AuthHandlerParams) *AuthHandler {
	ah := &AuthHandler{
		oAuthManager: p.OAuthManager,
		usersManager: p.UsersManager,
	}
	return ah
}

type RefTokenHandler struct {
	oAuthManager *auth.Manager
}

type RefTokenHandlerParams struct {
	fx.In
	OAuthManager *auth.Manager
}

func NewRefTokenHandler(p RefTokenHandlerParams) *RefTokenHandler {
	ah := &RefTokenHandler{
		oAuthManager: p.OAuthManager,
	}
	return ah
}

type UserAboutHandler struct {
	usersManager *users.Manager
	oAuthManager *auth.Manager
}

type UserAboutHandlerParams struct {
	fx.In
	OAuthManager *auth.Manager
	UsersManager *users.Manager
}

func NewUserAboutHandler(p UserAboutHandlerParams) *UserAboutHandler {
	ah := &UserAboutHandler{
		oAuthManager: p.OAuthManager,
		usersManager: p.UsersManager,
	}
	return ah
}

type ChatHandler struct {
	chatManager  *chats.Manager
	oAuthManager *auth.Manager
}

type ChatHandlerParams struct {
	fx.In
	OAuthManager *auth.Manager
	ChatManager  *chats.Manager
}

func NewChatHandler(p ChatHandlerParams) *ChatHandler {
	h := &ChatHandler{
		oAuthManager: p.OAuthManager,
		chatManager:  p.ChatManager,
	}
	return h
}

type HistoryHandler struct {
	messageManager *messages.Manager
	oAuthManager   *auth.Manager
}

type HistoryHandlerParams struct {
	fx.In
	OAuthManager   *auth.Manager
	MessageManager *messages.Manager
}

func NewHistoryHandler(p HistoryHandlerParams) *HistoryHandler {
	h := &HistoryHandler{
		oAuthManager:   p.OAuthManager,
		messageManager: p.MessageManager,
	}
	return h
}

type RouterParams struct {
	fx.In
	*RoutHandler
	*AuthHandler
	*UserAboutHandler
	*ChatHandler
	*HistoryHandler
	*RefTokenHandler
	*WSHandler
	*logger.Logger
}
type Router struct {
	logger           *logger.Logger
	routHandler      *RoutHandler
	authHandler      *AuthHandler
	userAboutHandler *UserAboutHandler
	chatHandler      *ChatHandler
	historyHandler   *HistoryHandler
	refTokenHandler  *RefTokenHandler
	wsHandler        *WSHandler
}

func NewRouter(p RouterParams) *Router {
	return &Router{
		logger:           p.Logger,
		routHandler:      p.RoutHandler,
		authHandler:      p.AuthHandler,
		userAboutHandler: p.UserAboutHandler,
		chatHandler:      p.ChatHandler,
		historyHandler:   p.HistoryHandler,
		refTokenHandler:  p.RefTokenHandler,
		wsHandler:        p.WSHandler,
	}
}

func registerFunc(lc fx.Lifecycle, router *Router) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			router.SetupRouts()
			return nil
		},
	})
}

func (router *Router) SetupRouts() {
	http.HandleFunc("/", router.handlerWithRedirect(router.routHandler.Home, "home request"))
	http.HandleFunc("/error", router.handlerWithWriteErrHeader(router.routHandler.Error, "error request"))
	http.HandleFunc("/authentication/login", router.handlerWithRedirect(router.authHandler.Login, "login request"))
	http.HandleFunc("/authentication/login/silent", router.handlerWithRedirect(router.routHandler.SilentLogin, "silent login request"))
	http.HandleFunc("/authentication/consent", router.handlerWithRedirect(router.routHandler.Consent, "consent request"))
	http.HandleFunc("/authentication/logout", router.handlerWithRedirect(router.routHandler.Logout, "logout request"))
	http.HandleFunc("/authentication/finish", router.handlerWithRedirect(router.routHandler.FinishAuth, "finish auth request"))
	http.HandleFunc("/authentication/finish/silent", router.handlerWithRedirect(router.routHandler.FinishSilentAuth, "finish silent auth request"))
	http.HandleFunc("/authentication/finish/silent/ok", router.handlerWithRedirect(router.routHandler.SilentAuthOK, "silent auth OK request"))
	http.HandleFunc("/authentication/refresh_tokens", router.handlerWithWriteErrHeader(router.refTokenHandler.RefreshTokens, "refresh tokens request"))
	http.HandleFunc("/chat", router.handlerWithRedirect(router.routHandler.Chat, "chat request"))
	http.HandleFunc("/chat/search", router.handlerWithWriteErrHeader(router.chatHandler.Search, "search request"))
	http.HandleFunc("/chat/list", router.handlerWithWriteErrHeader(router.chatHandler.ChatList, "chat list request"))
	http.HandleFunc("/chat/history", router.handlerWithWriteErrHeader(router.historyHandler.ChatHistory, "chat history request"))
	http.HandleFunc("/chat/create", router.handlerWithWriteErrHeader(router.chatHandler.CreateChat, "create chat request"))
	http.HandleFunc("/chat/chat", router.handlerWithWriteErrHeader(router.chatHandler.ChatAbout, "chat about request"))
	http.HandleFunc("/chat/user", router.handlerWithWriteErrHeader(router.userAboutHandler.UserAbout, "user about request"))
	http.Handle("/chat/ws", websocket.Handler(router.wsHandler.WSConnection))

	//TODO: NGINX
	http.Handle("/view/", http.FileServer(http.Dir("/usr/src/app")))
}

func (router *Router) handlerWithRedirect(handler func(rw http.ResponseWriter, r *http.Request) (err error),
	event string) func(http.ResponseWriter, *http.Request) {
	return router.handlerWithLogger(handler, event, redirectToErrorPage)
}

func (router *Router) handlerWithWriteErrHeader(handler func(rw http.ResponseWriter, r *http.Request) (err error),
	event string) func(http.ResponseWriter, *http.Request) {
	return router.handlerWithLogger(handler, event, writeErrorHeader)
}

func (router *Router) handlerWithLogger(handler func(rw http.ResponseWriter, r *http.Request) (err error),
	event string, errorHandler ErrorHandler) func(http.ResponseWriter, *http.Request) {
	return func(rw http.ResponseWriter, r *http.Request) {
		var err error
		ctxLogger := router.logger.WithEventField(event)
		ctxLogger.Info("start")
		defer ctxLogger.Info("finish")
		defer func() {
			if err == nil {
				data := recover()
				if data == nil {
					return
				}
				if dataValue, ok := data.(error); ok {
					err = fmt.Errorf("handlers: panic raised when executing %s, data %w", event, dataValue)
				} else {
					err = fmt.Errorf("handlers: panic raised when executing %s, data %v", event, dataValue)
				}
			}

			ctxLogger.Error(err.Error())
			ctxLogger.Sync()
			errorHandler(rw, r, err)
		}()

		err = handler(rw, r)
	}
}

type ErrorHandler func(rw http.ResponseWriter, r *http.Request, err error)

var redirectToErrorPage ErrorHandler = func(rw http.ResponseWriter, r *http.Request, err error) {
	//TODO: need config
	http.Redirect(rw, r, os.Getenv("URL_ERROR"), http.StatusSeeOther)
}

var writeErrorHeader ErrorHandler = func(rw http.ResponseWriter, r *http.Request, err error) {
	statusCode := http.StatusInternalServerError
	switch {
	case errors.Is(err, users.ErrInvalidCredentials), errors.Is(err, users.ErrInvalidPhoneFormat):
		statusCode = http.StatusBadRequest
	case errors.Is(err, users.ErrExists), errors.Is(err, users.ErrWrongCredentials):
		statusCode = http.StatusForbidden
	}
	http.Error(rw, err.Error(), statusCode)
}
