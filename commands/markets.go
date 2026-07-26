// Markets command – fetches real-time Forex Factory market rates and quotes.
package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waE2E"
)

func init() {
	Register(&Command{
		Name:        "markets",
		Aliases:     []string{"forex", "market", "ff", "fx"},
		Description: "View real-time Forex Factory currency & market rates",
		Category:    "info",
		IsPublic:    true,
		Handler:     handleMarkets,
	})
}

type FFInstrumentResponse struct {
	Data []FFInstrumentData `json:"data"`
}

type FFInstrumentData struct {
	Instrument struct {
		ID          int    `json:"id"`
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
		Decimals    int    `json:"decimals"`
		InHoliday   bool   `json:"is_in_holiday"`
	} `json:"instrument"`
	Metrics map[string]struct {
		Price  float64 `json:"price"`
		High   float64 `json:"high"`
		Low    float64 `json:"low"`
		Spread float64 `json:"spread"`
	} `json:"metrics"`
	Quotes []struct {
		Instrument string  `json:"instrument"`
		Bid        float64 `json:"bid"`
		Ask        float64 `json:"ask"`
	} `json:"quotes"`
}

func handleMarkets(ctx *Context) error {
	if len(ctx.Args) == 0 {
		// If sender device is NOT Android/iOS mobile (Device != 0, e.g. Web/Desktop companion), send normal buttons.
		// Else (Device == 0, Android/iOS mobile), send single_select native flow list.
		if ctx.Sender.Device != 0 {
			return sendMarketsButtonsMenu(ctx)
		}
		return sendMarketsSelectListMenu(ctx)
	}

	queryArg := strings.ToUpper(strings.TrimSpace(strings.Join(ctx.Args, "")))
	queryArg = strings.ReplaceAll(queryArg, "-", "/")
	queryArg = strings.ReplaceAll(queryArg, " ", "")

	if queryArg == "MENU" || queryArg == "LIST" || queryArg == "ALL" {
		return fetchAndSendAllMarkets(ctx)
	}

	switch queryArg {
	case "GOLD", "XAUUSD", "GOLD/USD":
		queryArg = "Gold/USD"
	case "SILVER", "XAGUSD", "SILVER/USD":
		queryArg = "Silver/USD"
	case "EURUSD":
		queryArg = "EUR/USD"
	case "GBPUSD":
		queryArg = "GBP/USD"
	case "USDJPY":
		queryArg = "USD/JPY"
	case "USDCHF":
		queryArg = "USD/CHF"
	case "USDCAD":
		queryArg = "USD/CAD"
	case "AUDUSD":
		queryArg = "AUD/USD"
	case "NZDUSD":
		queryArg = "NZD/USD"
	case "BTCUSD", "BTC":
		queryArg = "BTC/USD"
	}

	return fetchAndSendSingleMarket(ctx, queryArg)
}

