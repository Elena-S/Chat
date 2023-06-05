package vault

import (
	"context"
	"errors"
	"log"
	"os"
	"sync"

	vault "github.com/hashicorp/vault/api"
	auth "github.com/hashicorp/vault/api/auth/approle"
)

var (
	once   sync.Once
	client *vault.Client
)

func Client() *vault.Client {
	var err error

	once.Do(func() {
		config := vault.DefaultConfig()
		//needs config
		config.Address = "http://vault:8100"

		client, err = vault.NewClient(config)
		if err != nil {
			log.Fatal(err)
		}

		done := make(chan struct{})
		go renewToken(client, done)
		<-done
	})

	return client
}

func renewToken(client *vault.Client, ch chan struct{}) {
	closed := false
	for {
		vaultLoginResp, err := login(client)
		if err != nil {
			log.Fatalf("vault: %v", err.Error())
		}
		if !closed {
			close(ch)
			closed = true
		}
		if !vaultLoginResp.Auth.Renewable {
			log.Printf("vault: token is not configured to be renewable.")
			break
		}
		err = manageTokenLifecycle(client, vaultLoginResp)
		if err != nil {
			log.Fatalf("vault: unable to start managing token lifecycle: %v", err)
		}
	}
}

func manageTokenLifecycle(client *vault.Client, token *vault.Secret) error {

	watcher, err := client.NewLifetimeWatcher(&vault.LifetimeWatcherInput{
		Secret:    token,
		Increment: 3600,
	})
	if err != nil {
		return err
	}

	go watcher.Start()
	defer watcher.Stop()

	for {
		select {
		case err := <-watcher.DoneCh():
			if err != nil {
				log.Printf("vault: failed to renew token: %v. Re-attempting login.", err)
				return nil
			}
			log.Printf("vault: token can no longer be renewed. Re-attempting login.")
			return nil
		case renewal := <-watcher.RenewCh():
			log.Printf("vault: successfully renewed: %#v", renewal)
		}
	}
}

func login(client *vault.Client) (*vault.Secret, error) {
	roleID := os.Getenv("APPROLE_ROLE_ID")
	if roleID == "" {
		return nil, errors.New("vault: no role ID was provided in APPROLE_ROLE_ID env var")
	}

	secretID := &auth.SecretID{FromEnv: "APPROLE_SECRET_ID"}

	appRoleAuth, err := auth.NewAppRoleAuth(
		roleID,
		secretID,
	)
	if err != nil {
		return nil, err
	}

	authInfo, err := client.Auth().Login(context.Background(), appRoleAuth)
	if err != nil {
		return nil, err
	}
	if authInfo == nil {
		return nil, errors.New("vault: no auth info was returned after login")
	}

	return authInfo, nil
}
