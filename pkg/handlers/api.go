package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/bits"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/Elena-S/Chat/pkg/auth"
	"github.com/Elena-S/Chat/pkg/chats"
	"github.com/Elena-S/Chat/pkg/conns"
	"github.com/Elena-S/Chat/pkg/logger"
	"github.com/Elena-S/Chat/pkg/users"
	"golang.org/x/net/websocket"
)

func RefreshTokens(rw http.ResponseWriter, r *http.Request, ctxLogger logger.Logger) (err error) {
	if !(r.Method == "GET" || r.Method == "POST") {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	rh := NewResponseHelper(rw, r)

	accessToken, refreshToken, err := rh.RetrieveTokens()
	if err != nil {
		return
	}

	tokens, err := auth.OAuthManager.RefreshTokens(r.Context(), accessToken, refreshToken)
	if err != nil {
		return
	}
	return rh.SetTokens(tokens)
}

func UserAbout(rw http.ResponseWriter, r *http.Request, ctxLogger logger.Logger) (err error) {
	if !(r.Method == "GET" || r.Method == "POST") {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	rh := NewResponseHelper(rw, r)

	userID, err := rh.GetUserID()
	if err != nil {
		return
	}
	user, err := users.GetUserByID(userID)
	if err != nil {
		return
	}
	return rh.WriteJSONContent(user)
}

func Search(rw http.ResponseWriter, r *http.Request, ctxLogger logger.Logger) (err error) {
	if !(r.Method == "GET" || r.Method == "POST") {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	rh := NewResponseHelper(rw, r)

	userID, err := rh.GetUserID()
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
	return rh.WriteJSONContent(chatArr)
}

func CreateChat(rw http.ResponseWriter, r *http.Request, ctxLogger logger.Logger) (err error) {
	if r.Method != "POST" {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	rh := NewResponseHelper(rw, r)

	userID, err := rh.GetUserID()
	if err != nil {
		return
	}

	defer r.Body.Close()

	chat := new(chats.Chat)
	err = chat.Register(r.Context(), r.Body, userID)

	if err != nil {
		return
	}
	return rh.WriteJSONContent(chat)
}

func ChatAbout(rw http.ResponseWriter, r *http.Request, ctxLogger logger.Logger) (err error) {
	rh := NewResponseHelper(rw, r)

	userID, err := rh.GetUserID()
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
	chat, err := chats.GetChatInfoByID(uint(chatID), userID)
	if err != nil {
		return
	}
	return rh.WriteJSONContent(chat)
}

func ChatList(rw http.ResponseWriter, r *http.Request, ctxLogger logger.Logger) (err error) {
	if !(r.Method == "GET" || r.Method == "POST") {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	rh := NewResponseHelper(rw, r)

	userID, err := rh.GetUserID()
	if err != nil {
		return
	}

	chatArr, err := chats.List(userID)
	if err != nil {
		return
	}
	return rh.WriteJSONContent(chatArr)
}

func ChatHistory(rw http.ResponseWriter, r *http.Request, ctxLogger logger.Logger) (err error) {
	if !(r.Method == "GET" || r.Method == "POST") {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	rh := NewResponseHelper(rw, r)

	userID, err := rh.GetUserID()
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
	return rh.WriteJSONContent(history)
}

func WSConnection(ws *websocket.Conn) {
	var err error
	var userID uint
	var timer *time.Timer

	ctxLogger := logger.ChatLogger.WithEventField("ws connection").With("remote addr", ws.Request().RemoteAddr)
	ctxLogger.Info("Opened")

	defer func() {
		if err != nil {
			ctxLogger.Error(err.Error())
		}
		if data := recover(); data != nil {
			ctxLogger.Error(fmt.Sprintf("handlers: panic raised when handle websocket connection, %v", data))
		}
		ctxLogger.Info("Closed")
	}()

	defer func() {
		if timer != nil && timer.Stop() {
			timer.Reset(0)
		}
		if err := conns.Pool.CloseAndDelete(userID, ws); err != nil {
			ctxLogger.Error(err.Error())
		}
	}()

	userID, err = NewRequestHelper(ws.Request()).GetUserID()
	if err != nil {
		return
	}

	connNum, err := conns.Pool.Store(userID, ws)

	ctxLogger = ctxLogger.With("user id", userID).With("connection number", connNum)

	if err != nil {
		return
	}

	duration := time.Minute * 30
	timer = time.NewTimer(duration)

	go func() {
		<-timer.C
		if err := ws.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			ctxLogger.Error(err.Error())
		}
	}()

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
		if timer.Stop() {
			timer.Reset(duration)
		}

		message := new(chats.Message)
		errMsg = json.Unmarshal(reply, message)
		if errMsg != nil {
			ctxLogger.With("message", reply).Error(errMsg.Error())
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
	}
}
