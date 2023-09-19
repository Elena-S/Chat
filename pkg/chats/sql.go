package chats

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"strings"

	"github.com/Elena-S/Chat/pkg/database"
	"github.com/Elena-S/Chat/pkg/users"
)

var ErrExists = errors.New("chats: the same chat already exists")

type Repository struct {
	*database.Repository
}

func (r *Repository) create(ctx context.Context, tx *sql.Tx, chat *Chat) (err error) {
	err = tx.QueryRowContext(ctx, `
	INSERT INTO chats (type_id, name)
	VALUES ($1, $2)
	RETURNING id`, chat.Type, chat.Name).Scan(&chat.ID)
	if err != nil {
		return
	}
	err = r.addContacts(ctx, tx, *chat)
	if err != nil {
		return
	}
	return
}

func (r *Repository) addContacts(ctx context.Context, tx *sql.Tx, chat Chat) (err error) {
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
	_, err = tx.ExecContext(ctx, buf.String(), params...)
	return
}

func (r *Repository) exists(ctx context.Context, tx *sql.Tx, chat Chat) (exists bool, err error) {
	if chat.Type != ChatTypePrivate {
		return
	}
	if len(chat.Contacts) != MinContactsCount {
		err = ErrWrongContactsNumber
		return
	}
	funcQueryContext := r.QueryContext
	if tx != nil {
		funcQueryContext = tx.QueryContext
	}
	rows, err := funcQueryContext(ctx, `
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
	defer func() {
		errClose := rows.Close()
		if err == nil {
			err = errClose
		}
	}()

	exists = rows.Next()

	return
}

func (r *Repository) createNX(ctx context.Context, tx *sql.Tx, chat *Chat) (err error) {
	ok, err := r.exists(ctx, tx, *chat)
	if err != nil {
		err = fmt.Errorf("chats: chat validation error: %w", err)
		return
	} else if ok {
		err = fmt.Errorf("chats: chat validation error: %w", ErrExists)
		return
	}
	return r.create(ctx, tx, chat)
}

func (r *Repository) write(ctx context.Context, chat *Chat,
	handler func(ctx context.Context, tx *sql.Tx, chat *Chat) error) (err error) {
	err = chat.validate()
	if err != nil {
		return
	}
	tx, err := r.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	defer func() { err = r.RollbackTx(tx, err) }()
retry:
	err = handler(ctx, tx, chat)
	if r.SerializationFailureError(err) {
		//TODO: circuit breaker
		goto retry
	}
	if err != nil {
		return
	}
	return tx.Commit()
}

func (r *Repository) get(ctx context.Context, chatID ChatID, userID users.UserID) (chat Chat, err error) {
	q := queryChat{}
	q.Repository = r
	q.Context = ctx
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
	q.Params = []any{chatID, userID, ChatTypePrivate}

	chats, err := q.Result()
	if err != nil {
		return
	}

	if len(chats) == 0 {
		err = fmt.Errorf("chats: a chat with an ID %d and a user ID %d does not exist", chatID, userID)
		return
	}

	return chats[0], nil
}

func (r *Repository) list(ctx context.Context, userID users.UserID) (chatArr []Chat, err error) {
	q := queryChat{}
	q.Context = ctx
	q.Repository = r
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

	return q.Result()
}

func (r *Repository) search(ctx context.Context, userID users.UserID, phrase string) (chatArr []Chat, err error) {
	phrase = strings.ToLower(strings.TrimSpace(phrase))
	if phrase == "" {
		return
	}

	q := queryChat{}
	q.Context = ctx
	q.Repository = r
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

	return q.Result()
}

func (r *Repository) contacts(ctx context.Context, chatID ChatID) ([]users.UserID, error) {
	arr := make([]uint, MinContactsCount)
	err := r.QueryRowContext(ctx, `
	SELECT
		ARRAY_AGG(user_id) contacts
	FROM
		chat_contacts
	WHERE
		chat_id = $1`, chatID).Scan(database.Array(&arr))
	if err != nil {
		return nil, err
	}
	contacts := make([]users.UserID, len(arr))
	for i, id := range arr {
		contacts[i] = users.UserID(id)
	}
	return contacts, nil
}

type queryChat struct {
	Repository *Repository
	Context    context.Context
	Text       string
	Params     []any
}

func (q queryChat) fullText() string {
	return fmt.Sprintf(`
	WITH common_chat_info AS (%s),
	last_message_info AS (SELECT
		chats_last_message.id,
		chat_messages.date,
		chat_messages.text
	FROM
	(SELECT
		common_chat_info.id,
		MAX(chat_messages.id) AS last_message_id
	FROM common_chat_info
		JOIN chat_messages
		ON common_chat_info.id = chat_messages.chat_id
	GROUP BY
		common_chat_info.id) AS chats_last_message
		JOIN chat_messages
		ON chats_last_message.last_message_id = chat_messages.id),
	contacts_info AS (SELECT
		common_chat_info.id,
		ARRAY_AGG(chat_contacts.user_id) contacts
	FROM
		common_chat_info
		JOIN chat_contacts
		ON common_chat_info.id = chat_contacts.chat_id
	GROUP BY
		common_chat_info.id
	)	
	SELECT
		common_chat_info.*,
		COALESCE(contacts_info.contacts::bigint[], '{}'::bigint[]) contacts,
		COALESCE(last_message_info.date, DATE '0001-01-01') last_message_date,
		COALESCE(last_message_info.text, '') last_message_text
	FROM
		common_chat_info
		LEFT JOIN last_message_info
		ON common_chat_info.id = last_message_info.id
		LEFT JOIN contacts_info
		ON common_chat_info.id = contacts_info.id
	ORDER BY
		last_message_date DESC, name`, q.Text)
}

func (q queryChat) Result() (chatArr []Chat, err error) {
	rows, err := q.Repository.QueryContext(q.Context, q.fullText(), q.Params...)

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
		var userID users.UserID
		arr := make([]uint, MinContactsCount)
		chat := Chat{}
		err = rows.Scan(&chat.ID, &chat.Type, &chat.Name, &chat.Presentation,
			&chat.Phone, &userID, database.Array(&arr), &chat.LastMessageDate, &chat.LastMessageText)
		if err != nil {
			return
		}
		chat.Contacts = make([]users.UserID, len(arr))
		for i, id := range arr {
			chat.Contacts[i] = users.UserID(id)
		}
		if chat.Type == ChatTypePrivate && chat.ID == 0 && userID != 0 {
			chat.Contacts = append(chat.Contacts, userID)
		}
		chatArr = append(chatArr, chat)
	}

	return
}
