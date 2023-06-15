package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Elena-S/Chat/pkg/auth"
	"github.com/Elena-S/Chat/pkg/logger"
	"github.com/Elena-S/Chat/pkg/users"
)

func Home(rw http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/favicon.ico" {
		return
	}

	var err error
	rh := NewResponseHelper(rw, r)
	ctxLogger := logger.Logger.With(logger.EventField("Home request"))
	defer func() { rh.RedirectToErrorPage(ctxLogger, err) }()

	if r.URL.Path == "/" {
		if err = rh.RevokeToken(r.Context(), ctxLogger); err != nil {
			return
		}
		err = auth.OAuthManager.AuthRequest(r.Context(), rh)
	} else if _, err = rh.GetUserID(); err != nil {
		rh.Redirect("/")
	}
}

func Error(rw http.ResponseWriter, r *http.Request) {
	if !(r.Method == "GET" || r.Method == "POST") {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var err error
	rh := NewResponseHelper(rw, r)
	ctxLogger := logger.Logger.With(logger.EventField("Error request"))
	defer func() {
		if err != nil {
			ctxLogger.Error(err.Error())
		} else if data := recover(); data != nil {
			logger.ErrorPanic(ctxLogger, data)
		}
	}()

	rh.LoadPage("../../view/error/error.html")
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
	rh := NewResponseHelper(rw, r)
	ctxLogger := logger.Logger.With(logger.EventField("Logout GET request"))
	defer func() { rh.RedirectToErrorPage(ctxLogger, err) }()

	err = r.ParseForm()
	if err != nil {
		return
	}

	logoutChallenge := r.Form.Get("logout_challenge")
	if logoutChallenge == "" {
		err = errors.New("handlers: the logout challenge is not set")
		return
	}
	_, refreshToken, err := rh.RetrieveTokens()
	err = auth.OAuthManager.LogoutRequest(r.Context(), logoutChallenge, refreshToken, rh)
}

func Consent(rw http.ResponseWriter, r *http.Request) {
	if !(r.Method == "GET" || r.Method == "POST") {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var err error
	rh := NewResponseHelper(rw, r)
	ctxLogger := logger.Logger.With(logger.EventField("Consent request"))
	defer func() { rh.RedirectToErrorPage(ctxLogger, err) }()

	if err = r.ParseForm(); err != nil {
		return
	}

	consentChallenge := strings.TrimSpace(r.Form.Get("consent_challenge"))
	if consentChallenge == "" {
		err = errors.New("handlers: consent challenge is not set")
		return
	}

	err = auth.OAuthManager.AcceptConsentRequest(r.Context(), consentChallenge, rh)
}

func FinishAuth(rw http.ResponseWriter, r *http.Request) {
	if !(r.Method == "GET" || r.Method == "POST") {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var err error
	rh := NewResponseHelper(rw, r)
	ctxLogger := logger.Logger.With(logger.EventField("Chat request"))
	defer func() { rh.RedirectToErrorPage(ctxLogger, err) }()
	if err = r.ParseForm(); err != nil {
		return
	}

	state := strings.TrimSpace(r.Form.Get("state"))
	if state == "" {
		err = errors.New("handlers: the state is empty")
		return
	}

	code := strings.TrimSpace(r.Form.Get("code"))
	if code == "" {
		err = errors.New("handlers: the authorization code is empty")
		return
	}

	tokens, err := auth.OAuthManager.ExchangeForTokens(r.Context(), state, code)
	if err != nil {
		return
	}

	if err = rh.SetTokens(tokens); err != nil {
		return
	}

	rh.Redirect("/chat")
}

func Chat(rw http.ResponseWriter, r *http.Request) {
	if !(r.Method == "GET" || r.Method == "POST") {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var err error
	rh := NewResponseHelper(rw, r)
	ctxLogger := logger.Logger.With(logger.EventField("Chat request"))
	defer func() { rh.RedirectToErrorPage(ctxLogger, err) }()

	if _, err = rh.GetUserID(); err != nil {
		rh.Redirect("/")
		return
	}
	err = rh.LoadPage("../../view/chat/chat.html")
}

func loginGET(rw http.ResponseWriter, r *http.Request) {
	var err error
	rh := NewResponseHelper(rw, r)
	ctxLogger := logger.Logger.With(logger.EventField("Login GET request"))
	defer func() { rh.RedirectToErrorPage(ctxLogger, err) }()

	if err = r.ParseForm(); err != nil {
		return
	}

	loginChallenge := strings.TrimSpace(r.Form.Get("login_challenge"))
	if loginChallenge == "" {
		err = errors.New("handlers: the login challenge is not set")
		return
	}

	skipped, err := auth.OAuthManager.LoginRequest(r.Context(), loginChallenge, rh)
	if skipped || err != nil {
		return
	}
	err = rh.LoadPage("../../view/index.html")
}

func loginPOST(rw http.ResponseWriter, r *http.Request) {
	var err error
	rh := NewResponseHelper(rw, r)
	ctxLogger := logger.Logger.With(logger.EventField("Login POST request"))
	defer func() { rh.RedirectToErrorPage(ctxLogger, err) }()

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

	err = auth.OAuthManager.AcceptLoginRequest(r.Context(), user.IDToString(), loginChallenge, remember, rh)
}
