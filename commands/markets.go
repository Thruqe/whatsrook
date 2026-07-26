// Markets command – fetches real-time Forex Factory market rates and quotes.
package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
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

type FFListItem struct {
	ID                  int    `json:"id"`
	Name                string `json:"name"`
	Title               string `json:"title"`
	DisplayName         string `json:"display_name"`
	InstrumentClassName string `json:"instrument_class_name"`
	Rank                int    `json:"rank"`
}

type FFListResponse struct {
	Data []FFListItem `json:"data"`
}

func handleMarkets(ctx *Context) error {
	slog.Debug("handleMarkets executing", "chat", ctx.Chat.String(), "sender", ctx.Sender.String(), "device", ctx.Sender.Device, "args", ctx.Args)

	if len(ctx.Args) == 0 {
		// Device platform check:
		// Device != 0 indicates Web/Desktop companion client -> send normal buttons.
		// Device == 0 indicates Android/iOS mobile client -> send single_select native flow selectlist.
		slog.Info("handleMarkets: no args provided, routing menu", "device", ctx.Sender.Device)
		if ctx.Sender.Device != 0 {
			slog.Debug("handleMarkets: routing to sendMarketsButtonsMenu for non-mobile device", "device", ctx.Sender.Device)
			return sendMarketsButtonsMenu(ctx)
		}
		slog.Debug("handleMarkets: routing to sendMarketsSelectListMenu for mobile device", "device", ctx.Sender.Device)
		return sendMarketsSelectListMenu(ctx)
	}

	queryArg := strings.ToUpper(strings.TrimSpace(strings.Join(ctx.Args, "")))
	queryArg = strings.ReplaceAll(queryArg, "-", "/")
	queryArg = strings.ReplaceAll(queryArg, " ", "")
	slog.Debug("handleMarkets: parsed query argument", "raw_args", ctx.Args, "parsed_query", queryArg)

	if queryArg == "MENU" || queryArg == "LIST" || queryArg == "ALL" {
		slog.Info("handleMarkets: requested market summary overview", "query", queryArg)
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
	case "ETHUSD", "ETH":
		queryArg = "ETH/USD"
	case "HEATOIL/USD", "HEATOIL":
		queryArg = "Oil/USD"
	}

	slog.Info("handleMarkets: querying single instrument", "pair", queryArg)
	return fetchAndSendSingleMarket(ctx, queryArg)
}

func fetchForexFactoryInstrumentList(ctx context.Context) ([]FFListItem, error) {
	client := &http.Client{Timeout: 8 * time.Second}
	var allItems []FFListItem

	// 1. Fetch main instrument list
	apiURL := "https://mds-api.forexfactory.com/instrument-list"
	slog.Debug("fetchForexFactoryInstrumentList: fetching main instrument list from API", "url", apiURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err == nil {
		req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64)")
		if resp, err := client.Do(req); err == nil && resp.StatusCode == http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			var res FFListResponse
			if err := json.Unmarshal(body, &res); err == nil {
				allItems = append(allItems, res.Data...)
			}
		}
	}

	// 2. Fetch synthetic / crypto instrument list
	synthURL := "https://mds-api.forexfactory.com/synthetic-instrument-list"
	slog.Debug("fetchForexFactoryInstrumentList: fetching synthetic instrument list from API", "url", synthURL)
	synthReq, err := http.NewRequestWithContext(ctx, http.MethodGet, synthURL, nil)
	if err == nil {
		synthReq.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64)")
		if resp, err := client.Do(synthReq); err == nil && resp.StatusCode == http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			var res FFListResponse
			if err := json.Unmarshal(body, &res); err == nil {
				allItems = append(allItems, res.Data...)
			}
		}
	}

	slog.Debug("fetchForexFactoryInstrumentList: successfully retrieved total instruments", "count", len(allItems))
	return allItems, nil
}

