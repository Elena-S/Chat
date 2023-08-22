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
	"sync"
	"syscall"
	"time"

	"github.com/Elena-S/Chat/pkg/logger"
	"github.com/Elena-S/Chat/pkg/redis"
	ory "github.com/ory/client-go"
	"golang.org/x/oauth2"
)

const (
	tokenTypeAccess  = "access_token"
	tokenTypeRefresh = "refresh_token"
)

const keyStateTemplate = "oAuth2:state:%s"

var ErrExpiredToken = errors.New("auth: the access token is expired")

var OAuthManager *oAuthManager
var (
	oAuth2Client     *ory.OAuth2Client
	oAuth2ClientOnce sync.Once
	privateOnce      sync.Once
	privateClient    *ory.APIClient
	publicOnce       sync.Once
	publicClient     *ory.APIClient
)
var httpClient *http.Client

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

func init() {
	httpClient = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: time.Second * 30}

	OAuthManager = &oAuthManager{
		config: oauth2.Config{
			ClientID:     oAuth2HydraClient().GetClientId(),
			ClientSecret: oAuth2HydraClient().GetClientSecret(),
			Scopes:       strings.Split(oAuth2HydraClient().GetScope(), " "),
			Endpoint: oauth2.Endpoint{
				AuthURL:   "https://localhost:4444/oauth2/auth",
				TokenURL:  "https://hydra:4444/oauth2/token",
				AuthStyle: oauth2.AuthStyleInParams,
			},
		},
		statesStorage: redis.Client,
	}
}

func private() *ory.APIClient {
	privateOnce.Do(func() {
		cfg := ory.NewConfiguration()
		cfg.Servers = ory.ServerConfigurations{{URL: "https://hydra:4445"}}
		cfg.HTTPClient = httpClient
		privateClient = ory.NewAPIClient(cfg)
	})

	return privateClient
}

func public() *ory.APIClient {
	publicOnce.Do(func() {
		cfg := ory.NewConfiguration()
		cfg.Servers = ory.ServerConfigurations{{URL: "https://hydra:4444"}}
		cfg.HTTPClient = httpClient
		publicClient = ory.NewAPIClient(cfg)
	})

	return publicClient
}

func oAuth2HydraClient() *ory.OAuth2Client {
	oAuth2ClientOnce.Do(func() {
		ctxLogger := logger.ChatLogger.WithEventField("OAuth2 client creation")

		const envClientSecret = "HYDRA_OAUTH2_CLIENT_SECRET"
		clientSecret := os.Getenv(envClientSecret)
		if clientSecret == "" {
			err := fmt.Errorf("auth: no client secret was provided in %s env var", envClientSecret)
			ctxLogger.Fatal(err.Error())
		}
		clientName := "Chat"

		ctx := context.Background()

		var list []ory.OAuth2Client
		var response *http.Response
		var err error
		for i := 0; i < 3; i++ {
			list, response, err = private().OAuth2Api.ListOAuth2Clients(ctx).ClientName(clientName).Execute()
			if errors.Is(err, syscall.ECONNREFUSED) {
				time.Sleep(time.Second * 2)
				continue
			} else {
				break
			}
		}
		if err != nil {
			err = fmt.Errorf("auth: an error occured when calling OAuth2Api.ListOAuth2Clients: %w\nfull HTTP response: %v\n", err, response)
			ctxLogger.Fatal(err.Error())
		}

		if len(list) > 0 {
			oAuth2Client = &list[0]
			oAuth2Client.SetClientSecret(clientSecret)
			return
		}

		oAuth2Client = ory.NewOAuth2Client()
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

		oAuth2Client, response, err = private().OAuth2Api.CreateOAuth2Client(ctx).OAuth2Client(*oAuth2Client).Execute()
		if err != nil {
			err = fmt.Errorf("auth: an error occured when calling OAuth2Api.CreateOAuth2Client: %w\nfull HTTP response: %v\n", err, response)
			ctxLogger.Fatal(err.Error())
		}
	})

	return oAuth2Client
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

type oAuthManager struct {
	config        oauth2.Config
	statesStorage SetGetterEx
}

func (m *oAuthManager) AuthRequest(ctx context.Context, r Redirector) error {
	return m.authRequest(ctx, r, oauth2.SetAuthURLParam("redirect_uri", "https://localhost:8000/authentication/finish"))
}

func (m *oAuthManager) SilentAuthRequest(ctx context.Context, r Redirector) error {
	params := []oauth2.AuthCodeOption{
		oauth2.SetAuthURLParam("prompt", "none"),
		oauth2.SetAuthURLParam("redirect_uri", "https://localhost:8000/authentication/finish/silent"),
	}
	return m.authRequest(ctx, r, params...)
}

func (m *oAuthManager) ExchangeForTokens(ctx context.Context, state string, code string, uri string) (TokenInfoRetriver, error) {
	if _, err := m.statesStorage.GetEx(ctx, fmt.Sprintf(keyStateTemplate, state), time.Duration(0)); err != nil {
		return nil, err
	}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, httpClient)
	tokens, err := m.config.Exchange(ctx, code, oauth2.SetAuthURLParam("redirect_uri", uri))
	if err != nil {
		return nil, err
	}
	return &oAuthTokens{tokens}, nil
}

