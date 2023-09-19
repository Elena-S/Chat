package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/Elena-S/Chat/pkg/chats"
	"github.com/Elena-S/Chat/pkg/chats/messages"
	"github.com/Elena-S/Chat/pkg/users"
	"golang.org/x/net/websocket"
)

func (h *RefTokenHandler) RefreshTokens(rw http.ResponseWriter, r *http.Request) (err error) {
	if !(r.Method == "GET" || r.Method == "POST") {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	rh := NewResponseHelper(rw, r)

	accessToken, refreshToken, err := rh.RetrieveTokens()
	if err != nil {
		return
	}

	tokens, err := h.oAuthManager.RefreshTokens(r.Context(), accessToken, refreshToken)
	if err != nil {
		return
	}
	return rh.SetTokens(tokens)
}

func (h *UserAboutHandler) UserAbout(rw http.ResponseWriter, r *http.Request) (err error) {
	if !(r.Method == "GET" || r.Method == "POST") {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	rh := NewResponseHelper(rw, r)

	userID, err := rh.GetUserID(h.oAuthManager)
	if err != nil {
		return
	}
	user, err := h.usersManager.Get(r.Context(), userID)
	if err != nil {
		return
	}
	return rh.WriteJSONContent(user)
}

func (h *ChatHandler) Search(rw http.ResponseWriter, r *http.Request) (err error) {
	if !(r.Method == "GET" || r.Method == "POST") {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	rh := NewResponseHelper(rw, r)

	userID, err := rh.GetUserID(h.oAuthManager)
	if err != nil {
		return
	}

	if err = r.ParseForm(); err != nil {
		return
	}

	chatArr, err := h.chatManager.Search(r.Context(), userID, r.Form.Get("phrase"))
	if err != nil {
		return
	}
	return rh.WriteJSONContent(chatArr)
}

func (h *ChatHandler) CreateChat(rw http.ResponseWriter, r *http.Request) (err error) {
	if r.Method != "POST" {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	rh := NewResponseHelper(rw, r)

	userID, err := rh.GetUserID(h.oAuthManager)
	if err != nil {
		return
	}

	defer func() {
		errClose := r.Body.Close()
		if err == nil {
			err = errClose
		}
	}()

	chat, err := h.chatManager.Register(r.Context(), r.Body, userID)
	if err != nil {
		return
	}
	return rh.WriteJSONContent(chat)
}

func (h *ChatHandler) ChatAbout(rw http.ResponseWriter, r *http.Request) (err error) {
	rh := NewResponseHelper(rw, r)

	userID, err := rh.GetUserID(h.oAuthManager)
	if err != nil {
		return
	}
	if err = r.ParseForm(); err != nil {
		return
	}
	chatID, err := chats.StringToID(r.Form.Get("chat_id"))
	if err != nil {
		return
	}
	chat, err := h.chatManager.Get(r.Context(), chatID, userID)
	if err != nil {
		return
	}
	return rh.WriteJSONContent(chat)
}

func (h *ChatHandler) ChatList(rw http.ResponseWriter, r *http.Request) (err error) {
	if !(r.Method == "GET" || r.Method == "POST") {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	rh := NewResponseHelper(rw, r)

	userID, err := rh.GetUserID(h.oAuthManager)
	if err != nil {
		return
	}

	chatArr, err := h.chatManager.List(r.Context(), userID)
	if err != nil {
		return
	}
	return rh.WriteJSONContent(chatArr)
}

func (h *HistoryHandler) ChatHistory(rw http.ResponseWriter, r *http.Request) (err error) {
	if !(r.Method == "GET" || r.Method == "POST") {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	rh := NewResponseHelper(rw, r)

	userID, err := rh.GetUserID(h.oAuthManager)
	if err != nil {
		return
	}

	if err = r.ParseForm(); err != nil {
		return
	}

	chatID, err := chats.StringToID(r.Form.Get("chat_id"))
	if err != nil {
		return
	}

	messageID, err := messages.StringToID(r.Form.Get("message_id"))
	if err != nil {
		return
	}

	history, err := h.messageManager.List(r.Context(), chatID, userID, messageID)
	if err != nil {
		return
	}
	return rh.WriteJSONContent(history)
}

func (wsh *WSHandler) WSConnection(ws *websocket.Conn) {
	var err error
	var userID users.UserID

	ctxLogger := wsh.logger.WithEventField("ws connection").With("remote addr", ws.Request().RemoteAddr)
	ctxLogger.Info("opened")

	defer func() {
		data := recover()
		ctxLogger.OnDefer("handlers", err, data, "closed")
	}()

	userID, err = NewRequestHelper(ws.Request()).GetUserID(wsh.oAuthManager)
	if err != nil {
		return
	}
	ctxLogger = ctxLogger.With("user id", userID)

	connNum, err := wsh.connsManager.Store(userID, ws)
	if err != nil {
		return
	}
	defer func() {
		errClose := wsh.connsManager.CloseAndDelete(userID, ws)
		if err == nil {
			err = errClose
		}
	}()
	ctxLogger = ctxLogger.With("connection number", connNum)

	go func() {
		<-wsh.context.Done()
		ws.Close()
	}()

	payload := map[any]any{}
	err = wsh.broker.Subscribe(wsh.context, userID.String(), wsh.MessageHandler(ws), payload)
	if err != nil {
		return
	}
	defer func() {
		errUnsub := wsh.broker.Unsubscribe(wsh.context, userID.String(), payload)
		if err == nil {
			err = errUnsub
		}
	}()

	ctxDB, cancelFuncDB := context.WithTimeout(wsh.context, time.Second*30)
	user, err := wsh.usersManager.Get(ctxDB, userID)
	cancelFuncDB()
	if err != nil {
		return
	}

	// err = ws.SetDeadline(time.Now().Add(time.Minute * 31))
	// if err != nil {
	// 	return
	// }

	for {
		var reply []byte
		errMsg := websocket.Message.Receive(ws, &reply)
		if errMsg != nil {
			ctxLogger.Error(errMsg.Error())
			if wsh.StopReceiving(errMsg) {
				break
			} else {
				continue
			}
		}
		message, errMsg := wsh.messageManager.Register(wsh.context, reply, user)
		if errMsg != nil {
			ctxLogger.With("message", reply).Error(errMsg.Error())
			continue
		}
		//TODO: err if msg would registered, but not sent to broker
		errMsg = wsh.SendToBroker(message)
		if errMsg != nil {
			ctxLogger.Error(errMsg.Error())
			continue
		}
	}
}

func (wsh *WSHandler) StopReceiving(err error) bool {
	if errNetOp, ok := err.(*net.OpError); ok && errNetOp.Timeout() {
		return true
	} else if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
		return true
	} else if wsh.context.Err() != nil {
		return true
	}
	return false
}

func (wsh *WSHandler) SendToBroker(message messages.Message) (err error) {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}

	errs := make([]error, len(message.Receivers()))
	i := 0
	for _, receiver := range message.Receivers() {
		err := wsh.broker.Publish(wsh.context, receiver.String(), data)
		if err != nil {
			errs[i] = err
			i++
		}
	}

	err = errors.Join(errs[:i]...)
	if err != nil {
		return fmt.Errorf("handlers: one or more errors occurred when sending a message to the broker, %w", err)
	}
	return
}

func (wsh *WSHandler) SendToReceiver(data []byte, ws *websocket.Conn) (err error) {
	ok, err := wsh.messageManager.IsActual(data)
	if err != nil || !ok {
		return
	}
	return websocket.Message.Send(ws, string(data))
}

func (wsh *WSHandler) MessageHandler(ws *websocket.Conn) func(data []byte) error {
	return func(data []byte) error { return wsh.SendToReceiver(data, ws) }
}
