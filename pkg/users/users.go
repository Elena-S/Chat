package users

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Elena-S/Chat/pkg/database"
)

var (
	ErrUsrExists        = errors.New("a user with this phone number already exists")
	ErrNumUsr           = errors.New("there is more than 1 user with the same pnone number")
	ErrWrongCredentials = errors.New("wrong login or password")
)

type User struct {
	id           uint
	phone        string
	firstName    string
	lastName     string
	fullName     string
	searchName   string
	salt         []byte
	passwordHash string
}

func (user *User) Authorize(login, pwd string) (err error) {
	err = checkCredentials(login, pwd)
	if err != nil {
		return err
	}

	user.phone = login

	ok, err := user.exists()
	if err != nil {
		return err
	} else if !ok {
		return ErrWrongCredentials
	}
	pwdHash := hash(user.salt, pwd)

	if pwdHash != user.passwordHash {
		err = ErrWrongCredentials
	}
	return err
}

func (user *User) Register(login, pwd, firstName, lastName string) (err error) {
	err = checkRegData(login, pwd, firstName)
	if err != nil {
		return
	}

	salt, err := randSalt()
	if err != nil {
		return
	}

	user.phone = login
	user.firstName = strings.TrimSpace(firstName)
	user.lastName = strings.TrimSpace(lastName)
	user.fullName = fmt.Sprintf("%s %s", firstName, lastName)
	user.searchName = strings.ToLower(user.fullName)
	user.salt = salt
	user.passwordHash = hash(salt, pwd) //needs store in Vault

retry:
	err = user.createNX()
	if database.SerializationFailureError(err) {
		goto retry
	}

	return
}

func (user *User) FullName() string {
	return user.fullName
}

func (user *User) ID() uint {
	return user.id
}

func (user *User) Login() string {
	return user.phone
}

func (user *User) createNX() (err error) {
	tx, err := database.DB().BeginTx(context.Background(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	defer tx.Rollback()

	ok, err := user.exists()
	if err != nil {
		return
	} else if ok {
		err = ErrUsrExists
		return
	}

	err = user.create(tx)

	if err != nil {
		return
	}

	err = tx.Commit()
	return
}

func (user *User) exists() (exists bool, err error) {
	rows, err := database.DB().Query(`
	SELECT
		id, first_name, last_name, full_name, salt, password_hash
	FROM users
	WHERE phone = $1`, user.phone)
	if err != nil {
		return
	}
	defer rows.Close()
	if rows.Next() {
		err = rows.Scan(&user.id, &user.firstName, &user.lastName, &user.fullName, &user.salt, &user.passwordHash)
		if err != nil {
			return
		}
		exists = true
	}
	if rows.Next() {
		err = ErrNumUsr
	}
	return
}

func (user *User) create(tx *sql.Tx) (err error) {
	return tx.QueryRow(`
	INSERT INTO users (phone, first_name, last_name, full_name, search_name, password_hash, salt)
	VALUES ($1, $2, $3, $4, $5, $6, $7)
	RETURNING id`,
		user.phone, user.firstName, user.lastName,
		user.fullName, user.searchName, user.passwordHash, user.salt).Scan(&user.id)
}

func (user *User) MarshalJSON() (data []byte, err error) {
	var buf bytes.Buffer
	_, err = buf.WriteString("{\"ID\":")
	if err != nil {
		return
	}
	_, err = buf.WriteString(strconv.FormatUint(uint64(user.id), 10))
	if err != nil {
		return
	}
	_, err = buf.WriteString(", \"Phone\": \"")
	if err != nil {
		return
	}
	_, err = buf.WriteString(user.phone)
	if err != nil {
		return
	}
	_, err = buf.WriteString("\", \"FullName\": \"")
	if err != nil {
		return
	}
	_, err = buf.WriteString(user.fullName)
	if err != nil {
		return
	}
	_, err = buf.WriteString("\", \"FirstName\": \"")
	if err != nil {
		return
	}
	_, err = buf.WriteString(user.firstName)
	if err != nil {
		return
	}
	_, err = buf.WriteString("\", \"LastName\": \"")
	if err != nil {
		return
	}
	_, err = buf.WriteString(user.lastName)
	if err != nil {
		return
	}
	_, err = buf.WriteString("\"}")
	if err != nil {
		return
	}

	return buf.Bytes(), nil
}