func (m *oAuthManager) LoginRequest(ctx context.Context, loginChallenge string, r Redirector) (skipped bool, err error) {
	loginRequest, response, err := private().OAuth2Api.GetOAuth2LoginRequest(ctx).LoginChallenge(loginChallenge).Execute()
	if err != nil {
		return false, fmt.Errorf("auth: an error occured when calling OAuth2Api.GetOAuth2LoginRequest: %w\nfull HTTP response: %v", err, response)
	}
	if loginRequest.GetSkip() {
		err = m.AcceptLoginRequest(ctx, loginRequest.GetSubject(), loginChallenge, r)
		return err == nil, err
	}
	return
}

func (m *oAuthManager) AcceptLoginRequest(ctx context.Context, sub string, loginChallenge string, r Redirector) error {
	acceptRequest := ory.NewAcceptOAuth2LoginRequest(sub)
	acceptRequest.SetRemember(true)
	acceptRequest.SetRememberFor(15552000) //6 months
	acceptRequest.SetExtendSessionLifespan(true)
	redirectTo, response, err := private().OAuth2Api.AcceptOAuth2LoginRequest(ctx).LoginChallenge(loginChallenge).AcceptOAuth2LoginRequest(*acceptRequest).Execute()
	if err != nil {
		return fmt.Errorf("auth: an error occured when calling OAuth2Api.AcceptOAuth2LoginRequest: %w\nfull HTTP response: %v", err, response)
	}
	r.Redirect(redirectTo.GetRedirectTo())
	return nil
}

func (m *oAuthManager) AcceptConsentRequest(ctx context.Context, consentChallenge string, r Redirector) error {
	acceptRequest := ory.NewAcceptOAuth2ConsentRequest()
	acceptRequest.SetGrantScope(m.config.Scopes)
	acceptRequest.SetRemember(true)
	acceptRequest.SetRememberFor(15552000) //6 months
	redirectTo, response, err := private().OAuth2Api.AcceptOAuth2ConsentRequest(ctx).ConsentChallenge(consentChallenge).AcceptOAuth2ConsentRequest(*acceptRequest).Execute()
	if err != nil {
		return fmt.Errorf("auth: an error occured when calling OAuth2Api.AcceptOAuth2ConsentRequest: %w\nfull HTTP response: %v", err, response)
	}
	r.Redirect(redirectTo.GetRedirectTo())
	return nil
}

