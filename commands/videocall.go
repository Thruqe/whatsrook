package commands

func init() {
	Register(&Command{
		Name:         "videocall",
		Aliases:      []string{"vc", "vcall"},
		Description:  "Place a video call to a target number",
		Category:     "calls",
		HideFromMenu: false,
		IsPublic:     true,
		Handler:      handleVideoCall,
	})
}

func handleVideoCall(ctx *Context) error {
	targets := ctx.GetTargets()
	if len(targets) < 1 {
		return sendText(ctx, "usage: !videocall <number>")
	}
	target := targets[0].String()
	return placeVideoCall(ctx, target)
}
