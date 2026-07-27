// Miscellaneous commands – urban dictionary, QR generation, etc.
package commands

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
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
	Register(&Command{
		Name:        "urban",
		Aliases:     []string{"ud", "define"},
		Description: "Look up a word or phrase on Urban Dictionary",
		Category:    "misc",
		IsPublic:    true,
		Handler:     handleUrban,
	})
	Register(&Command{
		Name:        "qrcode",
		Aliases:     []string{"qr"},
		Description: "Generate a QR code image for a text or URL",
		Category:    "misc",
		IsPublic:    true,
		Handler:     handleQRCode,
	})
	Register(&Command{
		Name:        "shorturl",
		Aliases:     []string{"shorten", "tinyurl"},
		Description: "Shorten a long URL using TinyURL",
		Category:    "misc",
		IsPublic:    true,
		Handler:     handleShortURL,
	})
	Register(&Command{
		Name:        "stkinfo",
		Aliases:     []string{"stickerinfo"},
		Description: "View technical metadata for a replied sticker",
		Category:    "misc",
		IsPublic:    true,
		Handler:     handleStickerInfo,
	})
	Register(&Command{
		Name:        "calc",
		Aliases:     []string{"math", "evaluate"},
		Description: "Evaluate a mathematical expression. Usage: calc [expression]",
		Category:    "misc",
		IsPublic:    true,
		Handler:     handleCalc,
	})
}

