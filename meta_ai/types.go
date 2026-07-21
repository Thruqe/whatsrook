package meta_ai

import "go.mau.fi/whatsmeow/types"

type Data struct {
	ChatID                       string          `json:"chat_id"`
	Question                     string          `json:"question"`
	User                         types.JID       `json:"user"`
	QuotedMessageOfQuestion      string          `json:"quoted_message_of_question"`
	UserOfQuotedMessage          string          `json:"user_of_quoted_message"`
	QuotedMessageParticipantRole string          `json:"quoted_message_participant_role"`
	Role                         string          `json:"role"`
	ChatType                     string          `json:"chat_type"`
	IsSudo                       bool            `json:"is_sudo"`
	GroupMetaData                types.GroupInfo `json:"group_meta_data"`
}

type Tools struct {
	Shell   string `json:"shell"`   // Danger
	Command string `json:"command"` // name of a registered bot command to invoke, e.g. "cpu"
}

type Response struct {
	ChatID string `json:"chat_id"`
	Answer string `json:"answer"`
	Tools  Tools  `json:"tools"`
}
