// Quote image rendering engine based on gg context canvas drawing.
package utils

import (
	"context"
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/fogleman/gg"
)

type QuoteColorScheme struct {
	Background string
	BubbleBg   string
	QuotedBg   string
	AccentBar  string
	NameColor  string
	PhoneColor string
	TextColor  string
	QuotedName string
	QuotedText string
	TimeColor  string
}

func DefaultQuoteScheme() QuoteColorScheme {
	return QuoteColorScheme{
		Background: "#0b0b0f",
		BubbleBg:   "#1f1f26",
		QuotedBg:   "#17171c",
		AccentBar:  "#ff5da2",
		NameColor:  "#ff9d5c",
		PhoneColor: "#c9c9c9",
		TextColor:  "#f0f0f0",
		QuotedName: "#ff5da2",
		QuotedText: "#b8b8b8",
		TimeColor:  "#888888",
	}
}

type QuoteMessage struct {
	Username   string
	UserPhone  string // raw digits, e.g. "2348060598064"
	AvatarPath string // path to avatar image or empty
	Quoted     bool
	QuotedName string
	QuotedText string
	Content    string
	Timestamp  string
}

var (
	quoteRegularFontPath string
	quoteBoldFontPath    string
	quoteCJKFontPath     string
)

func resolveQuoteFonts() error {
	// 1. Try Roboto fonts in cli/resources/fonts
	candidatesRegular := []string{
		"cli/resources/fonts/static/Roboto-Regular.ttf",
		"resources/fonts/static/Roboto-Regular.ttf",
		"cli/resources/fonts/Roboto-VariableFont_wdth,wght.ttf",
		"/usr/share/fonts/dejavu-sans-fonts/DejaVuSans.ttf",
		"/usr/share/fonts/TTF/DejaVuSans.ttf",
	}
	candidatesBold := []string{
		"cli/resources/fonts/static/Roboto-Bold.ttf",
		"resources/fonts/static/Roboto-Bold.ttf",
		"cli/resources/fonts/static/Roboto-SemiBold.ttf",
		"/usr/share/fonts/dejavu-sans-fonts/DejaVuSans-Bold.ttf",
		"/usr/share/fonts/TTF/DejaVuSans-Bold.ttf",
	}

	for _, p := range candidatesRegular {
		if _, err := os.Stat(p); err == nil {
			quoteRegularFontPath, _ = filepath.Abs(p)
			break
		}
	}
	for _, p := range candidatesBold {
		if _, err := os.Stat(p); err == nil {
			quoteBoldFontPath, _ = filepath.Abs(p)
			break
		}
	}

	if quoteRegularFontPath == "" {
		return fmt.Errorf("regular font not found")
	}
	if quoteBoldFontPath == "" {
		quoteBoldFontPath = quoteRegularFontPath
	}

	cjkCandidates := []string{
		"/usr/share/fonts/google-noto-cjk/NotoSansCJK-Regular.ttc",
		"/usr/share/fonts/google-noto-sans-cjk-jp-fonts/NotoSansCJKjp-Regular.otf",
		"/usr/share/fonts/google-noto-sans-cjk-fonts/NotoSansCJK-Regular.ttc",
		"/usr/share/fonts/noto-cjk/NotoSansCJK-Regular.ttc",
	}
	for _, c := range cjkCandidates {
		if _, err := os.Stat(c); err == nil {
			quoteCJKFontPath = c
			break
		}
	}

	return nil
}

func isLatinCoverable(r rune) bool {
	switch {
	case r < 0x0530:
		return true
	case unicode.IsPunct(r), unicode.IsSpace(r), unicode.IsSymbol(r):
		return true
	case r >= 0x2000 && r <= 0x2BFF:
		return true
	default:
		return false
	}
}

type textRun struct {
	text   string
	useCJK bool
}

func splitRuns(s string) []textRun {
	var runs []textRun
	var cur []rune
	curCJK := false
	first := true

	flush := func() {
		if len(cur) > 0 {
			runs = append(runs, textRun{text: string(cur), useCJK: curCJK})
			cur = nil
		}
	}

	for _, r := range s {
		needsCJK := !isLatinCoverable(r)
		if first {
			curCJK = needsCJK
			first = false
		} else if needsCJK != curCJK {
			flush()
			curCJK = needsCJK
		}
		cur = append(cur, r)
	}
	flush()
	return runs
}

func drawMixedString(dc *gg.Context, s string, x, y float64, size float64, bold bool) float64 {
	basePath := quoteRegularFontPath
	if bold {
		basePath = quoteBoldFontPath
	}
	cursorX := x
	for _, run := range splitRuns(s) {
		facePath := basePath
		if run.useCJK && quoteCJKFontPath != "" {
			facePath = quoteCJKFontPath
		}
		_ = dc.LoadFontFace(facePath, size)
		dc.DrawString(run.text, cursorX, y)
		w, _ := dc.MeasureString(run.text)
		cursorX += w
	}
	return cursorX - x
}

