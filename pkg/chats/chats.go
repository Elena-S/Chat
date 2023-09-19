package chats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/Elena-S/Chat/pkg/database"
	"github.com/Elena-S/Chat/pkg/users"
	"go.uber.org/fx"
	"golang.org/x/exp/slices"
)

var (
	ErrWrongContactsNumber = fmt.Errorf("chats: not enought contacts. Min contacts number is %d", MinContactsCount)
	ErrNoName              = errors.New("chats: group chat must have a name")
)

const MinContactsCount = 2

type ChatTypeID uint

const (
	ChatTypePrivate ChatTypeID = iota + 1
	ChatTypeGroup
)

type ChatID uint

func (t ChatID) String() string {
	return strconv.FormatUint(uint64(t), 10)
}

type RegData struct {
	Type     ChatTypeID
	Name     string
	Contacts []users.UserID
}

type Chat struct {
	ID       ChatID
	Type     ChatTypeID
	Name     string
	Contacts []users.UserID

	Presentation    string
	Phone           string
	LastMessageText string
	LastMessageDate time.Time
}

func NewChat(regData RegData, ownerID users.UserID) (chat Chat) {
	chat.Type = regData.Type
	chat.Name = regData.Name
	chat.Contacts = regData.Contacts
	if slices.Index(chat.Contacts, ownerID) < 0 {
		chat.Contacts = append(chat.Contacts, ownerID)
	}
	return
}

func (c Chat) validate() (err error) {
	if len(c.Contacts) < MinContactsCount {
		err = ErrWrongContactsNumber
		return
	}
	if c.Type == ChatTypeGroup && c.Name == "" {
		err = ErrNoName
		return
	}
	return
}

type Manager struct {
	repository *Repository
}

type ManagerParams struct {
	fx.In
	Repository *database.Repository
}

func NewManager(p ManagerParams) *Manager {
	return &Manager{
		repository: &Repository{p.Repository},
	}
}

func (m *Manager) Register(ctx context.Context, r io.Reader, ownerID users.UserID) (chat Chat, err error) {
	regData := RegData{}
	err = json.NewDecoder(r).Decode(&regData)
	if err != nil {
		return
	}

	chat = NewChat(regData, ownerID)

	err = m.createNX(ctx, &chat)
	if err != nil {
		return
	}
	return
}

func (m *Manager) createNX(ctx context.Context, chat *Chat) (err error) {
	return m.repository.write(ctx, chat, m.repository.createNX)
}

func (m *Manager) Search(ctx context.Context, userID users.UserID, phrase string) (chatArr []Chat, err error) {
	return m.repository.search(ctx, userID, phrase)
}

func (m *Manager) List(ctx context.Context, userID users.UserID) (chatArr []Chat, err error) {
	return m.repository.list(ctx, userID)
}

func (m *Manager) Get(ctx context.Context, chatID ChatID, userID users.UserID) (chat Chat, err error) {
	return m.repository.get(ctx, chatID, userID)
}

func (m *Manager) Contacts(ctx context.Context, chatID ChatID) ([]users.UserID, error) {
	return m.repository.contacts(ctx, chatID)
}

func StringToID(chat string) (ChatID, error) {
	id, err := strconv.ParseUint(chat, 10, 64)
	if err != nil {
		return 0, err
	}
	return ChatID(id), err
}
