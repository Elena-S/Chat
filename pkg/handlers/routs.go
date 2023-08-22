package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/Elena-S/Chat/pkg/logger"
	"github.com/Elena-S/Chat/pkg/users"
	"golang.org/x/net/websocket"
)

func SetupRouts() {
	http.HandleFunc("/", handlerWithRedirect(Home, "Home request"))
	http.HandleFunc("/error", handlerWithWriteErrHeader(Error, "Error request"))
	http.HandleFunc("/authentication/login", handlerWithRedirect(Login, "Login request"))
	http.HandleFunc("/authentication/login/silent", handlerWithRedirect(SilentLogin, "Silent login request"))
	http.HandleFunc("/authentication/consent", handlerWithRedirect(Consent, "Consent request"))
	http.HandleFunc("/authentication/logout", handlerWithRedirect(Logout, "Logout request"))
	http.HandleFunc("/authentication/finish", handlerWithRedirect(FinishAuth, "Finish auth request"))
	http.HandleFunc("/authentication/finish/silent", handlerWithRedirect(FinishSilentAuth, "Finish silent auth request"))
	http.HandleFunc("/authentication/finish/silent/ok", handlerWithRedirect(SilentAuthOK, "Silent auth OK request"))
	http.HandleFunc("/authentication/refresh_tokens", handlerWithWriteErrHeader(RefreshTokens, "Refresh tokens request"))
	http.HandleFunc("/chat", handlerWithRedirect(Chat, "Chat request"))
	http.HandleFunc("/chat/search", handlerWithWriteErrHeader(Search, "Search request"))
	http.HandleFunc("/chat/list", handlerWithWriteErrHeader(ChatList, "Chat list request"))
	http.HandleFunc("/chat/history", handlerWithWriteErrHeader(ChatHistory, "Chat history request"))
	http.HandleFunc("/chat/create", handlerWithWriteErrHeader(CreateChat, "Create chat request"))
	http.HandleFunc("/chat/chat", handlerWithWriteErrHeader(ChatAbout, "Chat about request"))
	http.HandleFunc("/chat/user", handlerWithWriteErrHeader(UserAbout, "User about request"))
	http.Handle("/chat/ws", websocket.Handler(WSConnection))

	//NGINX
	http.Handle("/view/", http.FileServer(http.Dir("/usr/src/app")))
}

func handlerWithRedirect(handler func(rw http.ResponseWriter, r *http.Request, ctxLogger logger.Logger) (err error),
	event string) func(http.ResponseWriter, *http.Request) {
	return handlerWithChatLogger(handler, event, redirectToErrorPage)
}

func handlerWithWriteErrHeader(handler func(rw http.ResponseWriter, r *http.Request, ctxLogger logger.Logger) (err error),
	event string) func(http.ResponseWriter, *http.Request) {
	return handlerWithChatLogger(handler, event, writeErrorHeader)
}

func handlerWithChatLogger(handler func(rw http.ResponseWriter, r *http.Request, ctxLogger logger.Logger) (err error),
	event string,
	errorHandler func(rw http.ResponseWriter, r *http.Request, err error)) func(http.ResponseWriter, *http.Request) {
	return func(rw http.ResponseWriter, r *http.Request) {
		var err error
		ctxLogger := logger.ChatLogger.WithEventField(event)
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

		err = handler(rw, r, ctxLogger)
	}
}

func redirectToErrorPage(rw http.ResponseWriter, r *http.Request, err error) {
	http.Redirect(rw, r, os.Getenv("URL_ERROR"), http.StatusSeeOther)
}

func writeErrorHeader(rw http.ResponseWriter, r *http.Request, err error) {
	statusCode := http.StatusInternalServerError
	switch {
	case errors.Is(err, users.ErrInvalidCredentials), errors.Is(err, users.ErrInvalidLoginFormat):
		statusCode = http.StatusBadRequest
	case errors.Is(err, users.ErrUsrExists), errors.Is(err, users.ErrWrongCredentials):
		statusCode = http.StatusForbidden
	}
	http.Error(rw, err.Error(), statusCode)
}
