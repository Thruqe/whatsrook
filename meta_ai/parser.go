package meta_ai

import (
	"strings"

	stripmd "github.com/writeas/go-strip-markdown/v2"
)

// AnswerParserString converts an AI-generated response written in Markdown
// into plain, unformatted text.
//
// It strips common Markdown syntax — headers, emphasis (bold/italic),
// strikethrough, inline and fenced code blocks, links (keeping the link
// text, dropping the URL), images (keeping alt text), blockquotes, and
// list markers — leaving only the underlying human-readable text.
//
// The input pointer is mutated in place: *ai_response_string is replaced
// with its plain-text form.
func AnswerParserString(ai_response_string *string) {
	if ai_response_string == nil {
		return
	}

	plain := stripmd.Strip(*ai_response_string)
	plain = strings.TrimSpace(plain)

	*ai_response_string = plain
}

