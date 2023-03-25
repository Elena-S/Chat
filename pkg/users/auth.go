package users

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"errors"
	"fmt"
	"sync"
)

var (
	ErrInvalidCredentials = errors.New("login and password shouldn't be empty")
	ErrInvalidRegData     = errors.New("first name should be filled")
)

var tokenCashe sync.Map //JWT token

func NewToken(user User) (string, error) {
	token, err := token(user.Login())
	if err != nil {
		return token, err
	}
	tokenCashe.Store(token, user)
	return token, nil
}

func GetUserByToken(token string) (User, error) {
	user := User{}
	if token == "" {
		return user, errors.New("missing token")
	}

	value, ok := tokenCashe.Load(token)
	if !ok {
		return user, errors.New("gotten wrong token")
	}

	user, ok = value.(User)
	if !ok {
		return user, errors.New("mismatched type of user")
	}

	return user, nil
}

func DeleteToken(token string) {
	tokenCashe.Delete(token)
}

func token(s string) (string, error) {
	salt, err := randSalt()
	if err != nil {
		return "", nil
	}
	return hash(salt, s), nil
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

func checkCredentials(login, pwd string) error {
	if login == "" || pwd == "" {
		return ErrInvalidCredentials
	}
	return nil
}

func checkName(name string) error {
	if name == "" {
		return ErrInvalidRegData
	}
	return nil
}

func checkRegData(login, pwd, fistName string) error {
	err := checkCredentials(login, pwd)
	if err != nil {
		return err
	}
	err = checkName(fistName)
	if err != nil {
		return err
	}
	return err
}
