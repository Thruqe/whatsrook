package commands

func init() {
	Register(&Command{
		Name:         "videocall",
		Description:  "Video call a number, playing your saved (or next-provided) video",
		Category:     "calls",
		HideFromMenu: true, // stub — outbound video is not yet supported
		IsPublic:     true,
		Handler:      handleVideoCall,
	})
}

func handleVideoCall(ctx *Context) error {
	return sendText(ctx, "🎬 Video calling isn't fully supported yet — outbound video is unvalidated in the underlying call library.\n"+
		"Tracking upstream: https://github.com/purpshell/meowcaller")
}
