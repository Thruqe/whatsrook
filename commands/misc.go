// Miscellaneous commands – urban dictionary, QR generation, etc.
package commands

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func init() {
	Register(&Command{
		Name:        "save",
		Description: "Forward a replied message to your DM (or save status)",
		Category:    "misc",
		IsPublic:    true,
		Handler:     handleSave,
	})
	Register(&Command{
		Name:        "weather",
		Description: "Check real-time weather forecast for a city or town. Usage: weather [city]",
		Category:    "misc",
		IsPublic:    true,
		Handler:     handleWeather,
	})
}

func handleSave(ctx *Context) error {
	quoted := ctx.GetQuotedMessage()
	if quoted == nil {
		return ctx.Reply("ℹ The basic functionality of this command is to save status updates. Please reply to a status update or any message to forward it to your DM.")
	}

	if ctx.Client.Store.ID == nil {
		return ctx.Reply("Owner ID unavailable.")
	}

	ownerJID := ctx.Client.Store.ID.ToNonAD()
	_, err := ctx.Client.SendMessage(ctx.Ctx, ownerJID, quoted)
	if err != nil {
		return ctx.Reply(fmt.Sprintf(" Failed to forward message: %v", err))
	}

	return ctx.Reply("Message forwarded to your DM.")
}

func handleWeather(ctx *Context) error {
	if len(ctx.Args) == 0 {
		return ctx.Reply("Usage: weather [city/town]")
	}

	query := strings.Join(ctx.Args, " ")
	escapedQuery := url.QueryEscape(query)
	apiURL := fmt.Sprintf("https://wttr.in/%s?format=4", escapedQuery)

	req, err := http.NewRequestWithContext(ctx.Ctx, "GET", apiURL, nil)
	if err != nil {
		return ctx.Reply(fmt.Sprintf(" Error creating request: %v", err))
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ctx.Reply(fmt.Sprintf(" Network error: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ctx.Reply(fmt.Sprintf(" Weather service returned status: %s", resp.Status))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return ctx.Reply(fmt.Sprintf(" Error reading response: %v", err))
	}

	forecast := strings.TrimSpace(string(bodyBytes))
	if forecast == "" || strings.Contains(forecast, "Unknown location") {
		return ctx.Reply(fmt.Sprintf(" Could not find weather info for %q.", query))
	}

	return ctx.Reply(forecast)
}
