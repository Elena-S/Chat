package auth

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/Elena-S/Chat/pkg/logger"
	ory "github.com/ory/client-go"
	"go.uber.org/fx"
	"golang.org/x/oauth2"
)

const (
	tokenTypeAccess  = "access_token"
	tokenTypeRefresh = "refresh_token"
)

const keyStateTemplate = "oAuth2:state:%s"

var (
	ErrExpiredToken       = errors.New("auth: the access token is expired")
	ErrInvalidAccessToken = errors.New("auth: invalid access token")
)

type SetGetterEx interface {
	SetEx(ctx context.Context, key string, value any, expiration time.Duration) error
	GetEx(ctx context.Context, key string, expiration time.Duration) (string, error)
}

type Redirector interface {
	Redirect(url string)
}

type TokensReseter interface {
	ResetTokens()
}

type ResetTokensRedirector interface {
	Redirector
	TokensReseter
}

var Module = fx.Module("auth",
	fx.Provide(
		NewManager,
	),
)

type Manager struct {
	config        oauth2.Config
	tempStorage   SetGetterEx
	httpClient    *http.Client
	publicClient  *ory.APIClient
	privateClient *ory.APIClient
	logger        *logger.Logger
}

type Params struct {
	fx.In
	TempStorage SetGetterEx
	Logger      *logger.Logger
	Context     context.Context
}

func NewManager(p Params) (*Manager, error) {
	oAuthManager := new(Manager)

	oAuthManager.tempStorage = p.TempStorage
	oAuthManager.logger = p.Logger

	oAuthManager.httpClient = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: time.Second * 30,
	}

	cfgPrivate := ory.NewConfiguration()
	cfgPrivate.Servers = ory.ServerConfigurations{{URL: "https://hydra:4445"}}
	cfgPrivate.HTTPClient = oAuthManager.httpClient
	oAuthManager.privateClient = ory.NewAPIClient(cfgPrivate)

	cfgPublic := ory.NewConfiguration()
	cfgPublic.Servers = ory.ServerConfigurations{{URL: "https://hydra:4444"}}
	cfgPublic.HTTPClient = oAuthManager.httpClient
	oAuthManager.publicClient = ory.NewAPIClient(cfgPublic)

	oAuth2HydraClient, err := oAuthManager.oAuth2HydraClient(p.Context)
	if err != nil {
		return nil, err
	}
	oAuthManager.config = oauth2.Config{
		ClientID:     oAuth2HydraClient.GetClientId(),
		ClientSecret: oAuth2HydraClient.GetClientSecret(),
		Scopes:       strings.Split(oAuth2HydraClient.GetScope(), " "),
		Endpoint: oauth2.Endpoint{
			AuthURL:   "https://localhost:4444/oauth2/auth",
			TokenURL:  "https://hydra:4444/oauth2/token",
			AuthStyle: oauth2.AuthStyleInParams,
		},
	}

	return oAuthManager, nil
}

func (m *Manager) AuthRequest(ctx context.Context, r Redirector) error {
	return m.authRequest(ctx, r, oauth2.SetAuthURLParam("redirect_uri", "https://localhost:8000/authentication/finish"))
}

func (m *Manager) SilentAuthRequest(ctx context.Context, r Redirector) error {
	params := []oauth2.AuthCodeOption{
		oauth2.SetAuthURLParam("prompt", "none"),
		oauth2.SetAuthURLParam("redirect_uri", "https://localhost:8000/authentication/finish/silent"),
	}
	return m.authRequest(ctx, r, params...)
}

func (m *Manager) ExchangeForTokens(ctx context.Context, state string, code string, uri string) (TokenInfoRetriver, error) {
	if _, err := m.tempStorage.GetEx(ctx, fmt.Sprintf(keyStateTemplate, state), time.Duration(0)); err != nil {
		return nil, err
	}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, m.httpClient)
	tokens, err := m.config.Exchange(ctx, code, oauth2.SetAuthURLParam("redirect_uri", uri))
	if err != nil {
		return nil, err
	}
	return &oAuthTokens{tokens}, nil
}

func (m *Manager) LoginRequest(ctx context.Context, loginChallenge string, r Redirector) (skipped bool, err error) {
	loginRequest, response, err := m.privateClient.OAuth2Api.GetOAuth2LoginRequest(ctx).LoginChallenge(loginChallenge).Execute()
	if err != nil {
		return false, fmt.Errorf("auth: an error occured when calling OAuth2Api.GetOAuth2LoginRequest: %w\nfull HTTP response: %v", err, response)
	}
	if loginRequest.GetSkip() {
		err = m.AcceptLoginRequest(ctx, loginRequest.GetSubject(), loginChallenge, r)
		return err == nil, err
	}
	return
}

