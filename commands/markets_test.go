package commands

import (
	"encoding/json"
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
