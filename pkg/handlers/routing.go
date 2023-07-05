package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Elena-S/Chat/pkg/auth"
	"github.com/Elena-S/Chat/pkg/logger"
	"github.com/Elena-S/Chat/pkg/users"
)

func Home(rw http.ResponseWriter, r *http.Request, ctxLogger logger.Logger) (err error) {
	if r.URL.Path == "/favicon.ico" {
		return
	}

	rh := NewResponseHelper(rw, r)

	active, err := rh.TokenIsActive(r.Context())
	if err != nil {
		return
	}
	if active && r.URL.Path == "/" {
		rh.Redirect("/chat")
		return
	}

	return auth.OAuthManager.AuthRequest(r.Context(), rh)
}

func Error(rw http.ResponseWriter, r *http.Request, ctxLogger logger.Logger) (err error) {
	if !(r.Method == "GET" || r.Method == "POST") {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	return NewResponseHelper(rw, r).LoadPage("../../view/error/error.html")
}

func Login(rw http.ResponseWriter, r *http.Request, ctxLogger logger.Logger) (err error) {
	switch r.Method {
	case "GET":
		err = loginGET(rw, r, ctxLogger)
	case "POST":
		err = loginPOST(rw, r, ctxLogger)
	default:
		rw.WriteHeader(http.StatusMethodNotAllowed)
	}
	return
}

func SilentLogin(rw http.ResponseWriter, r *http.Request, ctxLogger logger.Logger) (err error) {
	rh := NewResponseHelper(rw, r)
	return auth.OAuthManager.SilentAuthRequest(r.Context(), rh)
}

func Logout(rw http.ResponseWriter, r *http.Request, ctxLogger logger.Logger) (err error) {
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
		err = errors.New("handlers: the logout challenge is not set")
		return
	}
	_, refreshToken, err := rh.RetrieveTokens()
	if !(err == nil || errors.Is(err, http.ErrNoCookie)) {
		return
	}
	return auth.OAuthManager.LogoutRequest(r.Context(), logoutChallenge, refreshToken, rh)
}

func Consent(rw http.ResponseWriter, r *http.Request, ctxLogger logger.Logger) (err error) {
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
		err = errors.New("handlers: consent challenge is not set")
		return
	}

	return auth.OAuthManager.AcceptConsentRequest(r.Context(), consentChallenge, rh)
}

func FinishAuth(rw http.ResponseWriter, r *http.Request, ctxLogger logger.Logger) (err error) {
	if !(r.Method == "GET" || r.Method == "POST") {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	rh := NewResponseHelper(rw, r)
	if err = fulfillAuth(rh); err != nil {
		return
	}

	rh.Redirect("/chat")
	return
}

func FinishSilentAuth(rw http.ResponseWriter, r *http.Request, ctxLogger logger.Logger) (err error) {
	if !(r.Method == "GET" || r.Method == "POST") {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	rh := NewResponseHelper(rw, r)
	if err = fulfillAuth(rh); err != nil {
		return
	}

	rh.Redirect("/authentication/finish/silent/ok")
	return
}

func SilentAuthOK(rw http.ResponseWriter, r *http.Request, ctxLogger logger.Logger) (err error) {
	if !(r.Method == "GET" || r.Method == "POST") {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	rh := NewResponseHelper(rw, r)
	return rh.LoadPage("../../view/silent_auth/ok.html")
}

func Chat(rw http.ResponseWriter, r *http.Request, ctxLogger logger.Logger) (err error) {
	if !(r.Method == "GET" || r.Method == "POST") {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	rh := NewResponseHelper(rw, r)
	if _, err = rh.GetUserID(); err != nil {
		rh.Redirect("/")
		return
	}
	return rh.LoadPage("../../view/chat/chat.html")
}

func loginGET(rw http.ResponseWriter, r *http.Request, ctxLogger logger.Logger) (err error) {
	if err = r.ParseForm(); err != nil {
		return
	}

	loginChallenge := strings.TrimSpace(r.Form.Get("login_challenge"))
	if loginChallenge == "" {
		err = errors.New("handlers: the login challenge is not set")
		return
	}

	rh := NewResponseHelper(rw, r)
	skipped, err := auth.OAuthManager.LoginRequest(r.Context(), loginChallenge, rh)
	if skipped || err != nil {
		return
	}
	return rh.LoadPage("../../view/index.html")
}

func loginPOST(rw http.ResponseWriter, r *http.Request, ctxLogger logger.Logger) (err error) {
	if err = r.ParseForm(); err != nil {
		return
	}

	loginChallenge := strings.TrimSpace(r.Form.Get("login_challenge"))
	if loginChallenge == "" {
		err = errors.New("handlers: the login challenge is not set")
		return
	}
	remember := r.Form.Get("remember_me") == "on"
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

	rh := NewResponseHelper(rw, r)
	return auth.OAuthManager.AcceptLoginRequest(r.Context(), user.IDToString(), loginChallenge, remember, rh)
}

func fulfillAuth(rh *responseHelper) (err error) {
	if err = rh.r.ParseForm(); err != nil {
		return
	}
	state := strings.TrimSpace(rh.r.Form.Get("state"))
	if state == "" {
		err = errors.New("handlers: the state is empty")
		return
	}
	code := strings.TrimSpace(rh.r.Form.Get("code"))
	if code == "" {
		err = errors.New("handlers: the authorization code is empty")
		return
	}
	_, refreshToken, errToken := rh.RetrieveTokens()
	if !(errToken == nil || errors.Is(errToken, http.ErrNoCookie)) {
		err = errToken
		return
	}
	if err = auth.OAuthManager.RevokeToken(rh.r.Context(), refreshToken, rh); err != nil {
		return
	}
	tokens, err := auth.OAuthManager.ExchangeForTokens(rh.r.Context(), state, code, rh.FullURL())
	if err != nil {
		return
	}
	return rh.SetTokens(tokens)
}
