package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/Elena-S/Chat/pkg/auth"
	"github.com/Elena-S/Chat/pkg/logger"
	"github.com/Elena-S/Chat/pkg/users"
	"go.uber.org/zap"
)

const (
	cookieNameAccessToken  = "__Secure-access_token"
	cookieNameRefreshToken = "__Secure-refresh_token"
)

func NewRequestHelper(r *http.Request) *requestHelper {
	return &requestHelper{r}
}

type requestHelper struct {
	r *http.Request
}

func (rh *requestHelper) GetUserID() (uint, error) {
	accessToken, _, err := rh.RetrieveTokens()
	if err != nil {
		return 0, err
	}
	return auth.OAuthManager.GetUserIDByToken(rh.r.Context(), accessToken)
}

func (rh *requestHelper) RetrieveTokens() (string, string, error) {
	cookie, err := rh.r.Cookie(cookieNameRefreshToken)
	if err != nil {
		return "", "", errors.New("handlers: missing a refresh token")
	}
	refreshToken := cookie.Value

	cookie, err = rh.r.Cookie(cookieNameAccessToken)
	if err != nil {
		return "", "", errors.New("handlers: missing an access token")
	}

	return cookie.Value, refreshToken, err
}

func NewResponseHelper(rw http.ResponseWriter, r *http.Request) *responseHelper {
	return &responseHelper{rw, requestHelper{r}}
}

type responseHelper struct {
	rw http.ResponseWriter
	requestHelper
}

func (rh *responseHelper) Redirect(url string) {
	http.Redirect(rh.rw, rh.requestHelper.r, url, http.StatusSeeOther)
}

func (rh *responseHelper) LoadPage(file string) (err error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return
	}
	rh.rw.Header().Set("Content-Type", "text/html")
	_, err = rh.rw.Write(b)
	return
}

func (rh *responseHelper) WriteJSONContent(object any) (err error) {
	err = json.NewEncoder(rh.rw).Encode(object)
	if err != nil {
		return
	}
	rh.rw.Header().Set("Content-Type", "application/json")
	return
}

func (rh *responseHelper) SetErrorStatus(ctxLogger *zap.Logger, err error) {
	if data := recover(); data != nil {
		rh.rw.WriteHeader(http.StatusInternalServerError)
		logger.ErrorPanic(ctxLogger, data)
	} else if err != nil {
		switch err {
		case users.ErrInvalidCredentials, users.ErrInvalidLoginFormat:
			rh.rw.WriteHeader(http.StatusBadRequest)
			io.WriteString(rh.rw, err.Error())
		case users.ErrUsrExists, users.ErrWrongCredentials:
			rh.rw.WriteHeader(http.StatusForbidden)
			io.WriteString(rh.rw, err.Error())
		default:
			rh.rw.WriteHeader(http.StatusInternalServerError)
		}
		ctxLogger.Error(err.Error())
	}
}

func (rh *responseHelper) RedirectToErrorPage(ctxLogger *zap.Logger, err error) {
	if err != nil {
		ctxLogger.Error(err.Error())
		rh.Redirect(os.Getenv("URL_ERROR"))
	} else if data := recover(); data != nil {
		logger.ErrorPanic(ctxLogger, data)
		rh.Redirect(os.Getenv("URL_ERROR"))
	}
}

func (rh *responseHelper) ResetTokens() {
	expiry := time.Now()
	rh.rw.Header().Add("Set-Cookie", tokenCookieString(cookieNameAccessToken, "", expiry))
	rh.rw.Header().Add("Set-Cookie", tokenCookieString(cookieNameRefreshToken, "", expiry))
}

func (rh *responseHelper) SetTokens(tokens auth.TokenInfoRetriver) error {
	if tokens.RefreshToken() == "" {
		return errors.New("handlers: missing a refresh token")
	}
	expiry := tokens.Expiry()
	rh.rw.Header().Add("Set-Cookie", tokenCookieString(cookieNameAccessToken, tokens.AccessToken(), expiry))
	rh.rw.Header().Add("Set-Cookie", tokenCookieString(cookieNameRefreshToken, tokens.RefreshToken(), expiry.Add(time.Hour*4320)))
	return nil
}

func (rh *responseHelper) RevokeToken(ctx context.Context, ctxLogger *zap.Logger) (err error) {
	_, refreshToken, errTokens := rh.RetrieveTokens()
	if errTokens != nil {
		ctxLogger.Error(errTokens.Error())
	}
	return auth.OAuthManager.RevokeToken(ctx, refreshToken, rh)
}

func tokenCookieString(name string, value string, expiry time.Time) string {
	return fmt.Sprintf("%s=%s; secure; httpOnly; sameSite=strict; expires=%s; path=/", name, value, expiry.Format(time.RFC1123))
}
