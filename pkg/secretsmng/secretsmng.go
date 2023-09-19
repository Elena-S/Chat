package secretsmng

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"

	"github.com/Elena-S/Chat/pkg/logger"
	vault "github.com/hashicorp/vault/api"
	auth "github.com/hashicorp/vault/api/auth/approle"
	"go.uber.org/fx"
)

var (
	ErrInvalidFormat = errors.New("secretsmng: invalid format of the storaged secret")
	ErrEmptyPassword = errors.New("secretsmng: got an empty password")
	ErrNoLoginInfo   = errors.New("secretsmng: no info was returned after login")
)

const keySalt = "salt"
const keyPasswordHash = "password_hash"

type Password string

func (p Password) IsEmpty() bool {
	return p == ""
}

type Secret struct {
	salt         []byte
	passwordHash string
}

func NewSecret(password Password) (secret *Secret, err error) {
	secret = new(Secret)
	err = secret.Set(password)
	if err != nil {
		secret = nil
	}
	return
}

func (s *Secret) Set(password Password) (err error) {
	if password == "" {
		err = ErrEmptyPassword
		return
	}
	s.salt, err = randSalt()
	if err != nil {
		return
	}
	s.passwordHash = hash(s.salt, string(password))
	return
}

func (s *Secret) IsEmpty() bool {
	return s.passwordHash == ""
}

func (s *Secret) Compare(password Password) (ok bool) {
	passwordHash := hash(s.salt, string(password))
	return passwordHash == s.passwordHash
}

var Module = fx.Module("vault",
	fx.Provide(
		NewClient,
	),
	fx.Invoke(registerFunc),
)

type Client struct {
	client      *vault.Client
	appRoleAuth *auth.AppRoleAuth
	secretPath  string
	ttl         int
	context     context.Context
	logger      *logger.Logger
	shutdowner  fx.Shutdowner
}

type ClientParams struct {
	fx.In
	Logger     *logger.Logger
	Context    context.Context
	Shutdowner fx.Shutdowner
}

func NewClient(p ClientParams) (c *Client, err error) {
	config := vault.DefaultConfig()
	//TODO: need config
	config.Address = "http://vault:8100"
	client, err := vault.NewClient(config)
	if err != nil {
		return
	}
	appRoleAuth, err := auth.NewAppRoleAuth(os.Getenv("APPROLE_ROLE_ID"), &auth.SecretID{FromEnv: "APPROLE_SECRET_ID"})
	if err != nil {
		return
	}

	c = &Client{
		logger:      p.Logger,
		client:      client,
		appRoleAuth: appRoleAuth,
		//TODO: need config
		ttl:        3600,
		secretPath: "kv/chat/user_profiles",
		context:    p.Context,
		shutdowner: p.Shutdowner,
	}
	return
}

func registerFunc(lc fx.Lifecycle, c *Client) {
	ctxMng, cancelFunc := context.WithCancel(c.context)
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) (err error) {
			ctxLogger := c.logger.WithEventField("secrets storage login")
			ctxLogger.Info("start")
			defer ctxLogger.OnDefer("secretsmng", err, nil, "finish")

			token, err := c.login(ctx)
			if err != nil {
				return
			}
			if token.Auth.Renewable {
				go c.renewToken(ctxMng, token)
			}
			return
		},
		OnStop: func(ctx context.Context) (err error) {
			cancelFunc()
			return
		},
	})
}

func (c *Client) WriteSecret(ctx context.Context, login string, secret *Secret) error {
	_, err := c.client.KVv2(c.secretPath).Put(ctx, login,
		map[string]any{
			keySalt:         secret.salt,
			keyPasswordHash: secret.passwordHash,
		})
	return err
}

func (c *Client) ReadSecret(ctx context.Context, login string) (secret *Secret, err error) {
	data, err := c.client.KVv2(c.secretPath).Get(ctx, login)
	if err != nil {
		return
	}

	value, ok := data.Data[keySalt]
	if !ok {
		err = ErrInvalidFormat
		return
	}
	valueStr, ok := value.(string)
	if !ok {
		err = ErrInvalidFormat
		return
	}

	salt, err := base64.StdEncoding.DecodeString(valueStr)
	if err != nil {
		return
	}

	value, ok = data.Data[keyPasswordHash]
	if !ok {
		err = ErrInvalidFormat
		return
	}
	passwordHash, ok := value.(string)
	if !ok {
		err = ErrInvalidFormat
		return
	}
	secret = new(Secret)
	secret.salt = salt
	secret.passwordHash = passwordHash
	return
}

func (c *Client) DeleteSecrete(ctx context.Context, login string) error {
	return c.client.KVv2(c.secretPath).Delete(ctx, login)
}

func (c *Client) ValidateSecret(ctx context.Context, login string, password Password) (ok bool, err error) {
	secret, err := c.ReadSecret(ctx, login)
	if err != nil {
		return
	}
	ok = secret.Compare(password)
	return
}

func (c *Client) login(ctx context.Context) (token *vault.Secret, err error) {
	token, err = c.client.Auth().Login(ctx, c.appRoleAuth)
	if err != nil {
		return
	} else if token == nil {
		err = ErrNoLoginInfo
		return
	}
	return
}

func (c *Client) renewToken(ctx context.Context, token *vault.Secret) {
	var err error
	ctxLogger := c.logger.WithEventField("secrets storage token renew")
	ctxLogger.Info("start")
	defer func() {
		data := recover()
		ctxLogger.OnDefer("secretsmng", err, data, "finish")
		if !(data == nil && err == nil) {
			if err = c.shutdowner.Shutdown(fx.ExitCode(1)); err != nil {
				ctxLogger.Error(err.Error())
			}
		}
	}()

	for {
		var watcher *vault.LifetimeWatcher
		watcher, err = c.client.NewLifetimeWatcher(&vault.LifetimeWatcherInput{
			Secret:    token,
			Increment: c.ttl,
		})
		if err != nil {
			return
		}

		go watcher.Start()
		defer watcher.Stop()

	loop:
		for {
			select {
			case err = <-watcher.DoneCh():
				if err != nil {
					ctxLogger.Info(fmt.Sprintf("secretsmng: failed to renew token: %v. Re-attempting login", err))
				} else {
					ctxLogger.Info("secretsmng: token can no longer be renewed. Re-attempting login")
				}
				token, err = c.login(ctx)
				if err != nil {
					return
				}
				break loop
			case renewal := <-watcher.RenewCh():
				ctxLogger.Info(fmt.Sprintf("secretsmng: successfully renewed: %#v", renewal))
			case <-ctx.Done():
				err = ctx.Err()
				return
			}
		}
	}
}

func hash(salt []byte, s string) string {
	var buf bytes.Buffer
	buf.Write(salt)
	buf.WriteString(s)
	b := crypto.SHA1.New().Sum(buf.Bytes())
	return fmt.Sprintf("%x", b)
}

func randSalt() ([]byte, error) {
	b := make([]byte, 16)
	n, err := rand.Read(b)
	if err != nil {
		return nil, err
	}
	return b[:n], nil
}
