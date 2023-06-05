package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/bits"
	"net"
	"net/http"
	"strconv"

	"github.com/Elena-S/Chat/pkg/chats"
	"github.com/Elena-S/Chat/pkg/conns"
	"github.com/Elena-S/Chat/pkg/hydra"
	"github.com/Elena-S/Chat/pkg/logger"
	"github.com/Elena-S/Chat/pkg/users"
	"golang.org/x/net/websocket"
	"golang.org/x/oauth2"
)

func RefreshTokens(rw http.ResponseWriter, r *http.Request) {
	if !(r.Method == "GET" || r.Method == "POST") {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var err error

	ctxLogger := logger.Logger.With(logger.EventField("Refresh tokens request"))
	defer func() { setStatusOfError(rw, ctxLogger, err) }()

	accessToken, refreshToken, err := retrieveTokens(r)
	if err != nil {
		return
	}

	tokenInfo, response, err := hydra.PrivateClient().OAuth2Api.IntrospectOAuth2Token(r.Context()).Token(accessToken).Execute()
	if err != nil {
		err = fmt.Errorf("handlers: an error occured when calling OAuth2Api.IntrospectOAuth2Token: %w\nfull HTTP response: %v", err, response)
		return
	}

	if !(tokenInfo.GetActive() && tokenInfo.GetTokenUse() == hydra.TokenTypeAccess && tokenInfo.GetClientId() == hydra.OAuthConf.Config.ClientID) {
		err = errors.New("handlers: invalid access token")
		return
	}

	ctx := context.WithValue(r.Context(), oauth2.HTTPClient, hydra.OAuthConf.HTTPClient)
	tokens, err := hydra.OAuthConf.Config.Exchange(ctx, "", oauth2.SetAuthURLParam("grant_type", hydra.GrantTypeRefreshToken), oauth2.SetAuthURLParam(hydra.TokenTypeRefresh, refreshToken))
	if err != nil {
		return
	}

	setTokens(rw, tokens)
}

func UserAbout(rw http.ResponseWriter, r *http.Request) {
	if !(r.Method == "GET" || r.Method == "POST") {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var err error

	ctxLogger := logger.Logger.With(logger.EventField("User about request"))
	defer func() { setStatusOfError(rw, ctxLogger, err) }()

	userID, err := getUserID(r)
	if err != nil {
		return
	}
	user, err := users.GetUserByID(userID)
	if err != nil {
		return
	}
	writeJSONContent(rw, user)
}

func Search(rw http.ResponseWriter, r *http.Request) {
	if !(r.Method == "GET" || r.Method == "POST") {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var err error

	ctxLogger := logger.Logger.With(logger.EventField("Search request"))
	defer func() { setStatusOfError(rw, ctxLogger, err) }()

	userID, err := getUserID(r)
	if err != nil {
		return
	}

	if err = r.ParseForm(); err != nil {
		return
	}

	chatArr, err := chats.Search(userID, r.Form.Get("phrase"))
	if err != nil {
		return
	}
	writeJSONContent(rw, chatArr)
}

func CreateChat(rw http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var err error

	ctxLogger := logger.Logger.With(logger.EventField("Create chat request"))
	defer func() { setStatusOfError(rw, ctxLogger, err) }()

	userID, err := getUserID(r)
	if err != nil {
		return
	}

	defer r.Body.Close()

	chat := new(chats.Chat)
	err = chat.Register(r.Body, userID)

	if err != nil {
		return
	}
	writeJSONContent(rw, chat)
}

func ChatAbout(rw http.ResponseWriter, r *http.Request) {
	var err error

	ctxLogger := logger.Logger.With(logger.EventField("Chat about request"))
	defer func() { setStatusOfError(rw, ctxLogger, err) }()

	userID, err := getUserID(r)
	if err != nil {
		return
	}

	if err = r.ParseForm(); err != nil {
		return
	}
	chatID, err := strconv.ParseUint(r.Form.Get("chat_id"), 10, bits.UintSize)
	chat, err := chats.GetChatInfoByID(uint(chatID), userID)
	if err != nil {
		return
	}
	writeJSONContent(rw, chat)
}

func ChatList(rw http.ResponseWriter, r *http.Request) {
	if !(r.Method == "GET" || r.Method == "POST") {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var err error

	ctxLogger := logger.Logger.With(logger.EventField("Chat list request"))
	defer func() { setStatusOfError(rw, ctxLogger, err) }()

	userID, err := getUserID(r)
	if err != nil {
		return
	}

	chatArr, err := chats.List(userID)
	if err != nil {
		return
	}
	writeJSONContent(rw, chatArr)
}

func ChatHistory(rw http.ResponseWriter, r *http.Request) {
	if !(r.Method == "GET" || r.Method == "POST") {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var err error

	ctxLogger := logger.Logger.With(logger.EventField("Chat history request"))
	defer func() { setStatusOfError(rw, ctxLogger, err) }()

	userID, err := getUserID(r)
	if err != nil {
		return
	}

	if err = r.ParseForm(); err != nil {
		return
	}

	chatID, err := strconv.ParseUint(r.Form.Get("chat_id"), 10, bits.UintSize)
	if err != nil {
		return
	}

	messageID, err := strconv.ParseUint(r.Form.Get("message_id"), 10, bits.UintSize)
	if err != nil {
		return
	}

	history := new(chats.History)
	err = history.Fill(userID, uint(chatID), uint(messageID))
	if err != nil {
		return
	}
	writeJSONContent(rw, history)
}

func SendMessage(ws *websocket.Conn) {
	var err error
	var userID uint
	// var timer *time.Timer

	ctxLogger := logger.NewLoggerWS(logger.Logger, ws.Request().RemoteAddr)
	ctxLogger.Info("Opened")

	defer func() {
		if err != nil {
			ctxLogger.Error(err.Error())
		}
		ctxLogger.Info("Closed")
	}()
	defer func() {
		// if timer != nil {
		// 	timer.Reset(time.Duration(0))
		// }
		err = conns.Pool.CloseAndDelete(userID, ws)
	}()

	r := ws.Request()

	userID, err = getUserID(r)
	if err != nil {
		return
	}

	connNum, err := conns.Pool.Store(userID, ws)

	ctxLogger.SetUserID(userID)
	ctxLogger.SetNumConn(connNum)

	if err != nil {
		return
	}

	// duration := time.Hour * 12
	// timer = time.NewTimer(duration)

	// go func() {
	// 	<-timer.C
	// 	conns.Pool.CloseAndDelete(userID, ws)
	// }()

	for {
		var reply []byte
		errMsg := websocket.Message.Receive(ws, &reply)

		// message := new(chats.Message)
		// errMsg := websocket.JSON.Receive(ws, message)
		if errMsg != nil {
			if errors.Is(errMsg, io.EOF) || errors.Is(errMsg, net.ErrClosed) {
				ctxLogger.Error(errMsg.Error())
				break
			} else {
				ctxLogger.Error(errMsg.Error())
				continue
			}
		}

		message := new(chats.Message)
		errMsg = json.Unmarshal(reply, message)
		if errMsg != nil {
			ctxLogger.SetMessage(reply)
			ctxLogger.Error(errMsg.Error())
			continue
		}

		errMsg = message.Register(userID)
		if errMsg != nil {
			ctxLogger.Error(errMsg.Error())
			continue
		}

		errMsg = message.Chat().SendMessage(*message)
		if errMsg != nil {
			ctxLogger.Error(errMsg.Error())
			continue
		}

		// timer.Reset(duration)
	}
}
