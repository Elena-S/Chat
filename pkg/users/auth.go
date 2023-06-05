package users

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"errors"
	"fmt"
	"regexp"
)

var (
	ErrInvalidLoginFormat = errors.New("users: invalid login format")
	ErrInvalidCredentials = errors.New("users: login and password shouldn't be empty")
	ErrInvalidRegData     = errors.New("users: first name should be filled")
)

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
	matched, err := regexp.MatchString(`^\+(?:[0-9] ?){6,14}[0-9]$`, login)
	if err != nil {
		return err
	}
	if !matched {
		return ErrInvalidLoginFormat
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
