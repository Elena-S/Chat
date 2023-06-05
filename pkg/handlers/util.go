package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"os"

	"github.com/Elena-S/Chat/pkg/logger"
	"github.com/Elena-S/Chat/pkg/users"
	"go.uber.org/zap"
)

func setStatusOfError(rw http.ResponseWriter, ctxLogger *zap.Logger, err error) {
	if data := recover(); data != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		logger.ErrorPanic(ctxLogger, data)
	} else if err != nil {
		switch err {
		case users.ErrInvalidCredentials, users.ErrInvalidLoginFormat:
			rw.WriteHeader(http.StatusBadRequest)
			io.WriteString(rw, err.Error())
		case users.ErrUsrExists, users.ErrWrongCredentials:
			rw.WriteHeader(http.StatusForbidden)
			io.WriteString(rw, err.Error())
		default:
			rw.WriteHeader(http.StatusInternalServerError)
		}
		ctxLogger.Error(err.Error())
	}
}

func redirectToErrorPage(rw http.ResponseWriter, r *http.Request, ctxLogger *zap.Logger, err error) {
	if err != nil {
		ctxLogger.Error(err.Error())
		http.Redirect(rw, r, os.Getenv("URL_ERROR"), http.StatusSeeOther)
	} else if data := recover(); data != nil {
		logger.ErrorPanic(ctxLogger, data)
		http.Redirect(rw, r, os.Getenv("URL_ERROR"), http.StatusSeeOther)
	}
}

func writeJSONContent(rw http.ResponseWriter, object any) (err error) {
	err = json.NewEncoder(rw).Encode(object)
	if err != nil {
		return
	}
	rw.Header().Set("Content-Type", "application/json")
	return
}
