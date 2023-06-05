package hydra

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	ory "github.com/ory/client-go"
	"golang.org/x/oauth2"
)

const (
	TokenTypeAccess       = "access_token"
	TokenTypeRefresh      = "refresh_token"
	GrantTypeRefreshToken = "refresh_token"
)

var States sync.Map //Redis
var httpClient *http.Client

var (
	oAuth2Client     *ory.OAuth2Client
	oAuth2ClientOnce sync.Once
	privateOnce      sync.Once
	privateClient    *ory.APIClient
	publicOnce       sync.Once
	publicClient     *ory.APIClient
)

var OAuthConf *oAuthConf

func init() {
	httpClient = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: time.Second * 30}

	OAuthConf = &oAuthConf{
		Config: oauth2.Config{
			ClientID:     oAuth2HydraClient().GetClientId(),
			ClientSecret: oAuth2HydraClient().GetClientSecret(),
			Scopes:       strings.Split(oAuth2HydraClient().GetScope(), " "),
			Endpoint: oauth2.Endpoint{
				AuthURL:   "https://localhost:4444/oauth2/auth",
				TokenURL:  "https://hydra:4444/oauth2/token",
				AuthStyle: oauth2.AuthStyleInParams,
			},
		},
		HTTPClient: httpClient}
}

func PrivateClient() *ory.APIClient {
	privateOnce.Do(func() {
		cfg := ory.NewConfiguration()
		cfg.Servers = ory.ServerConfigurations{{URL: "https://hydra:4445"}}
		cfg.HTTPClient = httpClient
		privateClient = ory.NewAPIClient(cfg)
	})

	return privateClient
}

func PublicClient() *ory.APIClient {
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
		context := context.Background()

		clientName := "Chat"
		clientSecret := os.Getenv("HYDRA_OAUTH2_CLIENT_SECRET")

		list, response, err := PrivateClient().OAuth2Api.ListOAuth2Clients(context).ClientName(clientName).Execute()
		if err != nil {
			log.Fatalf("hydra: an error occured when calling OAuth2Api.ListOAuth2Clients: %v\nfull HTTP response: %v\n", err, response)
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

		oAuth2Client, response, err = PrivateClient().OAuth2Api.CreateOAuth2Client(context).OAuth2Client(*oAuth2Client).Execute()
		if err != nil {
			log.Fatalf("hydra: an error occured when calling OAuth2Api.CreateOAuth2Client: %v\nfull HTTP response: %v\n", err, response)
		}
	})

	return oAuth2Client
}

type oAuthConf struct {
	InnerAuthURL string
	Config       oauth2.Config
	HTTPClient   *http.Client
}

func (conf *oAuthConf) OAuthURL(opts ...oauth2.AuthCodeOption) (string, error) {
	state, err := generateState()
	if err != nil {
		return "", err
	}
	States.Store(state, struct{}{}) //Redis with expiration date
	return conf.Config.AuthCodeURL(state, opts...), err
}

func generateState() (string, error) {
	b := make([]byte, 43)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}
