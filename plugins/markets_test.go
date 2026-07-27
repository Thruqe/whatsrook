package commands

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMarketsCommandRegistration(t *testing.T) {
	cmd, ok := Get("markets")
	if !ok {
		t.Fatal("expected 'markets' command to be registered")
	}

	if cmd.Category != "info" {
		t.Errorf("expected category 'info', got %q", cmd.Category)
	}

	if !cmd.IsPublic {
		t.Error("expected markets command to be public")
	}

	foundAlias := false
	for _, a := range cmd.Aliases {
		if a == "forex" {
			foundAlias = true
			break
		}
	}
	if !foundAlias {
		t.Error("expected alias 'forex' to be registered")
	}
}

func TestUnmarshalFFInstrumentResponse(t *testing.T) {
	sampleJSON := `{
		"data": [
			{
				"instrument": {
					"id": 7,
					"name": "EUR/USD",
					"display_name": "EUR/USD",
					"decimals": 4,
					"is_in_holiday": false
				},
				"metrics": {
					"D1": {
						"price": 1.13768,
						"high": 1.14009,
						"low": 1.13648,
						"spread": 0.6
					}
				},
				"quotes": [
					{
						"instrument": "EUR/USD",
						"bid": 1.1371,
						"ask": 1.13728
					}
				]
			}
		]
	}`

	var res FFInstrumentResponse
	err := json.Unmarshal([]byte(sampleJSON), &res)
	if err != nil {
		t.Fatalf("failed to unmarshal FFInstrumentResponse: %v", err)
	}

	if len(res.Data) != 1 {
		t.Fatalf("expected 1 item, got %d", len(res.Data))
	}

	item := res.Data[0]
	if item.Instrument.DisplayName != "EUR/USD" {
		t.Errorf("expected display_name 'EUR/USD', got %q", item.Instrument.DisplayName)
	}

	if len(item.Quotes) == 0 || item.Quotes[0].Bid != 1.1371 {
		t.Errorf("unexpected bid price")
	}
}

func TestUnmarshalFFBarsResponse(t *testing.T) {
	sampleJSON := `{
		"data": [
			{
				"timestamp": 1784926800,
				"data_source_id": "MDSAgg",
				"interval": "M5",
				"instrument": "Dow/USD",
				"open": 40120.5,
				"high": 40150.0,
				"low": 40100.0,
				"close": 40145.2
			}
		]
	}`

	var res FFBarsResponse
	err := json.Unmarshal([]byte(sampleJSON), &res)
	if err != nil {
		t.Fatalf("failed to unmarshal FFBarsResponse: %v", err)
	}

	if len(res.Data) != 1 {
		t.Fatalf("expected 1 bar, got %d", len(res.Data))
	}

	bar := res.Data[0]
	if bar.Instrument != "Dow/USD" {
		t.Errorf("expected instrument 'Dow/USD', got %q", bar.Instrument)
	}
	if bar.Close != 40145.2 {
		t.Errorf("expected close price 40145.2, got %f", bar.Close)
	}
}

func TestInstrumentCategorization(t *testing.T) {
	items := []FFListItem{
		{ID: 1, DisplayName: "EUR/USD", InstrumentClassName: "Forex"},
		{ID: 2, DisplayName: "Gold/USD", InstrumentClassName: "Metals"},
		{ID: 3, DisplayName: "Silver/USD", InstrumentClassName: "Metals"},
		{ID: 4, DisplayName: "WTI/USD", InstrumentClassName: "Energy"},
		{ID: 5, DisplayName: "NatGas/USD", InstrumentClassName: "Energy"},
		{ID: 6, DisplayName: "BTC/USD", InstrumentClassName: "Crypto"},
		{ID: 7, DisplayName: "DOGE/USD", InstrumentClassName: "Crypto"},
		{ID: 8, DisplayName: "Dow", InstrumentClassName: "Equities"},
		{ID: 9, DisplayName: "SPX", InstrumentClassName: "Equities"},
		{ID: 10, DisplayName: "NDX", InstrumentClassName: "Equities"},
		{ID: 11, DisplayName: "DXY", InstrumentClassName: "Equities"},
	}

	var forexRows, cryptoRows, commodityRows, otherRows []FFListItem

	for _, item := range items {
		className := strings.ToUpper(item.InstrumentClassName)
		upperName := strings.ToUpper(item.DisplayName)

		if className == "CRYPTO" || strings.HasPrefix(upperName, "BTC") || strings.HasPrefix(upperName, "ETH") || strings.HasPrefix(upperName, "DOGE") {
			cryptoRows = append(cryptoRows, item)
		} else if className == "METALS" || className == "COMMODITIES" || className == "ENERGY" ||
			strings.Contains(upperName, "GOLD") || strings.Contains(upperName, "SILVER") ||
			strings.Contains(upperName, "OIL") || strings.Contains(upperName, "GAS") {
			commodityRows = append(commodityRows, item)
		} else if className == "EQUITIES" || className == "INDICES" || className == "INDEX" ||
			upperName == "DOW" || upperName == "SPX" || upperName == "NDX" || upperName == "NIKKEI225" ||
			upperName == "DAX" || upperName == "FTSE100" || upperName == "STOXX50" || upperName == "US2000" ||
			upperName == "VIX" || upperName == "DXY" || upperName == "CAC" || upperName == "ASX" {
			otherRows = append(otherRows, item)
		} else if className == "FOREX" || (strings.Contains(upperName, "/") && len(upperName) == 7) {
			forexRows = append(forexRows, item)
		} else {
			otherRows = append(otherRows, item)
		}
	}

	if len(forexRows) != 1 || forexRows[0].DisplayName != "EUR/USD" {
		t.Errorf("unexpected forex categorization: %+v", forexRows)
	}
	if len(commodityRows) != 4 {
		t.Errorf("expected 4 commodity items (Gold, Silver, WTI, NatGas), got %d", len(commodityRows))
	}
	if len(cryptoRows) != 2 {
		t.Errorf("expected 2 crypto items (BTC, DOGE), got %d", len(cryptoRows))
	}
	if len(otherRows) != 4 {
		t.Errorf("expected 4 equity/other items (Dow, SPX, NDX, DXY), got %d", len(otherRows))
	}
}