func measureMixedString(dc *gg.Context, s string, size float64, bold bool) float64 {
	basePath := quoteRegularFontPath
	if bold {
		basePath = quoteBoldFontPath
	}
	total := 0.0
	for _, run := range splitRuns(s) {
		facePath := basePath
		if run.useCJK && quoteCJKFontPath != "" {
			facePath = quoteCJKFontPath
		}
		_ = dc.LoadFontFace(facePath, size)
		w, _ := dc.MeasureString(run.text)
		total += w
	}
	return total
}

func formatPhone(raw string) string {
	if raw == "" {
		return ""
	}
	hasPlus := strings.HasPrefix(raw, "+")
	digits := strings.Map(func(r rune) rune {
		if unicode.IsDigit(r) {
			return r
		}
		return -1
	}, raw)
	if digits == "" {
		return raw
	}

	oneDigit := map[byte]bool{'1': true, '7': true}
	twoDigitPrefixes := map[string]bool{
		"20": true, "27": true, "30": true, "31": true, "32": true, "33": true,
		"34": true, "36": true, "39": true, "40": true, "41": true, "43": true,
		"44": true, "45": true, "46": true, "47": true, "48": true, "49": true,
		"51": true, "52": true, "53": true, "54": true, "55": true, "56": true,
		"57": true, "58": true, "60": true, "61": true, "62": true, "63": true,
		"64": true, "65": true, "66": true, "81": true, "82": true, "84": true,
		"86": true, "90": true, "91": true, "92": true, "93": true, "94": true,
		"95": true, "98": true,
	}

	ccLen := 3
	if len(digits) >= 1 && oneDigit[digits[0]] {
		ccLen = 1
	} else if len(digits) >= 2 && twoDigitPrefixes[digits[:2]] {
		ccLen = 2
	}
	if ccLen >= len(digits) {
		ccLen = 1
	}

	cc := digits[:ccLen]
	rest := digits[ccLen:]

	var groups []string
	for len(rest) > 4 {
		groups = append(groups, rest[:3])
		rest = rest[3:]
	}
	if len(rest) > 0 {
		groups = append(groups, rest)
	}

	prefix := ""
	if hasPlus {
		prefix = "+"
	}
	return fmt.Sprintf("%s%s %s", prefix, cc, strings.Join(groups, "-"))
}

var pastelPalette = []string{
	"#AEE1F9",
	"#B8F2C9",
	"#FDE2B8",
	"#E8C7F9",
	"#FFD6E0",
	"#C7F0EB",
	"#FFF3B0",
}

func pickAvatarColor(name string) string {
	h := fnv.New32a()
	h.Write([]byte(name))
	idx := int(h.Sum32()) % len(pastelPalette)
	if idx < 0 {
		idx += len(pastelPalette)
	}
	return pastelPalette[idx]
}

func firstLetter(name string) string {
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return strings.ToUpper(string(r))
		}
	}
	return "?"
}

func drawAvatar(dc *gg.Context, msg QuoteMessage, cx, cy, radius float64) {
	if msg.AvatarPath != "" {
		if img, err := gg.LoadImage(msg.AvatarPath); err == nil {
			dc.Push()
			dc.DrawCircle(cx, cy, radius)
			dc.Clip()
			dc.DrawImageAnchored(resizeToFit(img, int(radius*2), int(radius*2)), int(cx), int(cy), 0.5, 0.5)
			dc.ResetClip()
			dc.Pop()
			return
		}
	}

	bg := pickAvatarColor(msg.Username)
	dc.SetColor(hexToRGBA(bg))
	dc.DrawCircle(cx, cy, radius)
	dc.Fill()

	letter := firstLetter(msg.Username)
	dc.SetColor(color.RGBA{40, 40, 40, 255})
	_ = dc.LoadFontFace(quoteBoldFontPath, radius)
	tw, th := dc.MeasureString(letter)
	dc.DrawString(letter, cx-tw/2, cy+th/2-2)
}

func resizeToFit(img image.Image, w, h int) image.Image {
	dc := gg.NewContext(w, h)
	sx := float64(w) / float64(img.Bounds().Dx())
	sy := float64(h) / float64(img.Bounds().Dy())
	dc.Scale(sx, sy)
	dc.DrawImage(img, 0, 0)
	return dc.Image()
}

func drawBubbleTail(dc *gg.Context, bubbleX, bubbleY float64, fillColor color.Color) {
	dc.SetColor(fillColor)
	dc.MoveTo(bubbleX, bubbleY+2)
	dc.LineTo(bubbleX-8, bubbleY+14)
	dc.LineTo(bubbleX, bubbleY+22)
	dc.ClosePath()
	dc.Fill()
}

const (
	quoteCanvasWidth   = 900
	quotePaddingX      = 28
	quoteAvatarRadius  = 32.0
	quoteAvatarGap     = 16.0
	quoteBubblePadding = 22
	quoteFontSize      = 22
	quoteNameFontSize  = 23
	quoteSmallFontSize = 18
	quoteLineHeight    = 28
)

func hexToRGBA(hex string) color.Color {
	c, err := parseHexColor(hex)
	if err != nil {
		return color.Black
	}
	return c
}