func (m *Manager) AcceptLoginRequest(ctx context.Context, sub string, loginChallenge string, r Redirector) error {
	acceptRequest := ory.NewAcceptOAuth2LoginRequest(sub)
	acceptRequest.SetRemember(true)
	acceptRequest.SetRememberFor(15552000) //6 months
	acceptRequest.SetExtendSessionLifespan(true)
	redirectTo, response, err := m.privateClient.OAuth2Api.AcceptOAuth2LoginRequest(ctx).LoginChallenge(loginChallenge).AcceptOAuth2LoginRequest(*acceptRequest).Execute()
	if err != nil {
		return fmt.Errorf("auth: an error occured when calling OAuth2Api.AcceptOAuth2LoginRequest: %w\nfull HTTP response: %v", err, response)
	}
	r.Redirect(redirectTo.GetRedirectTo())
	return nil
}

func (m *Manager) AcceptConsentRequest(ctx context.Context, consentChallenge string, r Redirector) error {
	acceptRequest := ory.NewAcceptOAuth2ConsentRequest()
	acceptRequest.SetGrantScope(m.config.Scopes)
	acceptRequest.SetRemember(true)
	acceptRequest.SetRememberFor(15552000) //6 months
	redirectTo, response, err := m.privateClient.OAuth2Api.AcceptOAuth2ConsentRequest(ctx).ConsentChallenge(consentChallenge).AcceptOAuth2ConsentRequest(*acceptRequest).Execute()
	if err != nil {
		return fmt.Errorf("auth: an error occured when calling OAuth2Api.AcceptOAuth2ConsentRequest: %w\nfull HTTP response: %v", err, response)
	}
	r.Redirect(redirectTo.GetRedirectTo())
	return nil
}

func (m *Manager) LogoutRequest(ctx context.Context, logoutChallenge string, refreshToken string, r ResetTokensRedirector) (err error) {
	_, response, err := m.privateClient.OAuth2Api.GetOAuth2LogoutRequest(ctx).LogoutChallenge(logoutChallenge).Execute()
	if err != nil {
		return fmt.Errorf("auth: an error occured when calling OAuth2Api.GetOAuth2LogoutRequest: %w\nfull HTTP response: %v", err, response)
	}

	redirectTo, response, err := m.privateClient.OAuth2Api.AcceptOAuth2LogoutRequest(ctx).LogoutChallenge(logoutChallenge).Execute()
	if err != nil {
		return fmt.Errorf("auth: an error occured when calling OAuth2Api.AcceptOAuth2LogoutRequest: %w\nfull HTTP response: %v", err, response)
	}

	if err = m.RevokeToken(ctx, refreshToken, r); err != nil {
		return
	}

	r.Redirect(redirectTo.GetRedirectTo())
	return
}

func (m *Manager) RevokeToken(ctx context.Context, token string, r TokensReseter) (err error) {
	if token == "" {
		return
	}
	revokeTokenRequest := m.publicClient.OAuth2Api.RevokeOAuth2Token(ctx).ClientId(m.config.ClientID).ClientSecret(m.config.ClientSecret)
	response, err := revokeTokenRequest.Token(token).Execute()
	if err != nil {
		err = fmt.Errorf("auth: an error occured when calling OAuth2Api.RevokeOAuth2Token: %w\nfull HTTP response: %v", err, response)
	}
	r.ResetTokens()
	return
}

func (m *Manager) RefreshTokens(ctx context.Context, accessToken string, refreshToken string) (TokenInfoRetriver, error) {
	tokenInfo, response, err := m.privateClient.OAuth2Api.IntrospectOAuth2Token(ctx).Token(accessToken).Execute()
	if err != nil {
		return nil, fmt.Errorf("auth: an error occured when calling OAuth2Api.IntrospectOAuth2Token: %w\nfull HTTP response: %v", err, response)
	}

	if !(tokenInfo.GetActive() && tokenInfo.GetTokenUse() == tokenTypeAccess && tokenInfo.GetClientId() == m.config.ClientID) {
		return nil, ErrInvalidAccessToken
	}

	ctx = context.WithValue(ctx, oauth2.HTTPClient, m.httpClient)
	tokens, err := m.config.Exchange(ctx, "", oauth2.SetAuthURLParam("grant_type", "refresh_token"), oauth2.SetAuthURLParam(tokenTypeRefresh, refreshToken))
	if err != nil {
		return nil, err
	}
	return &oAuthTokens{tokens}, nil
}

func (m *Manager) GetSubByToken(ctx context.Context, accessToken string) (string, error) {
	tokenInfo, err := m.introspectAccessToken(ctx, accessToken)
	if err != nil {
		return "", err
	}
	return (*tokenInfo).GetSub(), nil
}