func fetchAndSendSingleMarket(ctx *Context, pair string) error {
	p := ctx.GetPrefix()
	apiURL := fmt.Sprintf("https://mds-api.forexfactory.com/instruments?instruments=%s", url.QueryEscape(pair))
	slog.Debug("fetchAndSendSingleMarket: requesting market metrics from API", "pair", pair, "url", apiURL)

	req, err := http.NewRequestWithContext(ctx.Ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		slog.Error("fetchAndSendSingleMarket: failed to create request", "pair", pair, "err", err)
		return ctx.Reply(fmt.Sprintf("Failed to create request: %v", err))
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64)")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("fetchAndSendSingleMarket: HTTP request failed", "pair", pair, "err", err)
		return ctx.Reply(fmt.Sprintf("Failed to fetch market data for %s: %v", pair, err))
	}
	defer resp.Body.Close()

	slog.Debug("fetchAndSendSingleMarket: API response received", "pair", pair, "status", resp.StatusCode)
	if resp.StatusCode == http.StatusNotFound {
		slog.Warn("fetchAndSendSingleMarket: HTTP 404 instrument unavailable", "pair", pair)
		return ctx.Reply(fmt.Sprintf("Instrument %q is currently unavailable on Forex Factory.\n\nPopular Available Instruments:\n- %smarkets EUR/USD\n- %smarkets GBP/USD\n- %smarkets USD/JPY\n- %smarkets Gold/USD\n- %smarkets Silver/USD\n- %smarkets BTC/USD\n- %smarkets all", pair, p, p, p, p, p, p, p))
	} else if resp.StatusCode != http.StatusOK {
		slog.Warn("fetchAndSendSingleMarket: non-200 HTTP status from API", "pair", pair, "status", resp.StatusCode)
		return ctx.Reply(fmt.Sprintf("Market API error (HTTP %d). Usage:\n- %smarkets EUR/USD", resp.StatusCode, p))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("fetchAndSendSingleMarket: failed to read response body", "pair", pair, "err", err)
		return ctx.Reply("Failed to read market API response.")
	}

	var res FFInstrumentResponse
	if err := json.Unmarshal(body, &res); err != nil || len(res.Data) == 0 {
		slog.Warn("fetchAndSendSingleMarket: empty data returned", "pair", pair, "err", err, "data_len", len(res.Data))
		return ctx.Reply(fmt.Sprintf("Instrument %q is currently unavailable on Forex Factory.\n\nPopular Available Instruments:\n- %smarkets EUR/USD\n- %smarkets Gold/USD\n- %smarkets BTC/USD\n- %smarkets all", pair, p, p, p, p))
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

	slog.Info("fetchAndSendSingleMarket: parsed market data", "pair", displayName, "price", price, "bid", bid, "ask", ask, "high", high, "low", low, "spread", spread, "status", marketStatus)

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
	slog.Debug("fetchAndSendAllMarkets: requesting all major markets from API", "url", apiURL)

	req, err := http.NewRequestWithContext(ctx.Ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		slog.Error("fetchAndSendAllMarkets: failed to create request", "err", err)
		return ctx.Reply(fmt.Sprintf("Failed to create request: %v", err))
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64)")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("fetchAndSendAllMarkets: HTTP request failed", "err", err)
		return ctx.Reply(fmt.Sprintf("Failed to fetch market rates: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Warn("fetchAndSendAllMarkets: non-200 HTTP status from API", "status", resp.StatusCode)
		return ctx.Reply(fmt.Sprintf("Market API error (HTTP %d).", resp.StatusCode))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("fetchAndSendAllMarkets: failed to read response body", "err", err)
		return ctx.Reply("Failed to read market API response.")
	}

	var res FFInstrumentResponse
	if err := json.Unmarshal(body, &res); err != nil || len(res.Data) == 0 {
		slog.Warn("fetchAndSendAllMarkets: no data returned from API", "err", err, "data_len", len(res.Data))
		return ctx.Reply("No market rates available at this time.")
	}

	slog.Info("fetchAndSendAllMarkets: successfully parsed market overview", "item_count", len(res.Data))

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
	slog.Debug("sendMarketsButtonsMenu: building normal buttons for Web/Desktop non-mobile device", "chat", ctx.Chat.String(), "device", ctx.Sender.Device)

	bodyText := "Forex Factory Markets\n\nSelect an instrument below to check real-time rates:\n\nExamples:\n- " + p + "markets EUR/USD\n- " + p + "markets Gold/USD\n- " + p + "markets BTC/USD\n- " + p + "markets all"

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

	slog.Info("sendMarketsButtonsMenu: sending buttons message", "chat", ctx.Chat.String())
	_, err := ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg, extra)
	if err != nil {
		slog.Error("sendMarketsButtonsMenu: failed to send buttons message", "err", err)
	}
	return err
}

type selectListRow struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

type selectListSection struct {
	Title string          `json:"title"`
	Rows  []selectListRow `json:"rows"`
}

type selectListParams struct {
	Title    string              `json:"title"`
	Sections []selectListSection `json:"sections"`
}

