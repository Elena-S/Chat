package users

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/Elena-S/Chat/pkg/database"
	"github.com/Elena-S/Chat/pkg/secretsmng"
	"go.uber.org/fx"
)

var (
	ErrExists             = errors.New("users: a user with this phone number already exists")
	ErrNumUsr             = errors.New("users: there is more than 1 user with the same phone number")
	ErrWrongCredentials   = errors.New("users: wrong phone or password")
	ErrInvalidPhoneFormat = errors.New("users: invalid phone format")
	ErrInvalidCredentials = errors.New("users: phone and password shouldn't be empty")
	ErrInvalidRegData     = errors.New("users: first name should be filled")
)

type UserID uint

func (t UserID) String() string {
	return strconv.FormatUint(uint64(t), 10)
}

type User struct {
	id         UserID
	phone      string
	firstName  string
	lastName   string
	fullName   string
	searchName string
	secret     *secretsmng.Secret
}

func NewUser(regData RegData) (user User, err error) {
	user.phone = regData.Phone
	user.firstName = strings.TrimSpace(regData.FirstName)
	user.lastName = strings.TrimSpace(regData.LastName)
	user.fullName = fmt.Sprintf("%s %s", user.firstName, user.lastName)
	user.searchName = strings.ToLower(user.fullName)
	user.secret, err = secretsmng.NewSecret(regData.Password)
	return
}

func (u User) ID() UserID {
	return u.id
}

func (u User) Phone() string {
	return u.phone
}

func (u User) FullName() string {
	return u.fullName
}

func (u User) MarshalJSON() (data []byte, err error) {
	var buf bytes.Buffer
	_, err = buf.WriteString("{\"ID\":")
	if err != nil {
		return
	}
	_, err = buf.WriteString(u.id.String())
	if err != nil {
		return
	}
	_, err = buf.WriteString(", \"Phone\": \"")
	if err != nil {
		return
	}
	_, err = buf.WriteString(u.phone)
	if err != nil {
		return
	}
	_, err = buf.WriteString("\", \"FullName\": \"")
	if err != nil {
		return
	}
	_, err = buf.WriteString(u.fullName)
	if err != nil {
		return
	}
	_, err = buf.WriteString("\", \"FirstName\": \"")
	if err != nil {
		return
	}
	_, err = buf.WriteString(u.firstName)
	if err != nil {
		return
	}
	_, err = buf.WriteString("\", \"LastName\": \"")
	if err != nil {
		return
	}
	_, err = buf.WriteString(u.lastName)
	if err != nil {
		return
	}
	_, err = buf.WriteString("\"}")
	if err != nil {
		return
	}

	return buf.Bytes(), nil
}

func (u User) validate() (err error) {
	err = checkCredentials(u.phone, u.secret)
	if err != nil {
		return
	}
	if u.firstName == "" {
		err = ErrInvalidRegData
	}
	return
}

type Repository struct {
	*database.Repository
}

func (r *Repository) create(ctx context.Context, user *User, tx *sql.Tx) (err error) {
	return tx.QueryRowContext(ctx, `
	INSERT INTO users (phone, first_name, last_name, full_name, search_name)
	VALUES ($1, $2, $3, $4, $5)
	RETURNING id`,
		user.phone, user.firstName, user.lastName,
		user.fullName, user.searchName).Scan(&user.id)
}

func (r *Repository) createNX(ctx context.Context, user *User, tx *sql.Tx) (err error) {
	_, ok, err := r.exists(ctx, user.phone, tx)
	if err != nil {
		return
	} else if ok {
		err = ErrExists
		return
	}
	return r.create(ctx, user, tx)
}

func (r *Repository) get(ctx context.Context, userID UserID) (user User, err error) {
	err = r.QueryRowContext(ctx, `
	SELECT id, phone, first_name, last_name, full_name
	FROM users
	WHERE id = $1`, userID).Scan(&user.id, &user.phone, &user.firstName, &user.lastName, &user.fullName)
	return
}

func (r *Repository) exists(ctx context.Context, phone string, tx *sql.Tx) (userID UserID, exists bool, err error) {
	funcQueryContext := r.QueryContext
	if tx != nil {
		funcQueryContext = tx.QueryContext
	}
	rows, err := funcQueryContext(ctx, `
	SELECT id
	FROM users
	WHERE phone = $1`, phone)
	if err != nil {
		return
	}
	defer func() {
		errClose := rows.Close()
		if err == nil {
			err = errClose
		}
	}()

	if rows.Next() {
		err = rows.Scan(&userID)
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

func (r *Repository) write(ctx context.Context, user *User,
	handler func(ctx context.Context, user *User, tx *sql.Tx) error) (err error) {
	err = user.validate()
	if err != nil {
		return
	}
	tx, err := r.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	defer func() { err = r.RollbackTx(tx, err) }()
retry:
	err = handler(ctx, user, tx)
	if r.SerializationFailureError(err) {
		//TODO: circuit breaker
		goto retry
	}
	if err != nil {
		return
	}
	return tx.Commit()
}

type Manager struct {
	repository     *Repository
	secretsStorage *secretsmng.Client
}

type ManagerParams struct {
	fx.In
	Repository     *database.Repository
	SecretsStorage *secretsmng.Client
}

func NewManager(p ManagerParams) *Manager {
	return &Manager{
		repository:     &Repository{p.Repository},
		secretsStorage: p.SecretsStorage,
	}
}

type RegData struct {
	Phone     string
	FirstName string
	LastName  string
	secretsmng.Password
}

func (m *Manager) Authorize(ctx context.Context, regData RegData) (userID UserID, err error) {
	err = checkCredentials(regData.Phone, regData.Password)
	if err != nil {
		return
	}

	userID, ok, err := m.repository.exists(ctx, regData.Phone, nil)
	if err != nil {
		return
	} else if !ok {
		err = ErrWrongCredentials
		return
	}
	ok, err = m.secretsStorage.ValidateSecret(ctx, regData.Phone, regData.Password)
	if err != nil {
		return
	} else if !ok {
		err = ErrWrongCredentials
	}
	return
}

func (m *Manager) Register(ctx context.Context, regData RegData) (userID UserID, err error) {
	user, err := NewUser(regData)
	if err != nil {
		return
	}
	err = m.createNX(ctx, &user)
	return user.id, err
}

func (m *Manager) Get(ctx context.Context, userID UserID) (user User, err error) {
	return m.repository.get(ctx, userID)
}

func (m *Manager) createNX(ctx context.Context, user *User) (err error) {
	err = m.repository.write(ctx, user, m.repository.createNX)
	if err != nil {
		return
	}
	//TODO: error if, e.g. net will blink || err if unable to write (have data in DB and haven't data in vault)
	return m.secretsStorage.WriteSecret(ctx, user.phone, user.secret)
}

func StringToID(sub string) (UserID, error) {
	id, err := strconv.ParseUint(sub, 10, 64)
	if err != nil {
		return 0, err
	}
	return UserID(id), err
}

type IsEmptyer interface {
	IsEmpty() bool
}

func checkCredentials(phone string, password IsEmptyer) error {
	if phone == "" || password.IsEmpty() {
		return ErrInvalidCredentials
	}
	matched, err := regexp.MatchString(`^\+(?:[0-9] ?){6,14}[0-9]$`, phone)
	if err != nil {
		return err
	}
	if !matched {
		return ErrInvalidPhoneFormat
	}
	return nil
}