func (m *Manager) AccessTokenIsActive(ctx context.Context, accessToken string) (bool, error) {
	tokenInfo, err := m.introspectAccessToken(ctx, accessToken)
	if err != nil {
		if err == ErrExpiredToken {
			return false, nil
		} else {
			return false, err
		}
	}
	return tokenInfo != nil, nil
}

func (m *Manager) introspectAccessToken(ctx context.Context, accessToken string) (*ory.IntrospectedOAuth2Token, error) {
	tokenInfo, response, err := m.privateClient.OAuth2Api.IntrospectOAuth2Token(ctx).Token(accessToken).Execute()
	if err != nil {
		return nil, fmt.Errorf("auth: an error occured when calling OAuth2Api.IntrospectOAuth2Token: %w\nfull HTTP response: %v", err, response)
	}
	if !tokenInfo.GetActive() {
		return nil, ErrExpiredToken
	}

	if !(tokenInfo.GetTokenUse() == tokenTypeAccess && tokenInfo.GetClientId() == m.config.ClientID) {
		return nil, ErrInvalidAccessToken
	}
	return tokenInfo, err
}

func (m *Manager) authRequest(ctx context.Context, r Redirector, opts ...oauth2.AuthCodeOption) error {
	state, err := generateState()
	if err != nil {
		return err
	}
	if err = m.tempStorage.SetEx(ctx, fmt.Sprintf(keyStateTemplate, state), true, time.Minute*30); err != nil {
		return err
	}
	r.Redirect(m.config.AuthCodeURL(state, opts...))
	return nil
}

func (m *Manager) oAuth2HydraClient(ctx context.Context) (*ory.OAuth2Client, error) {
	const envClientSecret = "HYDRA_OAUTH2_CLIENT_SECRET"
	clientSecret := os.Getenv(envClientSecret)
	if clientSecret == "" {
		return nil, fmt.Errorf("auth: no client secret was provided in %s env var", envClientSecret)
	}
	clientName := "Chat"

	var list []ory.OAuth2Client
	var response *http.Response
	var err error
	for i := 0; i < 3; i++ {
		list, response, err = m.privateClient.OAuth2Api.ListOAuth2Clients(ctx).ClientName(clientName).Execute()
		if errors.Is(err, syscall.ECONNREFUSED) {
			time.Sleep(time.Second * 2)
			continue
		} else {
			break
		}
	}
	if err != nil {
		return nil, fmt.Errorf("auth: an error occured when calling OAuth2Api.ListOAuth2Clients: %w\nfull HTTP response: %v\n", err, response)
	}

	if len(list) > 0 {
		oAuth2Client := &list[0]
		oAuth2Client.SetClientSecret(clientSecret)
		return oAuth2Client, nil
	}

	oAuth2Client := ory.NewOAuth2Client()
	oAuth2Client.SetClientName(clientName)
	oAuth2Client.SetClientSecret(clientSecret)
	oAuth2Client.SetClientUri("https://localhost:8000")
	oAuth2Client.SetAudience([]string{"https://localhost:8000"})
	oAuth2Client.SetSkipConsent(true)
	//TODO: add subject obfuscation
	oAuth2Client.SetSubjectType("public")
	oAuth2Client.SetScope("openid offline_access")
	oAuth2Client.SetTokenEndpointAuthMethod("client_secret_post")
	oAuth2Client.SetTokenEndpointAuthSigningAlg("S256")
	oAuth2Client.SetGrantTypes([]string{"authorization_code", "refresh_token"})
	oAuth2Client.SetRedirectUris([]string{"https://localhost:8000/authentication/finish", "https://localhost:8000/authentication/finish/silent"})
	oAuth2Client.SetPostLogoutRedirectUris([]string{"https://localhost:8000"})

	oAuth2Client, response, err = m.privateClient.OAuth2Api.CreateOAuth2Client(ctx).OAuth2Client(*oAuth2Client).Execute()
	if err != nil {
		return nil, fmt.Errorf("auth: an error occured when calling OAuth2Api.CreateOAuth2Client: %w\nfull HTTP response: %v\n", err, response)
	}

	return oAuth2Client, nil
}

func generateState() (string, error) {
	b := make([]byte, 43)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

type TokenInfoRetriver interface {
	AccessToken() string
	RefreshToken() string
	Expiry() time.Time
}

var _ TokenInfoRetriver = (*oAuthTokens)(nil)

type oAuthTokens struct {
	tokens *oauth2.Token
}

func (t *oAuthTokens) AccessToken() string {
	return t.tokens.AccessToken
}

func (t *oAuthTokens) RefreshToken() string {
	return t.tokens.RefreshToken
}

func (t *oAuthTokens) Expiry() time.Time {
	return t.tokens.Expiry
}
