package chats

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"strings"
	"time"

	"github.com/Elena-S/Chat/pkg/database"
	"golang.org/x/exp/slices"
)

const minContactsCount = 2

type ChatTypeID uint

const (
	ChatTypePrivate ChatTypeID = iota + 1
	ChatTypeGroup
)

type Chat struct {
	ID       uint
	Type     ChatTypeID
	Name     string
	Contacts []uint

	Presentation    string
	Status          int8
	Phone           string
	LastMessageText string
	LastMessageDate time.Time
}

func (chat *Chat) Register(ctx context.Context, r io.Reader, ownerID uint) (err error) {
	err = json.NewDecoder(r).Decode(chat)
	if err != nil {
		return
	}

	if slices.Index(chat.Contacts, ownerID) < 0 {
		chat.Contacts = append(chat.Contacts, ownerID)
	}

	if chat.ID != 0 {
		err = errors.New("chats: got non zero chat identifier")
		return
	}

	if len(chat.Contacts) < minContactsCount {
		err = errors.New("chats: not enought contacts to create chat")
		return
	}

retry:
	err = chat.createNX(ctx)
	if database.SerializationFailureError(err) {
		goto retry
	}
	return
}

func (chat *Chat) SendMessage(message Message) (err error) {
	if err = chat.refreshContacts(); err != nil {
		return
	}
	if len(chat.Contacts) < minContactsCount {
		return fmt.Errorf("chats: number of receivers should be at least %d", minContactsCount)
	}
	err = message.Share(chat.Contacts)
	if err != nil {
		return fmt.Errorf("chats: one or more errors occurred when sending a message, %w", err)
	}
	return
}

func (chat *Chat) createNX(ctx context.Context) (err error) {
	tx, err := database.DB().BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return
	}
	defer func() { err = database.Rollback(tx, err) }()

	ok, err := chat.exists(tx)
	if err != nil {
		return
	} else if ok {
		return errors.New("chats: the same chat already exists")
	}

	err = chat.create(tx)
	if err != nil {
		return
	}
	err = chat.addContacts(tx)
	if err != nil {
		return
	}

	return tx.Commit()
}

func (chat *Chat) exists(tx *sql.Tx) (exists bool, err error) {
	if chat.Type != ChatTypePrivate {
		return
	}
	if len(chat.Contacts) > minContactsCount {
		err = errors.New("chats: contacts count doesn't match for private chat")
		return
	}
	rows, err := tx.Query(`
	SELECT 1
	FROM
		(SELECT chat_id
			FROM chat_contacts
			WHERE user_id = $1 INTERSECT
				SELECT chat_id
				FROM chat_contacts WHERE user_id = $2) AS T
	JOIN chats ON T.chat_id = chats.id
	AND chats.type_id = $3`, chat.Contacts[0], chat.Contacts[1], chat.Type)
	if err != nil {
		return
	}
	defer rows.Close()

	exists = rows.Next()

	return
}

func (chat *Chat) create(tx *sql.Tx) (err error) {
	err = tx.QueryRow(`
	INSERT INTO chats (type_id, name)
	VALUES ($1, $2)
	RETURNING id`, chat.Type, chat.Name).Scan(&chat.ID)
	return
}

func (chat *Chat) addContacts(tx *sql.Tx) (err error) {
	var buf strings.Builder
	queryTemp, err := template.New("Query").Funcs(template.FuncMap{"add": func(a, b int) int { return a + b }}).Parse(`
	INSERT INTO chat_contacts (chat_id, user_id) VALUES{{$li := add (len .) -1}}{{range $index, $item := .}}
	($1, ${{add $index 2}}){{if lt $index $li}},{{end}}{{end}}`)
	if err != nil {
		return
	}
	if err != nil {
		return
	}
	err = queryTemp.Execute(&buf, chat.Contacts)
	if err != nil {
		return
	}
	params := make([]any, 1+len(chat.Contacts))
	params[0] = chat.ID
	for i, userID := range chat.Contacts {
		params[i+1] = userID
	}
	_, err = tx.Exec(buf.String(), params...)
	return
}

func (chat *Chat) refreshContacts() (err error) {
	return database.DB().QueryRow(`
	SELECT
		ARRAY_AGG(user_id) contacts
	FROM
		chat_contacts
	WHERE
		chat_id = $1`, chat.ID).Scan(database.Array(&chat.Contacts))
}

func Search(userID uint, phrase string) (chatArr []Chat, err error) {
	phrase = strings.ToLower(strings.TrimSpace(phrase))
	if phrase == "" {
		return
	}

	q := QueryChat{}
	q.Text = `
	SELECT
		COALESCE(chats.chat_id, 0) id,
		$3 type,
		'' name,
		users.full_name presentation,
		users.phone,
		users.id user_id
	FROM 
		users 
		LEFT JOIN 
			(SELECT 
				chat_contacts.user_id,
				chat_contacts.chat_id	   
			FROM
				chat_contacts AS chat_contact
				JOIN chats
				ON chat_contact.user_id = $2
					AND chat_contact.chat_id = chats.id
					AND chats.type_id = $3
				JOIN chat_contacts
				ON chats.id = chat_contacts.chat_id) AS chats
		ON users.id = chats.user_id
	WHERE 
		users.search_name LIKE $1 AND users.id <> $2

	UNION ALL

	SELECT
		id,
		type_id,
		name,
		name,
		'',
		0
	FROM
		chats 
	WHERE 
		search_name LIKE $1 AND type_id <> $3
	LIMIT 25`
	q.Params = []any{fmt.Sprint("%", phrase, "%"), userID, ChatTypePrivate}

	return q.result()
}

func List(userID uint) (chatArr []Chat, err error) {
	q := QueryChat{}
	q.Text = `
	SELECT
		chats.id id,
		chats.type_id type,
		chats.name name,
		COALESCE(users.full_name, chats.name) presentation,
		COALESCE(users.phone, '') phone,
		COALESCE(users.id, 0) user_id
	FROM
		chat_contacts
		JOIN chats
		ON chat_contacts.user_id = $1
			AND chat_contacts.chat_id = chats.id
		LEFT JOIN chat_contacts chat_contact
		ON chats.type_id = $2
			AND chats.id = chat_contact.chat_id
			AND chat_contacts.user_id <> chat_contact.user_id
		LEFT JOIN users
		ON chat_contact.user_id = users.id`
	q.Params = []any{userID, ChatTypePrivate}

	return q.result()
}

func GetChatInfoByID(ID uint, userID uint) (chat Chat, err error) {
	q := QueryChat{}
	q.Text = `
	SELECT 
		chats.id id,
		chats.type_id type,
		chats.name name,
		COALESCE(users.full_name, chats.name) presentation,
		COALESCE(users.phone, '') phone,
		COALESCE(users.id, 0) user_id
	FROM 
		chats AS chats
			JOIN chat_contacts AS chat_contacts
			ON chats.id = $1
			AND chats.id = chat_contacts.chat_id
			AND chat_contacts.user_id = $2
			LEFT JOIN chat_contacts AS chat_contact
			ON chats.id = chat_contact.chat_id
			AND chat_contact.user_id != $2
			AND chats.type_id = $3
			LEFT JOIN users AS users
			ON chat_contact.user_id = users.id`
	q.Params = []any{ID, userID, ChatTypePrivate}

	chats, err := q.result()
	if err != nil {
		return
	}

	if len(chats) == 0 {
		err = fmt.Errorf("chats: a chat with an ID %d and a user ID %d does not exist", ID, userID)
		return
	}

	return chats[0], nil
}