func (m *oAuthManager) LogoutRequest(ctx context.Context, logoutChallenge string, refreshToken string, r ResetTokensRedirector) (err error) {
	_, response, err := private().OAuth2Api.GetOAuth2LogoutRequest(ctx).LogoutChallenge(logoutChallenge).Execute()
	if err != nil {
		return fmt.Errorf("auth: an error occured when calling OAuth2Api.GetOAuth2LogoutRequest: %w\nfull HTTP response: %v", err, response)
	}

	redirectTo, response, err := private().OAuth2Api.AcceptOAuth2LogoutRequest(ctx).LogoutChallenge(logoutChallenge).Execute()
	if err != nil {
		return fmt.Errorf("auth: an error occured when calling OAuth2Api.AcceptOAuth2LogoutRequest: %w\nfull HTTP response: %v", err, response)
	}

	if err = m.RevokeToken(ctx, refreshToken, r); err != nil {
		return
	}

	r.Redirect(redirectTo.GetRedirectTo())
	return
}

func (m *oAuthManager) RevokeToken(ctx context.Context, token string, r TokensReseter) (err error) {
	if token == "" {
		return
	}
	revokeTokenRequest := public().OAuth2Api.RevokeOAuth2Token(ctx).ClientId(m.config.ClientID).ClientSecret(m.config.ClientSecret)
	response, err := revokeTokenRequest.Token(token).Execute()
	if err != nil {
		err = fmt.Errorf("auth: an error occured when calling OAuth2Api.RevokeOAuth2Token: %w\nfull HTTP response: %v", err, response)
	}
	r.ResetTokens()
	return
}

func (m *oAuthManager) RefreshTokens(ctx context.Context, accessToken string, refreshToken string) (TokenInfoRetriver, error) {
	tokenInfo, response, err := private().OAuth2Api.IntrospectOAuth2Token(ctx).Token(accessToken).Execute()
	if err != nil {
		return nil, fmt.Errorf("auth: an error occured when calling OAuth2Api.IntrospectOAuth2Token: %w\nfull HTTP response: %v", err, response)
	}

	if !(tokenInfo.GetActive() && tokenInfo.GetTokenUse() == tokenTypeAccess && tokenInfo.GetClientId() == m.config.ClientID) {
		return nil, errors.New("auth: invalid access token")
	}

	ctx = context.WithValue(ctx, oauth2.HTTPClient, httpClient)
	tokens, err := m.config.Exchange(ctx, "", oauth2.SetAuthURLParam("grant_type", "refresh_token"), oauth2.SetAuthURLParam(tokenTypeRefresh, refreshToken))
	if err != nil {
		return nil, err
	}
	return &oAuthTokens{tokens}, nil
}

func (m *oAuthManager) GetSubByToken(ctx context.Context, accessToken string) (string, error) {
	tokenInfo, err := m.introspectAccessToken(ctx, accessToken)
	if err != nil {
		return "", err
	}
	return (*tokenInfo).GetSub(), nil
}

func (m *oAuthManager) AccessTokenIsActive(ctx context.Context, accessToken string) (bool, error) {
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

func (m *oAuthManager) introspectAccessToken(ctx context.Context, accessToken string) (*ory.IntrospectedOAuth2Token, error) {
	tokenInfo, response, err := private().OAuth2Api.IntrospectOAuth2Token(ctx).Token(accessToken).Execute()
	if err != nil {
		return nil, fmt.Errorf("auth: an error occured when calling OAuth2Api.IntrospectOAuth2Token: %w\nfull HTTP response: %v", err, response)
	}
	if !tokenInfo.GetActive() {
		return nil, ErrExpiredToken
	}

	if !(tokenInfo.GetTokenUse() == tokenTypeAccess && tokenInfo.GetClientId() == m.config.ClientID) {
		return nil, errors.New("auth: invalid access token")
	}
	return tokenInfo, err
}

func (m *oAuthManager) authRequest(ctx context.Context, r Redirector, opts ...oauth2.AuthCodeOption) error {
	state, err := generateState()
	if err != nil {
		return err
	}
	if err = m.statesStorage.SetEx(ctx, fmt.Sprintf(keyStateTemplate, state), true, time.Minute*30); err != nil {
		return err
	}
	r.Redirect(m.config.AuthCodeURL(state, opts...))
	return nil
}

func generateState() (string, error) {
	b := make([]byte, 43)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}
