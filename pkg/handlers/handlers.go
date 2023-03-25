package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/Elena-S/Chat/pkg/chats"
	"github.com/Elena-S/Chat/pkg/conns"
	"github.com/Elena-S/Chat/pkg/logger"
	"github.com/Elena-S/Chat/pkg/users"
	"golang.org/x/net/websocket"
)

type FsAccess struct {
	Handler http.Handler
}

func (f *FsAccess) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	if !(r.URL.Path == "/" || strings.HasPrefix(r.URL.Path, "/scripts") || strings.HasPrefix(r.URL.Path, "/style")) {
		_, err := getUser(r)
		if err != nil {
			http.Redirect(rw, r, "/", http.StatusSeeOther)
			return
		}
	}

	f.Handler.ServeHTTP(rw, r)
}

func Authorize(rw http.ResponseWriter, r *http.Request) {
	var err error

	ctxLogger := logger.Logger.With(logger.EventField("Authorization request"))
	defer func() { setStatusOfError(rw, ctxLogger, err) }()

	if err = r.ParseForm(); err != nil {
		return
	}

	login := r.PostForm.Get("login")
	pwd := r.PostForm.Get("pwd")

	user := new(users.User)
	err = user.Authorize(login, pwd)
	if err != nil {
		return
	}

	err = setToken(*user, r, rw)
	if err != nil {
		return
	}

	http.Redirect(rw, r, "/chat/chat.html", http.StatusSeeOther)
}

func Register(rw http.ResponseWriter, r *http.Request) {
	var err error

	ctxLogger := logger.Logger.With(logger.EventField("Register request"))
	defer func() { setStatusOfError(rw, ctxLogger, err) }()

	if err = r.ParseForm(); err != nil {
		return
	}

	login := r.PostForm.Get("login")
	pwd := r.PostForm.Get("pwd")
	fn := r.PostForm.Get("first_name")
	ln := r.PostForm.Get("last_name")

	user := new(users.User)
	err = user.Register(login, pwd, fn, ln)
	if err != nil {
		return
	}

	err = setToken(*user, r, rw)
	if err != nil {
		return
	}

	http.Redirect(rw, r, "/chat/chat.html", http.StatusSeeOther)
}

func UserAbout(rw http.ResponseWriter, r *http.Request) {
	var err error

	ctxLogger := logger.Logger.With(logger.EventField("User about request"))
	defer func() { setStatusOfError(rw, ctxLogger, err) }()

	user, err := getUser(r)
	if err != nil {
		return
	}

	err = json.NewEncoder(rw).Encode(&user)
	if err != nil {
		return
	}

	rw.Header().Set("Content-Type", "application/json")
}

func Search(rw http.ResponseWriter, r *http.Request) {
	var err error

	ctxLogger := logger.Logger.With(logger.EventField("Search request"))
	defer func() { setStatusOfError(rw, ctxLogger, err) }()

	user, err := getUser(r)
	if err != nil {
		return
	}

	if err = r.ParseForm(); err != nil {
		return
	}

	chatArr, err := chats.Search(user, r.Form.Get("phrase"))
	if err != nil {
		return
	}

	err = json.NewEncoder(rw).Encode(chatArr)
	if err != nil {
		return
	}

	rw.Header().Set("Content-Type", "application/json")
}

func CreateChat(rw http.ResponseWriter, r *http.Request) {
	var err error

	ctxLogger := logger.Logger.With(logger.EventField("Create chat request"))
	defer func() { setStatusOfError(rw, ctxLogger, err) }()

	user, err := getUser(r)
	if err != nil {
		return
	}

	defer r.Body.Close()

	chat := new(chats.Chat)
	err = chat.Register(r.Body, user)

	if err != nil {
		return
	}

	err = json.NewEncoder(rw).Encode(chat)
	if err != nil {
		return
	}

	rw.Header().Set("Content-Type", "application/json")
}

func ChatList(rw http.ResponseWriter, r *http.Request) {
	var err error

	ctxLogger := logger.Logger.With(logger.EventField("Chat list request"))
	defer func() { setStatusOfError(rw, ctxLogger, err) }()

	user, err := getUser(r)
	if err != nil {
		return
	}

	chatArr, err := chats.List(user)
	if err != nil {
		return
	}

	err = json.NewEncoder(rw).Encode(chatArr)
	if err != nil {
		return
	}

	rw.Header().Set("Content-Type", "application/json")
}

func ChatHistory(rw http.ResponseWriter, r *http.Request) {
	var err error

	ctxLogger := logger.Logger.With(logger.EventField("Chat history request"))
	defer func() { setStatusOfError(rw, ctxLogger, err) }()

	user, err := getUser(r)
	if err != nil {
		return
	}

	if err = r.ParseForm(); err != nil {
		return
	}

	chatID, err := strconv.ParseUint(r.Form.Get("chat_id"), 10, 64)
	if err != nil {
		return
	}

	messageID, err := strconv.ParseUint(r.Form.Get("message_id"), 10, 64)
	if err != nil {
		return
	}

	history := new(chats.History)
	err = history.Fill(user, uint(chatID), uint(messageID))
	if err != nil {
		return
	}

	err = json.NewEncoder(rw).Encode(history)
	if err != nil {
		return
	}

	rw.Header().Set("Content-Type", "application/json")
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

	user, err := getUser(r)
	if err != nil {
		return
	}

	userID = (&user).ID()
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

		errMsg = message.Register(user)
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