func parseHexColor(s string) (color.RGBA, error) {
	var r, g, b uint8
	_, err := fmt.Sscanf(s, "#%02x%02x%02x", &r, &g, &b)
	return color.RGBA{r, g, b, 255}, err
}

// RenderQuote creates an image representation of a chat quote message.
func RenderQuote(ctx context.Context, msg QuoteMessage, scheme QuoteColorScheme) (image.Image, error) {
	if err := resolveQuoteFonts(); err != nil {
		return nil, fmt.Errorf("font resolution error: %w", err)
	}

	formattedPhone := formatPhone(msg.UserPhone)
	bubbleX := float64(quotePaddingX) + quoteAvatarRadius*2 + quoteAvatarGap
	bubbleW := float64(quoteCanvasWidth) - bubbleX - float64(quotePaddingX)

	measureDC := gg.NewContext(quoteCanvasWidth, 10)
	_ = measureDC.LoadFontFace(quoteRegularFontPath, quoteFontSize)

	nameLineHeight := 32.0

	quoteHeight := 0.0
	var quoteLines []string
	if msg.Quoted && msg.QuotedText != "" {
		_ = measureDC.LoadFontFace(quoteRegularFontPath, quoteSmallFontSize)
		quoteLines = measureDC.WordWrap(msg.QuotedText, bubbleW-quoteBubblePadding*2-24)
		quoteHeight = 24 + float64(len(quoteLines))*22 + 12
	}

	_ = measureDC.LoadFontFace(quoteRegularFontPath, quoteFontSize)
	contentLines := measureDC.WordWrap(msg.Content, bubbleW-quoteBubblePadding*2)
	contentHeight := float64(len(contentLines)) * quoteLineHeight

	bubbleHeight := float64(quoteBubblePadding)*2 + nameLineHeight + quoteHeight + contentHeight + 28
	totalHeight := int(math.Ceil(math.Max(20+bubbleHeight+20, quoteAvatarRadius*2+40)))

	dc := gg.NewContext(quoteCanvasWidth, totalHeight)

	dc.SetColor(hexToRGBA(scheme.Background))
	dc.Clear()

	bubbleY := 20.0

	avatarCX := float64(quotePaddingX) + quoteAvatarRadius
	avatarCY := bubbleY + bubbleHeight/2
	drawAvatar(dc, msg, avatarCX, avatarCY, quoteAvatarRadius)

	dc.SetColor(hexToRGBA(scheme.BubbleBg))
	dc.DrawRoundedRectangle(bubbleX, bubbleY, bubbleW, bubbleHeight, 16)
	dc.Fill()

	drawBubbleTail(dc, bubbleX, bubbleY, hexToRGBA(scheme.BubbleBg))

	textX := bubbleX + quoteBubblePadding
	textY := bubbleY + quoteBubblePadding
	rightEdge := bubbleX + bubbleW - quoteBubblePadding

	displayName := "~ " + msg.Username
	if !strings.HasPrefix(msg.Username, "~") {
		displayName = "~ " + msg.Username
	} else {
		displayName = msg.Username
	}
	dc.SetColor(hexToRGBA(scheme.NameColor))
	drawMixedString(dc, displayName, textX, textY+16, quoteNameFontSize, true)

	if formattedPhone != "" {
		dc.SetColor(hexToRGBA(scheme.PhoneColor))
		phoneWidth := measureMixedString(dc, formattedPhone, quoteNameFontSize-3, false)
		drawMixedString(dc, formattedPhone, rightEdge-phoneWidth, textY+16, quoteNameFontSize-3, false)
	}

	textY += nameLineHeight

	if msg.Quoted && msg.QuotedText != "" {
		dc.SetColor(hexToRGBA(scheme.QuotedBg))
		dc.DrawRoundedRectangle(textX-6, textY-4, bubbleW-quoteBubblePadding*2+12, quoteHeight+4, 8)
		dc.Fill()

		dc.SetColor(hexToRGBA(scheme.AccentBar))
		dc.DrawRectangle(textX, textY, 4, quoteHeight-6)
		dc.Fill()

		dc.SetColor(hexToRGBA(scheme.QuotedName))
		drawMixedString(dc, msg.QuotedName, textX+16, textY+18, quoteSmallFontSize, true)

		dc.SetColor(hexToRGBA(scheme.QuotedText))
		qy := textY + 42
		for _, line := range quoteLines {
			drawMixedString(dc, line, textX+16, qy, quoteSmallFontSize, false)
			qy += 22
		}
		textY += quoteHeight + 8
	}

	dc.SetColor(hexToRGBA(scheme.TextColor))
	cy := textY + quoteFontSize
	for _, line := range contentLines {
		drawMixedString(dc, line, textX, cy, quoteFontSize, false)
		cy += quoteLineHeight
	}

	if msg.Timestamp != "" {
		dc.SetColor(hexToRGBA(scheme.TimeColor))
		_ = dc.LoadFontFace(quoteRegularFontPath, 15)
		tw, _ := dc.MeasureString(msg.Timestamp)
		dc.DrawString(msg.Timestamp, bubbleX+bubbleW-quoteBubblePadding-tw, bubbleY+bubbleHeight-10)
	}

	return dc.Image(), nil
}
