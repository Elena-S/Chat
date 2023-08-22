package chats

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Elena-S/Chat/pkg/broker"
	"github.com/Elena-S/Chat/pkg/database"
	"github.com/Elena-S/Chat/pkg/users"
	"golang.org/x/net/websocket"
)

type MessageType uint

type Message struct {
	ChatID   uint
	ID       uint
	Author   string
	AuthorID uint
	Text     string
	Date     time.Time
	Type     MessageType
}

const (
	MessageTypeOrdinary MessageType = iota + 1
	MessageTypeTyping
)

func (message *Message) Register(senderID uint) (err error) {
	if message.ID != 0 {
		err = errors.New("chats: got a non zero message identifier")
		return
	}
	if message.ChatID == 0 {
		err = errors.New("chats: got a zero chat identifier")
		return
	}
	if message.Text == "" {
		err = errors.New("chats: got an empty message")
		return
	}

	sender, err := users.GetUserByID(senderID)
	if err != nil {
		err = fmt.Errorf("chats: a sender with the given id %d does not exist", senderID)
		return
	}
	message.AuthorID = senderID
	message.Author = sender.FullName()
	message.Date = time.Now()

	if message.Type != MessageTypeOrdinary {
		return
	}

	tx, err := database.DB().Begin()
	if err != nil {
		return
	}
	defer func() { err = database.Rollback(tx, err) }()

	err = message.create(tx)
	if err != nil {
		return
	}

	return tx.Commit()
}

func (message *Message) Chat() (chat *Chat) {
	chat = new(Chat)
	chat.ID = message.ChatID
	return
}

func (message *Message) Share(recievers []uint) error {
	ctx := context.TODO()
	errs := make([]error, len(recievers))
	i := 0
	for _, reciever := range recievers {
		messageJSON, err := json.Marshal(*message)
		if err != nil {
			errs[i] = err
			i++
			continue
		}
		err = broker.Publish(ctx, users.IDToString(reciever), messageJSON)
		if err != nil {
			errs[i] = err
			i++
			continue
		}
	}
	return errors.Join(errs[:i]...)
}

func (message *Message) create(tx *sql.Tx) (err error) {
	return tx.QueryRow(`
	INSERT INTO chat_messages (chat_id, author_id, date, text) 
	VALUES ($1, $2, $3, $4)
	RETURNING id`, message.ChatID, message.AuthorID, message.Date, message.Text).Scan(&message.ID)
}

func SendMessage(xmessage []byte, ws *websocket.Conn) (err error) {
	message := new(Message)
	if err = json.Unmarshal(xmessage, message); err != nil {
		return err
	}
	if message.Type == MessageTypeTyping && message.Date.Add(time.Second*2).UnixMilli() < time.Now().UnixMilli() {
		return
	}
	return websocket.JSON.Send(ws, message)
}

type History struct {
	LastID   uint
	Messages []Message
}

func (history *History) Fill(userID uint, chatID uint, messageID uint) (err error) {
	if chatID == 0 {
		err = errors.New("chats: got zero chat id")
		return
	}

	rows, err := database.DB().Query(`
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
	defer rows.Close()

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
