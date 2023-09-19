package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Elena-S/Chat/pkg/secretsmng"
	"github.com/Elena-S/Chat/pkg/users"
)

var (
	ErrNoLoginChallenge   = errors.New("handlers: the login challenge is not set")
	ErrNoLogoutChallenge  = errors.New("handlers: the logout challenge is not set")
	ErrNoConsentChallenge = errors.New("handlers: consent challenge is not set")
	ErrNoState            = errors.New("handlers: the state is empty")
	ErrNoAuthCode         = errors.New("handlers: the authorization code is empty")
)

func (routHandler *RoutHandler) Home(rw http.ResponseWriter, r *http.Request) (err error) {
	if r.URL.Path == "/favicon.ico" {
		return
	}

	rh := NewResponseHelper(rw, r)

	active, err := rh.TokenIsActive(routHandler.oAuthManager)
	if err != nil {
		return
	}
	if active && r.URL.Path == "/" {
		rh.Redirect("/chat")
		return
	}

	return routHandler.oAuthManager.AuthRequest(r.Context(), rh)
}

func (routHandler *RoutHandler) Error(rw http.ResponseWriter, r *http.Request) (err error) {
	if !(r.Method == "GET" || r.Method == "POST") {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	return NewResponseHelper(rw, r).LoadPage("../../view/error/error.html")
}

func (authHandler *AuthHandler) Login(rw http.ResponseWriter, r *http.Request) (err error) {
	switch r.Method {
	case "GET":
		err = authHandler.loginGET(rw, r)
	case "POST":
		err = authHandler.loginPOST(rw, r)
	default:
		rw.WriteHeader(http.StatusMethodNotAllowed)
	}
	return
}

func (authHandler *AuthHandler) loginGET(rw http.ResponseWriter, r *http.Request) (err error) {
	if err = r.ParseForm(); err != nil {
		return
	}

	loginChallenge := strings.TrimSpace(r.Form.Get("login_challenge"))
	if loginChallenge == "" {
		err = ErrNoLoginChallenge
		return
	}

	rh := NewResponseHelper(rw, r)
	skipped, err := authHandler.oAuthManager.LoginRequest(r.Context(), loginChallenge, rh)
	if skipped || err != nil {
		return
	}
	return rh.LoadPage("../../view/index.html")
}

func (authHandler *AuthHandler) loginPOST(rw http.ResponseWriter, r *http.Request) (err error) {
	if err = r.ParseForm(); err != nil {
		return
	}

	loginChallenge := strings.TrimSpace(r.Form.Get("login_challenge"))
	if loginChallenge == "" {
		err = ErrNoLoginChallenge
		return
	}
	authFunc := authHandler.usersManager.Authorize
	regData := users.RegData{}
	regData.Phone = r.Form.Get("login")
	regData.Password = secretsmng.Password(r.Form.Get("pwd"))
	if r.Form.Get("registration") == "on" {
		regData.FirstName = r.Form.Get("first_name")
		regData.LastName = r.Form.Get("last_name")
		authFunc = authHandler.usersManager.Register
	}
	userID, err := authFunc(r.Context(), regData)
	if err != nil {
		return
	}

	rh := NewResponseHelper(rw, r)
	return authHandler.oAuthManager.AcceptLoginRequest(r.Context(), userID.String(), loginChallenge, rh)
}

func (routHandler *RoutHandler) SilentLogin(rw http.ResponseWriter, r *http.Request) (err error) {
	rh := NewResponseHelper(rw, r)
	return routHandler.oAuthManager.SilentAuthRequest(r.Context(), rh)
}

func (routHandler *RoutHandler) Logout(rw http.ResponseWriter, r *http.Request) (err error) {
	if !(r.Method == "GET" || r.Method == "POST") {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	rh := NewResponseHelper(rw, r)
	if err = r.ParseForm(); err != nil {
		return
	}

	logoutChallenge := r.Form.Get("logout_challenge")
	if logoutChallenge == "" {
		err = ErrNoLogoutChallenge
		return
	}
	_, refreshToken, err := rh.RetrieveTokens()
	if !(err == nil || errors.Is(err, http.ErrNoCookie)) {
		return
	}
	return routHandler.oAuthManager.LogoutRequest(r.Context(), logoutChallenge, refreshToken, rh)
}

func (routHandler *RoutHandler) Consent(rw http.ResponseWriter, r *http.Request) (err error) {
	if !(r.Method == "GET" || r.Method == "POST") {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	rh := NewResponseHelper(rw, r)
	if err = r.ParseForm(); err != nil {
		return
	}

	consentChallenge := strings.TrimSpace(r.Form.Get("consent_challenge"))
	if consentChallenge == "" {
		err = ErrNoConsentChallenge
		return
	}

	return routHandler.oAuthManager.AcceptConsentRequest(r.Context(), consentChallenge, rh)
}

func (routHandler *RoutHandler) FinishAuth(rw http.ResponseWriter, r *http.Request) (err error) {
	if !(r.Method == "GET" || r.Method == "POST") {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	rh := NewResponseHelper(rw, r)
	if err = routHandler.fulfillAuth(rh); err != nil {
		return
	}

	rh.Redirect("/chat")
	return
}

func (routHandler *RoutHandler) FinishSilentAuth(rw http.ResponseWriter, r *http.Request) (err error) {
	if !(r.Method == "GET" || r.Method == "POST") {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	rh := NewResponseHelper(rw, r)
	if err = routHandler.fulfillAuth(rh); err != nil {
		return
	}

	rh.Redirect("/authentication/finish/silent/ok")
	return
}

func (routHandler *RoutHandler) SilentAuthOK(rw http.ResponseWriter, r *http.Request) (err error) {
	if !(r.Method == "GET" || r.Method == "POST") {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	rh := NewResponseHelper(rw, r)
	return rh.LoadPage("../../view/silent_auth/ok.html")
}

func (routHandler *RoutHandler) Chat(rw http.ResponseWriter, r *http.Request) (err error) {
	if !(r.Method == "GET" || r.Method == "POST") {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	rh := NewResponseHelper(rw, r)
	if _, err = rh.GetUserID(routHandler.oAuthManager); err != nil {
		rh.Redirect("/")
		return
	}
	return rh.LoadPage("../../view/chat/chat.html")
}

func (routHandler *RoutHandler) fulfillAuth(rh *ResponseHelper) (err error) {
	if err = rh.r.ParseForm(); err != nil {
		return
	}
	state := strings.TrimSpace(rh.r.Form.Get("state"))
	if state == "" {
		err = ErrNoState
		return
	}
	code := strings.TrimSpace(rh.r.Form.Get("code"))
	if code == "" {
		err = ErrNoAuthCode
		return
	}
	_, refreshToken, errToken := rh.RetrieveTokens()
	if !(errToken == nil || errors.Is(errToken, http.ErrNoCookie)) {
		err = errToken
		return
	}
	if err = routHandler.oAuthManager.RevokeToken(rh.r.Context(), refreshToken, rh); err != nil {
		return
	}
	tokens, err := routHandler.oAuthManager.ExchangeForTokens(rh.r.Context(), state, code, rh.FullURL())
	if err != nil {
		return
	}
	return rh.SetTokens(tokens)
}
