package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Elena-S/Chat/pkg/hydra"
	"github.com/Elena-S/Chat/pkg/logger"
	"github.com/Elena-S/Chat/pkg/users"
	ory "github.com/ory/client-go"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
)

func Home(rw http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/favicon.ico" {
		return
	}

	var err error

	ctxLogger := logger.Logger.With(logger.EventField("Home request"))
	defer func() { redirectToErrorPage(rw, r, ctxLogger, err) }()

	if r.URL.Path == "/" {
		err = revokeToken(rw, r, ctxLogger)
		if err != nil {
			return
		}
		var oAuthUrl string
		oAuthUrl, err = hydra.OAuthConf.OAuthURL(r.Context())
		if err != nil {
			return
		}
		http.Redirect(rw, r, oAuthUrl, http.StatusSeeOther)
	} else if _, err := getUserID(r); err != nil {
		http.Redirect(rw, r, "/", http.StatusSeeOther)
	}
}

func Error(rw http.ResponseWriter, r *http.Request) {
	if !(r.Method == "GET" || r.Method == "POST") {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var err error

	ctxLogger := logger.Logger.With(logger.EventField("Error request"))
	defer func() {
		if err != nil {
			ctxLogger.Error(err.Error())
		} else if data := recover(); data != nil {
			logger.ErrorPanic(ctxLogger, data)
		}
	}()

	b, err := os.ReadFile("../../view/error/error.html")
	if err != nil {
		return
	}
	rw.Header().Set("Content-Type", "text/html")
	_, err = rw.Write(b)
}

func Login(rw http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		loginGET(rw, r)
	case "POST":
		loginPOST(rw, r)
	default:
		rw.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func Logout(rw http.ResponseWriter, r *http.Request) {
	if !(r.Method == "GET" || r.Method == "POST") {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var err error

	ctxLogger := logger.Logger.With(logger.EventField("Logout GET request"))
	defer func() { redirectToErrorPage(rw, r, ctxLogger, err) }()

	err = r.ParseForm()
	if err != nil {
		return
	}

	logoutChallenge := r.Form.Get("logout_challenge")
	if logoutChallenge == "" {
		err = errors.New("handlers: the logout challenge is not set")
		return
	}

	_, response, err := hydra.PrivateClient().OAuth2Api.GetOAuth2LogoutRequest(r.Context()).LogoutChallenge(logoutChallenge).Execute()
	if err != nil {
		err = fmt.Errorf("handlers: an error occured when calling OAuth2Api.GetOAuth2LogoutRequest: %w\nfull HTTP response: %v", err, response)
		return
	}

	redirectTo, response, err := hydra.PrivateClient().OAuth2Api.AcceptOAuth2LogoutRequest(r.Context()).LogoutChallenge(logoutChallenge).Execute()
	if err != nil {
		err = fmt.Errorf("handlers: an error occured when calling OAuth2Api.AcceptOAuth2LogoutRequest: %w\nfull HTTP response: %v", err, response)
		return
	}

	err = revokeToken(rw, r, ctxLogger)
	if err != nil {
		return
	}

	http.Redirect(rw, r, redirectTo.GetRedirectTo(), http.StatusSeeOther)
}

func Consent(rw http.ResponseWriter, r *http.Request) {
	if !(r.Method == "GET" || r.Method == "POST") {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var err error

	ctxLogger := logger.Logger.With(logger.EventField("Consent request"))
	defer func() { redirectToErrorPage(rw, r, ctxLogger, err) }()
	if err = r.ParseForm(); err != nil {
		return
	}

	consentChallenge := strings.TrimSpace(r.Form.Get("consent_challenge"))
	if consentChallenge == "" {
		err = errors.New("handlers: consent challenge is not set")
		return
	}

	acceptRequest := ory.NewAcceptOAuth2ConsentRequest()
	acceptRequest.SetGrantScope(hydra.OAuthConf.Config.Scopes)
	acceptRequest.SetRemember(true)
	acceptRequest.SetRememberFor(15552000)
	redirectTo, response, err := hydra.PrivateClient().OAuth2Api.AcceptOAuth2ConsentRequest(r.Context()).ConsentChallenge(consentChallenge).AcceptOAuth2ConsentRequest(*acceptRequest).Execute()
	if err != nil {
		err = fmt.Errorf("handlers: an error occured when calling OAuth2Api.AcceptOAuth2ConsentRequest: %w\nfull HTTP response: %v", err, response)
		return
	}

	http.Redirect(rw, r, redirectTo.GetRedirectTo(), http.StatusSeeOther)
}

func FinishAuth(rw http.ResponseWriter, r *http.Request) {
	if !(r.Method == "GET" || r.Method == "POST") {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var err error

	ctxLogger := logger.Logger.With(logger.EventField("Chat request"))
	defer func() { redirectToErrorPage(rw, r, ctxLogger, err) }()
	if err = r.ParseForm(); err != nil {
		return
	}

	state := strings.TrimSpace(r.Form.Get("state"))
	if state == "" {
		err = errors.New("handlers: the state is empty")
		return
	}

	_, err = hydra.StatesStorage.GetEx(r.Context(), fmt.Sprintf(hydra.KeyStateTemplate, state), time.Duration(0))
	if err != nil {
		return
	}

	code := strings.TrimSpace(r.Form.Get("code"))
	if code == "" {
		err = errors.New("handlers: the authorization code is empty")
		return
	}

	ctx := context.WithValue(r.Context(), oauth2.HTTPClient, hydra.OAuthConf.HTTPClient)
	tokens, err := hydra.OAuthConf.Config.Exchange(ctx, code)
	if err != nil {
		return
	}

	setTokens(rw, tokens)

	http.Redirect(rw, r, "/chat", http.StatusSeeOther)
}

func Chat(rw http.ResponseWriter, r *http.Request) {
	if !(r.Method == "GET" || r.Method == "POST") {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var err error

	ctxLogger := logger.Logger.With(logger.EventField("Chat request"))
	defer func() { redirectToErrorPage(rw, r, ctxLogger, err) }()

	if _, err := getUserID(r); err != nil {
		http.Redirect(rw, r, "/", http.StatusSeeOther)
		return
	}

	b, err := os.ReadFile("../../view/chat/chat.html")
	if err != nil {
		return
	}
	rw.Header().Set("Content-Type", "text/html")
	_, err = rw.Write(b)
}

func loginGET(rw http.ResponseWriter, r *http.Request) {
	var err error

	ctxLogger := logger.Logger.With(logger.EventField("Login GET request"))
	defer func() { redirectToErrorPage(rw, r, ctxLogger, err) }()

	err = r.ParseForm()
	if err != nil {
		return
	}

	loginChallenge := strings.TrimSpace(r.Form.Get("login_challenge"))
	if loginChallenge == "" {
		err = errors.New("handlers: the login challenge is not set")
		return
	}

	loginRequest, response, err := hydra.PrivateClient().OAuth2Api.GetOAuth2LoginRequest(r.Context()).LoginChallenge(loginChallenge).Execute()
	if err != nil {
		err = fmt.Errorf("handlers: an error occured when calling OAuth2Api.GetOAuth2LoginRequest: %w\nfull HTTP response: %v", err, response)
		return
	}

	if loginRequest.GetSkip() {
		err = acceptLoginRequest(rw, r, loginChallenge, loginRequest.GetSubject(), true)
		return
	}

	b, err := os.ReadFile("../../view/index.html")
	if err != nil {
		return
	}
	rw.Header().Set("Content-Type", "text/html")
	_, err = rw.Write(b)
}

func loginPOST(rw http.ResponseWriter, r *http.Request) {
	var err error

	ctxLogger := logger.Logger.With(logger.EventField("Login POST request"))
	defer func() { redirectToErrorPage(rw, r, ctxLogger, err) }()

	if err = r.ParseForm(); err != nil {
		return
	}

	login := r.Form.Get("login")
	pwd := r.Form.Get("pwd")

	user := new(users.User)
	if r.Form.Get("registration") == "on" {
		fn := r.Form.Get("first_name")
		ln := r.Form.Get("last_name")
		err = user.Register(r.Context(), login, pwd, fn, ln)
	} else {
		err = user.Authorize(r.Context(), login, pwd)
	}

	if err != nil {
		return
	}

	loginChallenge := strings.TrimSpace(r.Form.Get("login_challenge"))
	remember := r.Form.Get("remember_me") == "on"

	if loginChallenge == "" {
		err = errors.New("handlers: the login challenge is not set")
		return
	}

	err = acceptLoginRequest(rw, r, loginChallenge, user.IDToString(), remember)
}

func acceptLoginRequest(rw http.ResponseWriter, r *http.Request, loginChallenge string, sub string, remember bool) (err error) {
	acceptRequest := ory.NewAcceptOAuth2LoginRequest(sub)
	acceptRequest.SetRemember(remember)
	acceptRequest.SetRememberFor(15552000) //6 months
	acceptRequest.SetExtendSessionLifespan(remember)
	redirectTo, response, err := hydra.PrivateClient().OAuth2Api.AcceptOAuth2LoginRequest(r.Context()).LoginChallenge(loginChallenge).AcceptOAuth2LoginRequest(*acceptRequest).Execute()
	if err != nil {
		err = fmt.Errorf("handlers: an error occured when calling OAuth2Api.AcceptOAuth2LoginRequest: %w\nfull HTTP response: %v", err, response)
		return
	}
	http.Redirect(rw, r, redirectTo.GetRedirectTo(), http.StatusSeeOther)
	return
}

func revokeToken(rw http.ResponseWriter, r *http.Request, ctxLogger *zap.Logger) (err error) {
	_, refreshToken, errTokens := retrieveTokens(r)
	if errTokens != nil {
		ctxLogger.Error(errTokens.Error())
	}
	if refreshToken == "" {
		return
	}

	oAuthClient := hydra.OAuthConf.Config
	revokeTokenRequest := hydra.PublicClient().OAuth2Api.RevokeOAuth2Token(r.Context()).ClientId(oAuthClient.ClientID).ClientSecret(oAuthClient.ClientSecret)
	response, err := revokeTokenRequest.Token(refreshToken).Execute()
	if err != nil {
		err = fmt.Errorf("handlers: an error occured when calling OAuth2Api.RevokeOAuth2Token: %w\nfull HTTP response: %v", err, response)
		return
	}

	resetTokens(rw)
	return
}
