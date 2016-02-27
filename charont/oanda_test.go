package charont

import (
	"strings"
	"testing"
	"time"

	"github.com/alonsovidales/pit/cfg"
	"github.com/alonsovidales/pit/log"
)

func TestPlaceOrder(t *testing.T) {
	cfg.Init("v", "dev")

	api, err := InitOandaApi(
		cfg.GetStr("oanda", "endpoint"),
		cfg.GetStr("oanda", "token"),
		int(cfg.GetInt("oanda", "account-id")),
		strings.Split(cfg.GetStr("oanda", "currencies"), ","),
		cfg.GetStr("oanda", "exanges-log"),
	)

	if err != nil {
		t.Error("Problem connecting with oanda, Error:", err)
	}

	curr := api.GetBaseCurrency()
	if curr != "EUR" {
		t.Error("The configured value on the test account was EUR, but:", curr, "was returned")
	}

	currs := api.GetCurrencies()
	log.Debug(currs)

	order, err := api.Buy("USD", 1, 1.3, true, time.Now().Unix())
	if err != nil {
		t.Error("Problem placing an order, Error:", err)
	}

	err = api.CloseOrder(order, time.Now().Unix())
	if err != nil {
		t.Error("Problem closing an order, Error:", err)
	}

	order, err = api.Sell("USD", 1, 1.0, true, time.Now().Unix())
	if err != nil {
		t.Error("Problem placing an order, Error:", err)
	}

	err = api.CloseOrder(order, time.Now().Unix())
	if err != nil {
		t.Error("Problem closing an order, Error:", err)
	}

	order, err = api.Buy("USD", 1, 1.3, false, time.Now().Unix())
	if err != nil {
		t.Error("Problem placing an order, Error:", err)
	}

	err = api.CloseOrder(order, time.Now().Unix())
	if err != nil {
		t.Error("Problem closing an order, Error:", err)
	}

	order, err = api.Sell("USD", 1, 1.0, false, time.Now().Unix())
	if err != nil {
		t.Error("Problem placing an order, Error:", err)
	}

	err = api.CloseOrder(order, time.Now().Unix())
	if err != nil {
		t.Error("Problem closing an order, Error:", err)
	}
}
