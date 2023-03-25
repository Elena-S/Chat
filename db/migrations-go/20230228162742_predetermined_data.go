package migrations

import (
	"bytes"
	"database/sql"
	"fmt"
	"html/template"

	"github.com/Elena-S/Chat/pkg/chats"
	"github.com/pressly/goose/v3"
)

var PredeterminedChatTypes = map[chats.ChatTypeID](struct {
	ID   chats.ChatTypeID
	Name string
}){
	chats.ChatTypePrivate: {chats.ChatTypePrivate, "Private"},
	chats.ChatTypeGroup:   {chats.ChatTypeGroup, "Group"},
}

func init() {
	goose.AddMigration(upPredeterminedData, downPredeterminedData)
}

func upPredeterminedData(tx *sql.Tx) error {
	err := updateChatTypes(tx)
	return err
}

func downPredeterminedData(tx *sql.Tx) error {
	err := trancateChatTypes(tx)
	return err
}

func updateChatTypes(tx *sql.Tx) (err error) {
	templ, err := template.New("query").Parse(`
	SELECT
		id, SUM(exists) exists
	FROM (
		SELECT
			id, name, 1 exists, 1 delete
		FROM chat_types
		{{range .}}
		UNION ALL

		SELECT
			{{.ID}}, '{{.Name}}', 0 exists, 0{{end}}) T
	GROUP BY
		id
	HAVING
		MAX(exists) = 0 OR MIN(delete) = 1 OR NOT MIN(name) = MAX(name)
	`)

	if err != nil {
		return
	}

	var buf bytes.Buffer
	err = templ.Execute(&buf, PredeterminedChatTypes)
	if err != nil {
		return
	}
	query := buf.String()

	rows, err := tx.Query(query)
	if err != nil {
		return
	}

	defer rows.Close()

	buf.Reset()

	res := struct {
		ID     chats.ChatTypeID
		Exists uint8
	}{}
	for rows.Next() {
		err = rows.Scan(&res.ID, &res.Exists)
		if err != nil {
			return
		}

		chatType := PredeterminedChatTypes[res.ID]
		switch res.Exists {
		case 0:
			query = fmt.Sprintf(`
			INSERT INTO public.chat_types (id, name)
			VALUES (%d, '%s');`, chatType.ID, chatType.Name)
			buf.WriteString(query)
		case 1:
			query = fmt.Sprintf(`
			DELETE FROM public.chat_types
			WHERE id = %d;`, chatType.ID)
			buf.WriteString(query)
		case 2:
			query = fmt.Sprintf(`
			UPDATE public.chat_types
			SET (name = '%s')
			WHERE id = %d;`, chatType.Name, chatType.ID)
			buf.WriteString(query)
		}
	}

	_, err = tx.Exec(buf.String())

	return
}

func trancateChatTypes(tx *sql.Tx) (err error) {
	_, err = tx.Exec(`TRUNCATE TABLE public.chat_types RESTART IDENTITY CASCADE`)
	return
}
