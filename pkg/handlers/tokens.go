package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/Elena-S/Chat/pkg/hydra"
	"golang.org/x/oauth2"
)

const (
	cookieNameAccessToken  = "__Secure-access_token"
	cookieNameRefreshToken = "__Secure-refresh_token"
)

func getUserID(r *http.Request) (uint, error) {
	accessToken, refreshToken, err := retrieveTokens(r)
	if err != nil {
		return 0, err
	}
	return getUserByAccesToken(accessToken, refreshToken, r.Context())
}

func retrieveTokens(r *http.Request) (string, string, error) {
	cookie, err := r.Cookie(cookieNameRefreshToken)
	if err != nil {
		return "", "", errors.New("handlers: missing a refresh token")
	}
	refreshToken := cookie.Value

	cookie, err = r.Cookie(cookieNameAccessToken)
	if err != nil {
		return "", "", errors.New("handlers: missing an access token")
	}

	return cookie.Value, refreshToken, err
}

func setTokens(rw http.ResponseWriter, tokens *oauth2.Token) error {
	if tokens.RefreshToken == "" {
		return errors.New("handlers: missing a refresh token")
	}
	rw.Header().Add("Set-Cookie", tokenCookieString(cookieNameAccessToken, tokens.AccessToken, tokens.Expiry))
	rw.Header().Add("Set-Cookie", tokenCookieString(cookieNameRefreshToken, tokens.RefreshToken, tokens.Expiry.Add(time.Hour*4320)))
	return nil
}

func resetTokens(rw http.ResponseWriter) {
	expiry := time.Now()
	rw.Header().Add("Set-Cookie", tokenCookieString(cookieNameAccessToken, "", expiry))
	rw.Header().Add("Set-Cookie", tokenCookieString(cookieNameRefreshToken, "", expiry))
}

func tokenCookieString(name string, value string, expiry time.Time) string {
	return fmt.Sprintf("%s=%s; secure; httpOnly; sameSite=strict; expires=%s; path=/", name, value, expiry.Format(time.RFC1123))
}

func getUserByAccesToken(accessToken string, refreshToken string, ctx context.Context) (id uint, err error) {
	tokenInfo, response, err := hydra.PrivateClient().OAuth2Api.IntrospectOAuth2Token(ctx).Token(accessToken).Execute()
	if err != nil {
		err = fmt.Errorf("handlers: an error occured when calling OAuth2Api.IntrospectOAuth2Token: %w\nfull HTTP response: %v", err, response)
		return
	}

	if !tokenInfo.GetActive() {
		err = errors.New("handlers: the access token is expired")
		return
	}

	if !(tokenInfo.GetTokenUse() == hydra.TokenTypeAccess && tokenInfo.GetClientId() == hydra.OAuthConf.Config.ClientID) {
		err = errors.New("handlers: invalid access token")
		return
	}

	value, err := strconv.ParseUint((*tokenInfo).GetSub(), 10, 64)
	return uint(value), err
}
