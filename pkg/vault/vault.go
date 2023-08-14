package vault

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/Elena-S/Chat/pkg/logger"
	"github.com/Elena-S/Chat/pkg/srcmng"
	vault "github.com/hashicorp/vault/api"
	auth "github.com/hashicorp/vault/api/auth/approle"
)

var _ srcmng.SourceManager = (*storage)(nil)

var ErrInvalidFormat = errors.New("vault: invalid format of the storaged secret")

const secretPath = "kv/chat/user_profiles"

var SecretStorage *storage = &storage{
	chLoggedIn: make(chan struct{}),
	chClosed:   make(chan struct{}),
}

type Secret struct {
	Salt         []byte
	PasswordHash string
}

type storage struct {
	onceLaunch   sync.Once
	client       *vault.Client
	onceClose    sync.Once
	chClosed     chan struct{}
	onceLoggedIn sync.Once
	chLoggedIn   chan struct{}
}

func (s *storage) MustLaunch() {
	s.onceLaunch.Do(func() {
		config := vault.DefaultConfig()
		//needs config
		config.Address = "http://vault:8100"

		var err error
		s.client, err = vault.NewClient(config)
		if err != nil {
			logger.ChatLogger.Fatal(err.Error())
		}
		go s.renewToken()
		<-s.chLoggedIn
	})
}

func (s *storage) Close() error {
	s.onceClose.Do(func() { close(s.chClosed) })
	return nil
}

func (s *storage) WriteSecret(ctx context.Context, login string, secret Secret) error {
	_, err := s.initClient().KVv2(secretPath).Put(ctx, login,
		map[string]any{
			"salt":          secret.Salt,
			"password_hash": secret.PasswordHash,
		})
	return err
}

func (s *storage) ReadSecret(ctx context.Context, login string) (secret Secret, err error) {
	data, err := s.initClient().KVv2(secretPath).Get(ctx, login)
	if err != nil {
		return
	}

	value, ok := data.Data["salt"]
	if !ok {
		err = ErrInvalidFormat
		return
	}
	valueStr, ok := value.(string)
	if !ok {
		err = ErrInvalidFormat
		return
	}
	secret.Salt, err = base64.StdEncoding.DecodeString(valueStr)
	if err != nil {
		return
	}

	value, ok = data.Data["password_hash"]
	if !ok {
		err = ErrInvalidFormat
		return
	}
	secret.PasswordHash, ok = value.(string)
	if !ok {
		err = ErrInvalidFormat
		return
	}

	return
}

func (s *storage) DeleteSecrete(ctx context.Context, login string) error {
	return s.initClient().KVv2(secretPath).Delete(ctx, login)
}

func (s *storage) initClient() *vault.Client {
	s.MustLaunch()
	return s.client
}

func (s *storage) renewToken() {
	for {
		vaultLoginResp, err := s.login()
		if err != nil {
			logger.ChatLogger.Fatal(fmt.Sprintf("vault: %v", err.Error()))
		}

		s.onceLoggedIn.Do(func() { close(s.chLoggedIn) })

		if !vaultLoginResp.Auth.Renewable {
			logger.ChatLogger.Info("vault: token is not configured to be renewable.")
			break
		}

		err, stop := s.manageTokenLifecycle(vaultLoginResp)
		if err != nil {
			logger.ChatLogger.Fatal(fmt.Sprintf("vault: unable to start managing token lifecycle: %v", err))
		}
		if stop {
			break
		}
	}
}

func (s *storage) login() (*vault.Secret, error) {
	const envAppRoleID = "APPROLE_ROLE_ID"
	roleID := os.Getenv(envAppRoleID)
	if roleID == "" {
		return nil, fmt.Errorf("vault: no role ID was provided in %s env var", envAppRoleID)
	}

	secretID := &auth.SecretID{FromEnv: "APPROLE_SECRET_ID"}

	appRoleAuth, err := auth.NewAppRoleAuth(
		roleID,
		secretID,
	)
	if err != nil {
		return nil, err
	}

	authInfo, err := s.client.Auth().Login(context.Background(), appRoleAuth)
	if err != nil {
		return nil, err
	}
	if authInfo == nil {
		return nil, errors.New("vault: no auth info was returned after login")
	}

	return authInfo, nil
}

func (s *storage) manageTokenLifecycle(token *vault.Secret) (error, bool) {

	watcher, err := s.client.NewLifetimeWatcher(&vault.LifetimeWatcherInput{
		Secret:    token,
		Increment: 3600,
	})
	if err != nil {
		return err, true
	}

	go watcher.Start()
	defer watcher.Stop()

	for {
		select {
		case err := <-watcher.DoneCh():
			if err != nil {
				logger.ChatLogger.Info(fmt.Sprintf("vault: failed to renew token: %v. Re-attempting login.", err))
				return nil, false
			}
			logger.ChatLogger.Info("vault: token can no longer be renewed. Re-attempting login.")
			return nil, false
		case renewal := <-watcher.RenewCh():
			logger.ChatLogger.Info(fmt.Sprintf("vault: successfully renewed: %#v", renewal))
		case <-s.chClosed:
			return nil, true
		}
	}
}