func sendMarketsSelectListMenu(ctx *Context) error {
	msgVersion := int32(1)
	p := ctx.GetPrefix()
	slog.Debug("sendMarketsSelectListMenu: building single_select native flow list for mobile device", "chat", ctx.Chat.String(), "device", ctx.Sender.Device)

	// Fetch dynamic instrument list from Forex Factory API
	apiItems, err := fetchForexFactoryInstrumentList(ctx.Ctx)

	var majorRows []selectListRow
	var cryptoRows []selectListRow
	var commodityRows []selectListRow
	seenIDs := make(map[string]bool)

	if err == nil && len(apiItems) > 0 {
		slog.Info("sendMarketsSelectListMenu: dynamically populating menu from Forex Factory API", "total_api_items", len(apiItems))

		// Sort by Rank
		slices.SortFunc(apiItems, func(a, b FFListItem) int {
			if a.Rank == 0 {
				return 1
			}
			if b.Rank == 0 {
				return -1
			}
			return a.Rank - b.Rank
		})

		for _, item := range apiItems {
			name := item.DisplayName
			if name == "" {
				name = item.Name
			}
			if name == "" || seenIDs[name] {
				continue
			}

			title := item.Title
			if title == "" {
				title = name
			}

			row := selectListRow{
				ID:          fmt.Sprintf("%smarkets %s", p, name),
				Title:       name,
				Description: title,
			}

			upperName := strings.ToUpper(name)
			className := strings.ToUpper(item.InstrumentClassName)

			if className == "CRYPTO" || strings.HasPrefix(upperName, "BTC") || strings.HasPrefix(upperName, "ETH") {
				if len(cryptoRows) < 5 {
					cryptoRows = append(cryptoRows, row)
					seenIDs[name] = true
				}
			} else if strings.Contains(upperName, "GOLD") || strings.Contains(upperName, "SILVER") || strings.Contains(upperName, "OIL") {
				if len(commodityRows) < 5 {
					commodityRows = append(commodityRows, row)
					seenIDs[name] = true
				}
			} else if len(majorRows) < 7 {
				majorRows = append(majorRows, row)
				seenIDs[name] = true
			}
		}
	} else {
		slog.Warn("sendMarketsSelectListMenu: API fetch failed or empty, using fallback presets", "err", err)
	}

	// Fallback presets if API rows are empty
	if len(majorRows) == 0 {
		majorRows = []selectListRow{
			{ID: p + "markets EUR/USD", Title: "EUR/USD", Description: "Euro / US Dollar"},
			{ID: p + "markets GBP/USD", Title: "GBP/USD", Description: "British Pound / US Dollar"},
			{ID: p + "markets USD/JPY", Title: "USD/JPY", Description: "US Dollar / Japanese Yen"},
			{ID: p + "markets USD/CAD", Title: "USD/CAD", Description: "US Dollar / Canadian Dollar"},
			{ID: p + "markets AUD/USD", Title: "AUD/USD", Description: "Australian Dollar / US Dollar"},
		}
	}
	if len(cryptoRows) == 0 {
		cryptoRows = []selectListRow{
			{ID: p + "markets BTC/USD", Title: "BTC/USD", Description: "Bitcoin / US Dollar"},
			{ID: p + "markets ETH/USD", Title: "ETH/USD", Description: "Ethereum / US Dollar"},
		}
	}
	if len(commodityRows) == 0 {
		commodityRows = []selectListRow{
			{ID: p + "markets Gold/USD", Title: "Gold/USD", Description: "Spot Gold / US Dollar"},
			{ID: p + "markets Silver/USD", Title: "Silver/USD", Description: "Spot Silver / US Dollar"},
		}
	}

	paramsObj := selectListParams{
		Title: "Select Instrument",
		Sections: []selectListSection{
			{
				Title: "Forex Major Pairs",
				Rows:  majorRows,
			},
			{
				Title: "Cryptocurrencies",
				Rows:  cryptoRows,
			},
			{
				Title: "Metals & Commodities",
				Rows:  commodityRows,
			},
			{
				Title: "Overview",
				Rows: []selectListRow{
					{
						ID:          p + "markets all",
						Title:       "All Majors Summary",
						Description: "View rates summary for all major pairs",
					},
				},
			},
		},
	}

	paramsJSON, err := json.Marshal(paramsObj)
	if err != nil {
		slog.Error("sendMarketsSelectListMenu: failed to marshal params JSON", "err", err)
		return ctx.Reply("Failed to construct market menu.")
	}

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
									Name:             new("single_select"),
									ButtonParamsJSON: new(string(paramsJSON)),
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

	slog.Info("sendMarketsSelectListMenu: sending single_select list message", "chat", ctx.Chat.String())
	_, err = ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg, extra)
	if err != nil {
		slog.Error("sendMarketsSelectListMenu: failed to send list message", "err", err)
	}
	return err
}