func handleSave(ctx *Context) error {
	quoted := ctx.GetQuotedMessage()
	if quoted == nil {
		return ctx.Reply("The basic functionality of this command is to save status updates. Please reply to a status update or any message to forward it to your DM.")
	}

	if ctx.Client.Store.ID == nil {
		return ctx.Reply("Owner ID unavailable.")
	}

	_, err := ctx.Client.SendMessage(ctx.Ctx, ctx.Sender, quoted)
	if err != nil {
		return ctx.Reply(fmt.Sprintf("Failed to forward message: %v", err))
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
		return ctx.Reply(fmt.Sprintf("Error creating request: %v", err))
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ctx.Reply(fmt.Sprintf("Network error: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ctx.Reply(fmt.Sprintf("Weather service returned status: %s", resp.Status))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return ctx.Reply(fmt.Sprintf("Error reading response: %v", err))
	}

	forecast := strings.TrimSpace(string(bodyBytes))
	if forecast == "" || strings.Contains(forecast, "Unknown location") {
		return ctx.Reply(fmt.Sprintf("Could not find weather info for %q.", query))
	}

	return ctx.Reply(forecast)
}

func handleUrban(ctx *Context) error {
	if len(ctx.Args) == 0 {
		return ctx.Reply("Usage: urban [term]")
	}

	query := strings.Join(ctx.Args, " ")
	apiURL := fmt.Sprintf("https://api.urbandictionary.com/v0/define?term=%s", url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx.Ctx, "GET", apiURL, nil)
	if err != nil {
		return ctx.Reply(fmt.Sprintf("Error creating request: %v", err))
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ctx.Reply(fmt.Sprintf("Network error: %v", err))
	}
	defer resp.Body.Close()

	var result struct {
		List []struct {
			Word       string `json:"word"`
			Definition string `json:"definition"`
			Example    string `json:"example"`
			Author     string `json:"author"`
			Permalink  string `json:"permalink"`
		} `json:"list"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || len(result.List) == 0 {
		return ctx.Reply(fmt.Sprintf("Could not find Urban Dictionary definition for %q.", query))
	}

	def := result.List[0]
	cleanDef := strings.ReplaceAll(strings.ReplaceAll(def.Definition, "[", ""), "]", "")
	cleanExample := strings.ReplaceAll(strings.ReplaceAll(def.Example, "[", ""), "]", "")

	out := fmt.Sprintf("Urban Dictionary: %s\n\nDefinition:\n%s", def.Word, cleanDef)
	if cleanExample != "" {
		out += fmt.Sprintf("\n\nExample:\n%s", cleanExample)
	}
	if def.Author != "" {
		out += fmt.Sprintf("\n\nAuthor: %s", def.Author)
	}
	return ctx.Reply(out)
}

func handleQRCode(ctx *Context) error {
	query := ctx.RawArgs
	if query == "" {
		if quoted := ctx.GetQuotedMessage(); quoted != nil {
			query = extractTextFromProto(quoted)
		}
	}
	if query == "" {
		p := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Usage: %sqr [text or url] (or reply to a message)", p))
	}

	apiURL := fmt.Sprintf("https://api.qrserver.com/v1/create-qr-code/?size=500x500&data=%s", url.QueryEscape(query))
	req, err := http.NewRequestWithContext(ctx.Ctx, "GET", apiURL, nil)
	if err != nil {
		return ctx.Reply(fmt.Sprintf("Error creating request: %v", err))
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ctx.Reply(fmt.Sprintf("Network error generating QR code: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ctx.Reply(fmt.Sprintf("QR code service returned status: %s", resp.Status))
	}

	imgData, err := io.ReadAll(resp.Body)
	if err != nil || len(imgData) == 0 {
		return ctx.Reply("Failed to read QR code image data.")
	}

	return ctx.ReplyWithImage(imgData, "image/png", "QR Code Generated")
}

func handleShortURL(ctx *Context) error {
	query := ctx.RawArgs
	if query == "" {
		if quoted := ctx.GetQuotedMessage(); quoted != nil {
			query = extractTextFromProto(quoted)
		}
	}
	query = strings.TrimSpace(query)
	if query == "" {
		p := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Usage: %sshorturl [url]", p))
	}

	if !strings.HasPrefix(query, "http://") && !strings.HasPrefix(query, "https://") {
		query = "https://" + query
	}

	apiURL := fmt.Sprintf("https://tinyurl.com/api-create.php?url=%s", url.QueryEscape(query))
	req, err := http.NewRequestWithContext(ctx.Ctx, "GET", apiURL, nil)
	if err != nil {
		return ctx.Reply(fmt.Sprintf("Error creating request: %v", err))
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ctx.Reply(fmt.Sprintf("Network error: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ctx.Reply("Failed to shorten URL. Please check if the URL is valid.")
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return ctx.Reply("Failed to read URL shortener response.")
	}

	short := strings.TrimSpace(string(bodyBytes))
	return ctx.Reply("Shortened URL: " + short)
}

func handleStickerInfo(ctx *Context) error {
	quoted := ctx.GetQuotedMessage()
	if quoted == nil || quoted.StickerMessage == nil {
		return ctx.Reply("Please reply to a sticker message to view its metadata.")
	}

	stk := quoted.StickerMessage
	mime := stk.GetMimetype()
	if mime == "" {
		mime = "image/webp"
	}

	shaHex := hex.EncodeToString(stk.GetFileSHA256())
	if shaHex == "" {
		shaHex = "unknown"
	}

	length := stk.GetFileLength()
	sizeStr := fmt.Sprintf("%d bytes", length)
	if length > 1024*1024 {
		sizeStr = fmt.Sprintf("%.2f MB", float64(length)/(1024*1024))
	} else if length > 1024 {
		sizeStr = fmt.Sprintf("%.2f KB", float64(length)/1024)
	}

	isAnimated := "No"
	if stk.GetIsAnimated() {
		isAnimated = "Yes"
	}

	out := fmt.Sprintf("Sticker Metadata\n\nMIME Type: %s\nFile Size: %s\nAnimated: %s\nSHA256: %s", mime, sizeStr, isAnimated, shaHex)
	return ctx.Reply(out)
}

func handleCalc(ctx *Context) error {
	if len(ctx.Args) == 0 {
		p := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Usage: %scalc [expression]", p))
	}

	exprStr := strings.Join(ctx.Args, "")
	val, err := evalMathExpr(exprStr)
	if err != nil {
		return ctx.Reply(fmt.Sprintf("Math error: %v", err))
	}

	return ctx.Reply(fmt.Sprintf("Result: %g", val))
}

func evalMathExpr(expr string) (float64, error) {
	expr = strings.ReplaceAll(expr, " ", "")
	if expr == "" {
		return 0, fmt.Errorf("empty expression")
	}
	p := &exprParser{s: expr}
	res, err := p.parseExpr()
	if err != nil {
		return 0, err
	}
	if p.pos < len(p.s) {
		return 0, fmt.Errorf("unexpected token %q at position %d", p.s[p.pos:], p.pos)
	}
	return res, nil
}

type exprParser struct {
	s   string
	pos int
}

func (p *exprParser) parseExpr() (float64, error) {
	val, err := p.parseTerm()
	if err != nil {
		return 0, err
	}
	for p.pos < len(p.s) {
		op := p.s[p.pos]
		if op != '+' && op != '-' {
			break
		}
		p.pos++
		nextVal, err := p.parseTerm()
		if err != nil {
			return 0, err
		}
		if op == '+' {
			val += nextVal
		} else {
			val -= nextVal
		}
	}
	return val, nil
}

func (p *exprParser) parseTerm() (float64, error) {
	val, err := p.parseFactor()
	if err != nil {
		return 0, err
	}
	for p.pos < len(p.s) {
		op := p.s[p.pos]
		if op != '*' && op != '/' && op != '%' {
			break
		}
		p.pos++
		nextVal, err := p.parseFactor()
		if err != nil {
			return 0, err
		}
		switch op {
		case '*':
			val *= nextVal
		case '/':
			if nextVal == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			val /= nextVal
		default:
			if nextVal == 0 {
				return 0, fmt.Errorf("modulo by zero")
			}
			val = float64(int64(val) % int64(nextVal))
		}
	}
	return val, nil
}

func (p *exprParser) parseFactor() (float64, error) {
	if p.pos >= len(p.s) {
		return 0, fmt.Errorf("unexpected end of expression")
	}

	if p.s[p.pos] == '-' {
		p.pos++
		val, err := p.parseFactor()
		return -val, err
	}
	if p.s[p.pos] == '+' {
		p.pos++
		return p.parseFactor()
	}

	if p.s[p.pos] == '(' {
		p.pos++
		val, err := p.parseExpr()
		if err != nil {
			return 0, err
		}
		if p.pos >= len(p.s) || p.s[p.pos] != ')' {
			return 0, fmt.Errorf("missing closing parenthesis")
		}
		p.pos++
		return val, nil
	}

	start := p.pos
	for p.pos < len(p.s) && ((p.s[p.pos] >= '0' && p.s[p.pos] <= '9') || p.s[p.pos] == '.') {
		p.pos++
	}
	if start == p.pos {
		return 0, fmt.Errorf("invalid character %q", p.s[p.pos:p.pos+1])
	}

	val, err := strconv.ParseFloat(p.s[start:p.pos], 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number %q", p.s[start:p.pos])
	}
	return val, nil
}