func fetchAndSendSingleMarket(ctx *Context, pair string) error {
	p := ctx.GetPrefix()
	apiURL := fmt.Sprintf("https://mds-api.forexfactory.com/instruments?instruments=%s", url.QueryEscape(pair))

	req, err := http.NewRequestWithContext(ctx.Ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return ctx.Reply(fmt.Sprintf("Failed to create request: %v", err))
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64)")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ctx.Reply(fmt.Sprintf("Failed to fetch market data for %s: %v", pair, err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ctx.Reply(fmt.Sprintf("Market API error (HTTP %d). Usage:\n- %smarkets EUR/USD", resp.StatusCode, p))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ctx.Reply("Failed to read market API response.")
	}

	var res FFInstrumentResponse
	if err := json.Unmarshal(body, &res); err != nil || len(res.Data) == 0 {
		return ctx.Reply(fmt.Sprintf("Instrument %q not found. Usage:\n- %smarkets EUR/USD\n- %smarkets Gold/USD\n- %smarkets all", pair, p, p, p))
	}

	item := res.Data[0]
	displayName := item.Instrument.DisplayName
	if displayName == "" {
		displayName = pair
	}

	var price, high, low, spread float64
	var bid, ask float64

	if d1, ok := item.Metrics["D1"]; ok {
		price = d1.Price
		high = d1.High
		low = d1.Low
		spread = d1.Spread
	} else if h1, ok := item.Metrics["H1"]; ok {
		price = h1.Price
		high = h1.High
		low = h1.Low
		spread = h1.Spread
	}

	if len(item.Quotes) > 0 {
		bid = item.Quotes[0].Bid
		ask = item.Quotes[0].Ask
		if price == 0 {
			price = (bid + ask) / 2
		}
	}

	decimals := item.Instrument.Decimals
	if decimals == 0 {
		decimals = 4
	}

	marketStatus := "Open"
	if item.Instrument.InHoliday {
		marketStatus = "Holiday / Closed"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Forex Factory Rates - %s\n\n", displayName))
	if price > 0 {
		sb.WriteString(fmt.Sprintf("Price: %.*f\n", decimals, price))
	}
	if bid > 0 && ask > 0 {
		sb.WriteString(fmt.Sprintf("Bid: %.*f | Ask: %.*f\n", decimals, bid, decimals, ask))
	}
	if high > 0 && low > 0 {
		sb.WriteString(fmt.Sprintf("24h High: %.*f | 24h Low: %.*f\n", decimals, high, decimals, low))
	}
	if spread > 0 {
		sb.WriteString(fmt.Sprintf("Spread: %.1f pips\n", spread))
	}
	sb.WriteString(fmt.Sprintf("Market Status: %s\n", marketStatus))

	return ctx.Reply(sb.String())
}

func fetchAndSendAllMarkets(ctx *Context) error {
	pairs := []string{"EUR/USD", "GBP/USD", "USD/JPY", "USD/CHF", "USD/CAD", "AUD/USD", "NZD/USD", "Gold/USD"}
	apiURL := fmt.Sprintf("https://mds-api.forexfactory.com/instruments?instruments=%s", url.QueryEscape(strings.Join(pairs, ",")))

	req, err := http.NewRequestWithContext(ctx.Ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return ctx.Reply(fmt.Sprintf("Failed to create request: %v", err))
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64)")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ctx.Reply(fmt.Sprintf("Failed to fetch market rates: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ctx.Reply(fmt.Sprintf("Market API error (HTTP %d).", resp.StatusCode))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ctx.Reply("Failed to read market API response.")
	}

	var res FFInstrumentResponse
	if err := json.Unmarshal(body, &res); err != nil || len(res.Data) == 0 {
		return ctx.Reply("No market rates available at this time.")
	}

	var sb strings.Builder
	sb.WriteString("Forex Factory Market Overview\n\n")

	for _, item := range res.Data {
		displayName := item.Instrument.DisplayName
		var price float64
		if len(item.Quotes) > 0 {
			price = (item.Quotes[0].Bid + item.Quotes[0].Ask) / 2
		}
		if price == 0 {
			if d1, ok := item.Metrics["D1"]; ok {
				price = d1.Price
			}
		}

		decimals := item.Instrument.Decimals
		if decimals == 0 {
			decimals = 4
		}

		fmt.Fprintf(&sb, "- %s: %.*f\n", displayName, decimals, price)
	}

	return ctx.Reply(strings.TrimSpace(sb.String()))
}

