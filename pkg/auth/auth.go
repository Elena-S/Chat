package auth

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Elena-S/Chat/pkg/redis"
	ory "github.com/ory/client-go"
	"golang.org/x/oauth2"
)

const (
	tokenTypeAccess  = "access_token"
	tokenTypeRefresh = "refresh_token"
)

const keyStateTemplate = "oAuth2:state:%s"

var httpClient *http.Client
var OAuthManager *oAuthManager
var (
	oAuth2Client     *ory.OAuth2Client
	oAuth2ClientOnce sync.Once
	privateOnce      sync.Once
	privateClient    *ory.APIClient
	publicOnce       sync.Once
	publicClient     *ory.APIClient
)

type SetGetExCloser interface {
	SetEx(ctx context.Context, key string, value any, expiration time.Duration) error
	GetEx(ctx context.Context, key string, expiration time.Duration) (string, error)
	io.Closer
}

type Redirector interface {
	Redirect(url string)
}

type ReseterTokens interface {
	ResetTokens()
}

type ResetTokensRedirector interface {
	Redirector
	ReseterTokens
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
		httpClient:    httpClient,
		statesStorage: redis.Client(),
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
		const envClientSecret = "HYDRA_OAUTH2_CLIENT_SECRET"
		clientSecret := os.Getenv(envClientSecret)
		if clientSecret == "" {
			log.Fatalf("auth: no client secret was provided in %s env var", envClientSecret)
		}
		clientName := "Chat"

		ctx := context.Background()

		list, response, err := private().OAuth2Api.ListOAuth2Clients(ctx).ClientName(clientName).Execute()
		if err != nil {
			log.Fatalf("auth: an error occured when calling OAuth2Api.ListOAuth2Clients: %v\nfull HTTP response: %v\n", err, response)
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
		oAuth2Client.SetRedirectUris([]string{"https://localhost:8000/authentication/finish"})
		oAuth2Client.SetBackchannelLogoutUri("https://localhost:8000/authentication/logout")
		oAuth2Client.SetBackchannelLogoutSessionRequired(true)
		oAuth2Client.SetPostLogoutRedirectUris([]string{"https://localhost:8000"})

		oAuth2Client, response, err = private().OAuth2Api.CreateOAuth2Client(ctx).OAuth2Client(*oAuth2Client).Execute()
		if err != nil {
			log.Fatalf("auth: an error occured when calling OAuth2Api.CreateOAuth2Client: %v\nfull HTTP response: %v\n", err, response)
		}
	})

	return oAuth2Client
}

type TokenInfoRetriver interface {
	AccessToken() string
	RefreshToken() string
	Expiry() time.Time
}

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
	httpClient    *http.Client
	statesStorage SetGetExCloser
}

func (m *oAuthManager) AuthRequest(ctx context.Context, r Redirector, opts ...oauth2.AuthCodeOption) error {
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

func (m *oAuthManager) ExchangeForTokens(ctx context.Context, state string, code string) (TokenInfoRetriver, error) {
	if _, err := m.statesStorage.GetEx(ctx, fmt.Sprintf(keyStateTemplate, state), time.Duration(0)); err != nil {
		return nil, err
	}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, m.httpClient)
	tokens, err := m.config.Exchange(ctx, code)
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
		err = m.AcceptLoginRequest(ctx, loginRequest.GetSubject(), loginChallenge, true, r)
		return err == nil, err
	}
	return
}

func (m *oAuthManager) AcceptLoginRequest(ctx context.Context, sub string, loginChallenge string, remember bool, r Redirector) error {
	acceptRequest := ory.NewAcceptOAuth2LoginRequest(sub)
	acceptRequest.SetRemember(remember)
	acceptRequest.SetRememberFor(15552000) //6 months
	acceptRequest.SetExtendSessionLifespan(remember)
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

func (m *oAuthManager) RevokeToken(ctx context.Context, token string, r ReseterTokens) (err error) {
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

	ctx = context.WithValue(ctx, oauth2.HTTPClient, m.httpClient)
	tokens, err := m.config.Exchange(ctx, "", oauth2.SetAuthURLParam("grant_type", "refresh_token"), oauth2.SetAuthURLParam(tokenTypeRefresh, refreshToken))
	if err != nil {
		return nil, err
	}
	return &oAuthTokens{tokens}, nil
}

func (m *oAuthManager) GetUserIDByToken(ctx context.Context, accessToken string) (uint, error) {
	tokenInfo, response, err := private().OAuth2Api.IntrospectOAuth2Token(ctx).Token(accessToken).Execute()
	if err != nil {
		return 0, fmt.Errorf("auth: an error occured when calling OAuth2Api.IntrospectOAuth2Token: %w\nfull HTTP response: %v", err, response)
	}

	if !tokenInfo.GetActive() {
		return 0, errors.New("auth: the access token is expired")
	}

	if !(tokenInfo.GetTokenUse() == tokenTypeAccess && tokenInfo.GetClientId() == m.config.ClientID) {
		return 0, errors.New("auth: invalid access token")
	}

	id, err := strconv.ParseUint((*tokenInfo).GetSub(), 10, 64)
	if err != nil {
		return 0, err
	}
	return uint(id), nil
}

func (m *oAuthManager) Close() error {
	return m.statesStorage.Close()
}

func generateState() (string, error) {
	b := make([]byte, 43)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}
