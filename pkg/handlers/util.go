package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/Elena-S/Chat/pkg/auth"
	"github.com/Elena-S/Chat/pkg/users"
)

const (
	cookieNameAccessToken  = "__Secure-access_token"
	cookieNameRefreshToken = "__Secure-refresh_token"
)

var (
	ErrNoRefreshToken = errors.New("handlers: missing a refresh token")
)

func NewRequestHelper(r *http.Request) *RequestHelper {
	return &RequestHelper{r}
}

type RequestHelper struct {
	r *http.Request
}

func (rh *RequestHelper) FullURL() string {
	protocol := "https"
	if rh.r.TLS == nil {
		protocol = "http"
	}
	return fmt.Sprintf(`%s://%s%s`, protocol, rh.r.Host, rh.r.URL.Path)
}

func (rh *RequestHelper) RetrieveTokens() (string, string, error) {
	cookie, err := rh.r.Cookie(cookieNameRefreshToken)
	if err != nil {
		return "", "", fmt.Errorf("handlers: missing a refresh token, %w", err)
	}
	refreshToken := cookie.Value

	cookie, err = rh.r.Cookie(cookieNameAccessToken)
	if err != nil {
		return "", "", fmt.Errorf("handlers: missing an access token, %w", err)
	}

	return cookie.Value, refreshToken, err
}

func (rh *RequestHelper) GetUserID(oAuthManager *auth.Manager) (users.UserID, error) {
	accessToken, _, err := rh.RetrieveTokens()
	if err != nil {
		return 0, err
	}
	sub, err := oAuthManager.GetSubByToken(rh.r.Context(), accessToken)
	if err != nil {
		return 0, err
	}
	return users.StringToID(sub)
}

func (rh *RequestHelper) TokenIsActive(oAuthManager *auth.Manager) (bool, error) {
	accessToken, _, err := rh.RetrieveTokens()
	if err != nil {
		if errors.Is(err, http.ErrNoCookie) {
			return false, nil
		}
		return false, err
	}
	return oAuthManager.AccessTokenIsActive(rh.r.Context(), accessToken)
}

func NewResponseHelper(rw http.ResponseWriter, r *http.Request) *ResponseHelper {
	return &ResponseHelper{rw, RequestHelper{r}}
}

type ResponseHelper struct {
	rw http.ResponseWriter
	RequestHelper
}

func (rh *ResponseHelper) Redirect(url string) {
	http.Redirect(rh.rw, rh.r, url, http.StatusSeeOther)
}

func (rh *ResponseHelper) LoadPage(file string) (err error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return
	}
	rh.rw.Header().Set("Content-Type", "text/html")
	_, err = rh.rw.Write(b)
	return
}

func (rh *ResponseHelper) WriteJSONContent(object any) (err error) {
	err = json.NewEncoder(rh.rw).Encode(object)
	if err != nil {
		return
	}
	rh.rw.Header().Set("Content-Type", "application/json")
	return
}

func (rh *ResponseHelper) ResetTokens() {
	expiry := time.Now()
	rh.rw.Header().Add("Set-Cookie", tokenCookieString(cookieNameAccessToken, "", expiry))
	rh.rw.Header().Add("Set-Cookie", tokenCookieString(cookieNameRefreshToken, "", expiry))
}

func (rh *ResponseHelper) SetTokens(tokens auth.TokenInfoRetriver) error {
	if tokens.RefreshToken() == "" {
		return ErrNoRefreshToken
	}

	expiry := tokens.Expiry()
	rh.rw.Header().Add("Set-Cookie", tokenCookieString(cookieNameAccessToken, tokens.AccessToken(), expiry))
	rh.rw.Header().Add("Set-Cookie", tokenCookieString(cookieNameRefreshToken, tokens.RefreshToken(), expiry.Add(time.Hour*4320)))
	return nil
}

func tokenCookieString(name string, value string, expiry time.Time) string {
	return fmt.Sprintf("%s=%s; secure; httpOnly; sameSite=strict; expires=%s; path=/", name, value, expiry.Format(time.RFC1123))
}
