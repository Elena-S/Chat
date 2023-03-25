package chats

import (
	"fmt"

	"github.com/Elena-S/Chat/pkg/conns"
	"github.com/Elena-S/Chat/pkg/database"
)

type QueryChat struct {
	CommonText string
	Params     []any
}

func (q *QueryChat) fullText() string {

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
		last_message_date DESC, name`, q.CommonText)
}

func (q *QueryChat) result() (chatArr []Chat, err error) {
	rows, err := database.DB().Query(q.fullText(), q.Params...)

	if err != nil {
		return
	}

	defer rows.Close()

	for rows.Next() {
		var userID uint
		chat := Chat{Status: -1}
		err = rows.Scan(&chat.ID, &chat.Type, &chat.Name, &chat.Presentation,
			&chat.Phone, &userID, database.Array(&chat.Contacts), &chat.LastMessageDate, &chat.LastMessageText)
		if err != nil {
			return
		}
		if chat.Type == ChatTypePrivate {
			if chat.ID == 0 && userID != 0 {
				chat.Contacts = append(chat.Contacts, userID)
			}
			chat.Status = conns.Pool.UserStatus(userID)
		}
		chatArr = append(chatArr, chat)
	}

	return
}
