package handlers

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/Elena-S/Chat/pkg/logger"
	"github.com/Elena-S/Chat/pkg/users"
	"go.uber.org/zap"
)

func setStatusOfError(rw http.ResponseWriter, ctxLogger *zap.Logger, err error) {
	if err != nil {
		switch err {
		case users.ErrInvalidCredentials:
			rw.WriteHeader(http.StatusBadRequest)
			io.WriteString(rw, err.Error())
		case users.ErrUsrExists, users.ErrWrongCredentials:
			rw.WriteHeader(http.StatusForbidden)
			io.WriteString(rw, err.Error())
		default:
			rw.WriteHeader(http.StatusInternalServerError)
		}
		ctxLogger.Error(err.Error())
	} else if data := recover(); data != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		logger.ErrorPanic(ctxLogger, data)
	}
}

func setToken(user users.User, r *http.Request, rw http.ResponseWriter) error {
	token, err := users.NewToken(user)
	if err != nil {
		return err
	}
	rw.Header().Add("Set-Cookie", fmt.Sprintf("token=%s; secure; httpOnly; sameSite=strict", token))
	return err
}

func getUser(r *http.Request) (users.User, error) {
	cookie, err := r.Cookie("token")
	if err != nil {
		return users.User{}, errors.New("unauthorized user")
	}

	return users.GetUserByToken(cookie.Value)
}

// func deleteToken(r *http.Request) error {
// 	cookie, err := r.Cookie("token")
// 	if err != nil {
// 		return errors.New("unauthorized user")
// 	}
// 	users.DeleteToken(cookie.Value)

// 	return nil
// }
