package vault

import (
	"context"
	"encoding/base64"
	"errors"
)

const secretPath = "kv/chat/user_profiles"

var ErrInvalidFormat = errors.New("vault: invalid format of the storaged secret")

type Secret struct {
	Salt         []byte
	PasswordHash string
}

func WriteSecret(ctx context.Context, login string, secret Secret) error {
	_, err := Client().KVv2(secretPath).Put(ctx, login,
		map[string]any{
			"salt":          secret.Salt,
			"password_hash": secret.PasswordHash,
		})
	return err
}

func ReadSecret(ctx context.Context, login string) (secret Secret, err error) {
	data, err := Client().KVv2(secretPath).Get(ctx, login)
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

func DeleteSecrete(ctx context.Context, login string) error {
	return Client().KVv2(secretPath).Delete(ctx, login)
}
