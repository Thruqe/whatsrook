package commands

import (
	"go.mau.fi/whatsmeow/proto/waAICommon"
	"go.mau.fi/whatsmeow/proto/waAICommonDeprecated"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func init() {
	Register(&Command{
		Name:        "botai",
		Description: "Send an AI-rich response forwarded message",
		Category:    "example",
		IsPublic:    true,
		Handler:     handleBotAI,
	})
}

func handleBotAI(ctx *Context) error {
	unifiedJSON := `{
		"response_id": "test",
		"sections": [
			{
				"view_model": {
					"primitive": {
						"text": "Image Example",
						"__typename": "GenAIMetadataTextPrimitive"
					},
					"__typename": "GenAISingleLayoutViewModel"
				}
			},
			{
				"view_model": {
					"primitive": {
						"media": {
							"url": "https://i.pinimg.com/originals/23/9e/69/239e69f8446499b2a903a1d9df4bfdb1.jpg",
							"mime_type": "image/jpeg"
						},
						"imagine_type": "IMAGE",
						"status": {
							"status": "READY"
						},
						"__typename": "GenAIImaginePrimitive"
					},
					"__typename": "GenAISingleLayoutViewModel"
				}
			}
		]
	}`

	msg := &waE2E.Message{
		BotForwardedMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				RichResponseMessage: &waE2E.AIRichResponseMessage{
					MessageType: waAICommonDeprecated.AIRichResponseMessageType_AI_RICH_RESPONSE_TYPE_STANDARD.Enum(),
					Submessages: []*waAICommonDeprecated.AIRichResponseSubMessage{
						{
							MessageType: waAICommonDeprecated.AIRichResponseSubMessageType_AI_RICH_RESPONSE_TEXT.Enum(),
							MessageText: proto.String("Hello World"),
						},
					},
					UnifiedResponse: &waAICommon.AIRichResponseUnifiedResponse{
						Data: []byte(unifiedJSON),
					},
					ContextInfo: &waE2E.ContextInfo{
						ForwardingScore: proto.Uint32(1),
						IsForwarded:     proto.Bool(true),
						ForwardedAiBotMessageInfo: &waAICommon.ForwardedAIBotMessageInfo{
							BotJID: proto.String("0@bot"),
						},
						ForwardOrigin: waE2E.ContextInfo_META_AI.Enum(),
					},
				},
			},
		},
	}

	_, err := ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg)
	return err
}
