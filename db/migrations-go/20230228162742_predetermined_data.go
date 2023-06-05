package migrations

import (
	"bytes"
	"database/sql"
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
	MERGE INTO chat_types
	USING
		(SELECT 
			id,
			MAX(CASE
					WHEN exists = -1
						THEN name
					ELSE NULL
				END) name,
			SUM(exists) exists
		FROM
			(SELECT
				id,
				name,
				1 exists
			FROM chat_types
			{{range .}}
			UNION ALL
			
			SELECT
				{{.ID}},
				'{{.Name}}',
				-1{{end}}) T
			GROUP BY ID
			HAVING
				NOT(MAX(exists) = 0 AND MIN(name) = MAX(name))) AS data
	ON chat_types.id = data.id
	WHEN MATCHED AND data.exists = 1 THEN
	DELETE
	WHEN MATCHED AND data.exists = 0 THEN
	UPDATE SET name = data.name
	WHEN NOT MATCHED THEN
	INSERT (id, name)
	VALUES(data.id, data.name)`)

	if err != nil {
		return
	}

	var buf bytes.Buffer
	err = templ.Execute(&buf, PredeterminedChatTypes)
	if err != nil {
		return
	}
	_, err = tx.Exec(buf.String())

	return
}

func trancateChatTypes(tx *sql.Tx) (err error) {
	_, err = tx.Exec(`TRUNCATE TABLE public.chat_types RESTART IDENTITY CASCADE`)
	return
}
