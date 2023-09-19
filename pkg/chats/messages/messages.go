package messages

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/Elena-S/Chat/pkg/chats"
	"github.com/Elena-S/Chat/pkg/database"
	"github.com/Elena-S/Chat/pkg/users"
	"go.uber.org/fx"
)

var (
	ErrUnregisterable = errors.New("messages: the message is not registerable")
	ErrEmpty          = errors.New("messages: got an empty message")
	ErrZeroChatID     = errors.New("messages: got a zero chat identifier")
	ErrZeroID         = errors.New("messages: got a non zero message identifier")
)

type MessageType uint

const (
	MessageTypeOrdinary MessageType = iota + 1
	MessageTypeTyping
)

type MessageID uint

func (t MessageID) String() string {
	return strconv.FormatUint(uint64(t), 10)
}

type Message struct {
	ChatID    chats.ChatID
	ID        MessageID
	Author    string
	AuthorID  users.UserID
	Text      string
	Date      time.Time
	Type      MessageType
	receivers []users.UserID
}

func NewMessage(data []byte) (message Message, err error) {
	err = json.Unmarshal(data, &message)
	return
}

func (m Message) Receivers() []users.UserID {
	return m.receivers
}

func (m Message) IsActual() (ok bool) {
	return m.Type != MessageTypeTyping || time.Since(m.Date) < time.Second*2
}

func (m Message) validate() (err error) {
	if m.Text == "" {
		err = fmt.Errorf("messages: message validation error: %w", ErrEmpty)
		return
	}
	if m.Type != MessageTypeOrdinary {
		return ErrUnregisterable
	}
	if m.ChatID == 0 {
		err = fmt.Errorf("messages: message validation error: %w", ErrZeroChatID)
		return
	}
	if len(m.receivers) < chats.MinContactsCount {
		err = fmt.Errorf("messages: message validation error: %w", chats.ErrWrongContactsNumber)
		return
	}
	return
}

type History struct {
	LastID   MessageID
	Messages []Message
}

type Repository struct {
	//TODO: mb Cassandra
	*database.Repository
}

func (r *Repository) create(ctx context.Context, message *Message, tx *sql.Tx) (err error) {
	return tx.QueryRowContext(ctx, `
	INSERT INTO chat_messages (chat_id, author_id, date, text) 
	VALUES ($1, $2, $3, $4)
	RETURNING id`, message.ChatID, message.AuthorID, message.Date, message.Text).Scan(&message.ID)
}

func (r *Repository) list(ctx context.Context, chatID chats.ChatID, userID users.UserID, messageID MessageID) (history History, err error) {
	if chatID == 0 {
		err = ErrZeroChatID
		return
	}

	rows, err := r.QueryContext(ctx, `
	SELECT
		chat_messages.id,
		users.full_name author,
		chat_messages.author_id author_id,
		chat_messages.text text,
		chat_messages.date
	FROM
		chats
		JOIN chat_contacts
		ON chats.id = $1
			AND chats.id = chat_contacts.chat_id
			AND chat_contacts.user_id = $2
		JOIN chat_messages
		ON chats.id = chat_messages.chat_id
			AND (chat_messages.id < $3 OR $3 = 0)
		JOIN users
		ON chat_messages.author_id = users.id
	ORDER BY
		chat_messages.id DESC
	LIMIT 100`, chatID, userID, messageID)
	if err != nil {
		return
	}
	defer func() {
		errClose := rows.Close()
		if err == nil {
			err = errClose
		}
	}()

	for rows.Next() {
		message := Message{ChatID: chatID}
		err = rows.Scan(&message.ID, &message.Author, &message.AuthorID, &message.Text, &message.Date)
		if err != nil {
			return
		}
		history.Messages = append(history.Messages, message)
		history.LastID = message.ID
	}

	return
}

func (r *Repository) write(ctx context.Context, message *Message,
	handler func(ctx context.Context, message *Message, tx *sql.Tx) error) (err error) {
	err = message.validate()
	if err != nil {
		return
	}
	tx, err := r.Begin()
	if err != nil {
		return
	}
	defer func() { err = r.RollbackTx(tx, err) }()

	err = handler(ctx, message, tx)
	if err != nil {
		return
	}
	return tx.Commit()
}

type Manager struct {
	chatManager *chats.Manager
	repository  *Repository
}

type ManagerParams struct {
	fx.In
	ChatManager *chats.Manager
	Repository  *database.Repository
}

func NewManager(p ManagerParams) *Manager {
	return &Manager{
		chatManager: p.ChatManager,
		repository:  &Repository{p.Repository},
	}
}

func (m *Manager) Register(ctx context.Context, data []byte, sender users.User) (message Message, err error) {
	message, err = NewMessage(data)
	if err != nil {
		return
	}

	if message.ID != 0 {
		err = ErrZeroID
		return
	}

	//TODO: need cache
	message.receivers, err = m.chatManager.Contacts(ctx, message.ChatID)
	if err != nil {
		return
	}
	message.AuthorID = sender.ID()
	message.Author = sender.FullName()
	message.Date = time.Now()

	err = m.repository.write(ctx, &message, m.repository.create)
	if err == ErrUnregisterable {
		err = nil
	}
	return
}

func (m *Manager) List(ctx context.Context, chatID chats.ChatID, userID users.UserID, messageID MessageID) (history History, err error) {
	return m.repository.list(ctx, chatID, userID, messageID)
}

func (m *Manager) IsActual(data []byte) (ok bool, err error) {
	message, err := NewMessage(data)
	if err != nil {
		return
	}
	return message.IsActual(), nil
}

func StringToID(messageID string) (MessageID, error) {
	id, err := strconv.ParseUint(messageID, 10, 64)
	if err != nil {
		return 0, err
	}
	return MessageID(id), err
}
