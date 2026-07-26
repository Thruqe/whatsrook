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
	}

	slog.Info("handleMarkets: querying single instrument", "pair", queryArg)
	return fetchAndSendSingleMarket(ctx, queryArg)
}

func fetchForexFactoryInstrumentList(ctx context.Context) ([]FFListItem, error) {
	client := &http.Client{Timeout: 8 * time.Second}
	var allItems []FFListItem

	// 1. Fetch main instrument list from Forex Factory API
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

	// 2. Fetch synthetic / crypto instrument list from Forex Factory API
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

	slog.Debug("fetchForexFactoryInstrumentList: retrieved total instruments from APIs", "count", len(allItems))
	return allItems, nil
}

func fetchAndSendSingleMarket(ctx *Context, pair string) error {
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

	// If HTTP 404 or non-200 occurs, fetch live instrument list from API to display valid choices
	if resp.StatusCode != http.StatusOK {
		slog.Warn("fetchAndSendSingleMarket: HTTP non-200 status from API", "pair", pair, "status", resp.StatusCode)
		return sendAvailableInstrumentsList(ctx, pair)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("fetchAndSendSingleMarket: failed to read response body", "pair", pair, "err", err)
		return ctx.Reply("Failed to read market API response.")
	}

	var res FFInstrumentResponse
	if err := json.Unmarshal(body, &res); err != nil || len(res.Data) == 0 {
		slog.Warn("fetchAndSendSingleMarket: empty data returned from API", "pair", pair, "err", err, "data_len", len(res.Data))
		return sendAvailableInstrumentsList(ctx, pair)
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

func sendAvailableInstrumentsList(ctx *Context, requestedPair string) error {
	p := ctx.GetPrefix()
	apiItems, err := fetchForexFactoryInstrumentList(ctx.Ctx)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Instrument %q is not available on Forex Factory.\n\nAvailable Active Markets:\n", requestedPair))

	if err == nil && len(apiItems) > 0 {
		seen := make(map[string]bool)
		count := 0
		for _, item := range apiItems {
			name := item.DisplayName
			if name == "" {
				name = item.Name
			}
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			fmt.Fprintf(&sb, "- %smarkets %s\n", p, name)
			count++
			if count >= 12 {
				break
			}
		}
	} else {
		fmt.Fprintf(&sb, "- %smarkets EUR/USD\n- %smarkets GBP/USD\n- %smarkets USD/JPY\n- %smarkets Gold/USD\n", p, p, p, p)
	}

	fmt.Fprintf(&sb, "\nView all summary:\n- %smarkets all", p)
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

	// Fetch live API instruments to populate buttons dynamically
	apiItems, _ := fetchForexFactoryInstrumentList(ctx.Ctx)

	var btn1, btn2, btn3 string
	if len(apiItems) >= 3 {
		btn1 = apiItems[0].DisplayName
		btn2 = apiItems[1].DisplayName
		btn3 = apiItems[2].DisplayName
	} else {
		btn1 = "EUR/USD"
		btn2 = "GBP/USD"
		btn3 = "Gold/USD"
	}

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
							ButtonID: new(p + "markets " + btn1),
							ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
								DisplayText: new(btn1),
							},
							Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
						},
						{
							ButtonID: new(p + "markets " + btn2),
							ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
								DisplayText: new(btn2),
							},
							Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
						},
						{
							ButtonID: new(p + "markets " + btn3),
							ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
								DisplayText: new(btn3),
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

	// Fetch live instrument list directly from Forex Factory API endpoints
	apiItems, err := fetchForexFactoryInstrumentList(ctx.Ctx)
	if err != nil || len(apiItems) == 0 {
		slog.Error("sendMarketsSelectListMenu: failed to fetch live instruments from Forex Factory API", "err", err)
		return ctx.Reply("Failed to fetch market instruments from Forex Factory. Please try again later.")
	}

	slog.Info("sendMarketsSelectListMenu: fetched live instruments from Forex Factory API", "total_items", len(apiItems))

	// Sort items by rank (or ID if rank is 0)
	slices.SortFunc(apiItems, func(a, b FFListItem) int {
		if a.Rank == 0 && b.Rank == 0 {
			return a.ID - b.ID
		}
		if a.Rank == 0 {
			return 1
		}
		if b.Rank == 0 {
			return -1
		}
		return a.Rank - b.Rank
	})

	var forexRows []selectListRow
	var cryptoRows []selectListRow
	var commodityRows []selectListRow
	var otherRows []selectListRow

	seenNames := make(map[string]bool)

	for _, item := range apiItems {
		name := strings.TrimSpace(item.DisplayName)
		if name == "" {
			name = strings.TrimSpace(item.Name)
		}
		if name == "" || seenNames[strings.ToUpper(name)] {
			continue
		}
		seenNames[strings.ToUpper(name)] = true

		title := strings.TrimSpace(item.Title)
		if title == "" {
			title = name
		}

		row := selectListRow{
			ID:          fmt.Sprintf("%smarkets %s", p, name),
			Title:       name,
			Description: title,
		}

		className := strings.ToUpper(strings.TrimSpace(item.InstrumentClassName))
		upperName := strings.ToUpper(name)

		if className == "CRYPTO" || strings.HasPrefix(upperName, "BTC") || strings.HasPrefix(upperName, "ETH") {
			if len(cryptoRows) < 10 {
				cryptoRows = append(cryptoRows, row)
			}
		} else if strings.Contains(upperName, "GOLD") || strings.Contains(upperName, "SILVER") || strings.Contains(upperName, "OIL") {
			if len(commodityRows) < 10 {
				commodityRows = append(commodityRows, row)
			}
		} else if className == "FOREX" || strings.Contains(upperName, "/") {
			if len(forexRows) < 10 {
				forexRows = append(forexRows, row)
			}
		} else {
			if len(otherRows) < 5 {
				otherRows = append(otherRows, row)
			}
		}
	}

	var sections []selectListSection

	if len(forexRows) > 0 {
		sections = append(sections, selectListSection{
			Title: "Forex Currency Pairs",
			Rows:  forexRows,
		})
	}
	if len(commodityRows) > 0 {
		sections = append(sections, selectListSection{
			Title: "Metals & Commodities",
			Rows:  commodityRows,
		})
	}
	if len(cryptoRows) > 0 {
		sections = append(sections, selectListSection{
			Title: "Cryptocurrencies",
			Rows:  cryptoRows,
		})
	}
	if len(otherRows) > 0 {
		sections = append(sections, selectListSection{
			Title: "Other Instruments",
			Rows:  otherRows,
		})
	}

	sections = append(sections, selectListSection{
		Title: "Market Summary",
		Rows: []selectListRow{
			{
				ID:          p + "markets all",
				Title:       "All Major Markets",
				Description: "View live summary for all major market pairs",
			},
		},
	})

	paramsObj := selectListParams{
		Title:    "Select Market Instrument",
		Sections: sections,
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
						Text: new("Forex Factory Market Rates\n\nClick the button below to choose an active instrument:"),
					},
					Footer: &waE2E.InteractiveMessage_Footer{
						Text: new("Live Forex Factory Data"),
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

	slog.Info("sendMarketsSelectListMenu: sending live API single_select list message", "chat", ctx.Chat.String())
	_, err = ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg, extra)
	if err != nil {
		slog.Error("sendMarketsSelectListMenu: failed to send list message", "err", err)
	}
	return err
}