func sendMarketsButtonsMenu(ctx *Context) error {
	p := ctx.GetPrefix()
	bodyText := "Forex Factory Markets\n\nSelect an instrument below to check real-time rates:\n\nExamples:\n- " + p + "markets EUR/USD\n- " + p + "markets Gold/USD\n- " + p + "markets all"

	msg := &waE2E.Message{
		DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				ButtonsMessage: &waE2E.ButtonsMessage{
					ContentText: new(bodyText),
					FooterText:  new("Forex Factory Rates"),
					HeaderType:  waE2E.ButtonsMessage_EMPTY.Enum(),
					Buttons: []*waE2E.ButtonsMessage_Button{
						{
							ButtonID: new(p + "markets EUR/USD"),
							ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
								DisplayText: new("EUR/USD"),
							},
							Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
						},
						{
							ButtonID: new(p + "markets GBP/USD"),
							ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
								DisplayText: new("GBP/USD"),
							},
							Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
						},
						{
							ButtonID: new(p + "markets Gold/USD"),
							ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
								DisplayText: new("GOLD/USD"),
							},
							Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
						},
					},
				},
			},
		},
	}

	bizNode := waBinary.Node{
		Tag:   "biz",
		Attrs: waBinary.Attrs{},
		Content: []waBinary.Node{
			{
				Tag:   "interactive",
				Attrs: waBinary.Attrs{"type": "native_flow", "v": "1"},
				Content: []waBinary.Node{
					{
						Tag:   "native_flow",
						Attrs: waBinary.Attrs{"name": "quick_reply"},
					},
				},
			},
		},
	}

	extra := whatsmeow.SendRequestExtra{
		AdditionalNodes: &[]waBinary.Node{bizNode},
	}

	_, err := ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg, extra)
	return err
}

func sendMarketsSelectListMenu(ctx *Context) error {
	msgVersion := int32(1)
	p := ctx.GetPrefix()

	msg := &waE2E.Message{
		DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				InteractiveMessage: &waE2E.InteractiveMessage{
					Body: &waE2E.InteractiveMessage_Body{
						Text: new("Forex Factory Market Rates\n\nClick the button below to choose an instrument:"),
					},
					Footer: &waE2E.InteractiveMessage_Footer{
						Text: new("Forex Factory Data"),
					},
					InteractiveMessage: &waE2E.InteractiveMessage_NativeFlowMessage_{
						NativeFlowMessage: &waE2E.InteractiveMessage_NativeFlowMessage{
							Buttons: []*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{
								{
									Name: new("single_select"),
									ButtonParamsJSON: new(fmt.Sprintf(`{
										"title": "Select Instrument",
										"sections": [
											{
												"title": "Forex Major Pairs",
												"rows": [
													{
														"id": "%smarkets EUR/USD",
														"title": "EUR/USD",
														"description": "Euro / US Dollar"
													},
													{
														"id": "%smarkets GBP/USD",
														"title": "GBP/USD",
														"description": "British Pound / US Dollar"
													},
													{
														"id": "%smarkets USD/JPY",
														"title": "USD/JPY",
														"description": "US Dollar / Japanese Yen"
													},
													{
														"id": "%smarkets USD/CAD",
														"title": "USD/CAD",
														"description": "US Dollar / Canadian Dollar"
													},
													{
														"id": "%smarkets AUD/USD",
														"title": "AUD/USD",
														"description": "Australian Dollar / US Dollar"
													}
												]
											},
											{
												"title": "Metals & Commodities",
												"rows": [
													{
														"id": "%smarkets Gold/USD",
														"title": "Gold/USD",
														"description": "Spot Gold / US Dollar"
													},
													{
														"id": "%smarkets Silver/USD",
														"title": "Silver/USD",
														"description": "Spot Silver / US Dollar"
													}
												]
											},
											{
												"title": "Overview",
												"rows": [
													{
														"id": "%smarkets all",
														"title": "All Majors Summary",
														"description": "View rates summary for all major pairs"
													}
												]
											}
										]
									}`, p, p, p, p, p, p, p, p)),
								},
							},
							MessageVersion: &msgVersion,
						},
					},
				},
			},
		},
	}

	bizNode := waBinary.Node{
		Tag:   "biz",
		Attrs: waBinary.Attrs{},
		Content: []waBinary.Node{
			{
				Tag:   "interactive",
				Attrs: waBinary.Attrs{"type": "native_flow", "v": "1"},
				Content: []waBinary.Node{
					{
						Tag:   "native_flow",
						Attrs: waBinary.Attrs{"v": "9", "name": "mixed"},
					},
				},
			},
		},
	}

	extra := whatsmeow.SendRequestExtra{
		AdditionalNodes: &[]waBinary.Node{bizNode},
	}

	_, err := ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg, extra)
	return err
}
