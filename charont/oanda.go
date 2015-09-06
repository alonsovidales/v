package charont

import (
	"encoding/json"
	"fmt"
	"github.com/alonsovidales/pit/log"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	MAX_RATES_TO_STORE        = 10000
	COLLECT_BY_SECOND         = 3
	FAKE_GENERATE_ACCOUNT_URL = "https://api-fxpractice.oanda.com/v1/accounts"
	ACCOUNT_INFO_URL          = "https://api-fxpractice.oanda.com/v1/accounts/"
	PLACE_ORDER_URL           = "https://api-fxpractice.oanda.com/v1/accounts/%d/orders"
	FEEDS_URL                 = "https://api-fxpractice.oanda.com/v1/prices?instruments="
	CHECK_ORDER_URL           = "https://api-fxpractice.oanda.com/v1/accounts/%d/trades/%d"
)

type feedStruc struct {
	Instrument string  `json:"instrument"`
	Time       string  `json:"time"`
	Bid        float64 `json:"bid"`
	Ask        float64 `json:"ask"`
}

type orderInfoStruc struct {
	Id int64 `json:"id"`
}
type orderStruc struct {
	Time  string          `json:"time"`
	Price float64         `json:"price"`
	Info  *orderInfoStruc `json:"tradeOpened"`
}

type accountStruc struct {
	AccountId       int     `json:"accountId"`
	AccountName     string  `json:"accountName"`
	Balance         float64 `json:"balance"`
	UnrealizedPl    float64 `json:"unrealizedPl"`
	RealizedPl      float64 `json:"realizedPl"`
	MarginUsed      float64 `json:"marginUsed"`
	MarginAvail     float64 `json:"marginAvail"`
	OpenTrades      float64 `json:"openTrades"`
	OpenOrders      float64 `json:"openOrders"`
	MarginRate      float64 `json:"marginRate"`
	AccountCurrency string  `json:"accountCurrency"`
	Pass            string
}

type Oanda struct {
	mutex          sync.Mutex
	authToken      string
	currencies     []string
	currencyValues map[string][]*CurrVal
	account        *accountStruc
	mutexCurr      map[string]*sync.Mutex
	openOrders     map[int64]*Order
	currLogsFile   *os.File
}

func InitOandaApi(authToken string, accountId int, currencies []string, currLogsFile string) (api *Oanda, err error) {
	var resp []byte

	api = &Oanda{
		authToken:  authToken,
		currencies: currencies,
		mutexCurr:  make(map[string]*sync.Mutex),
	}

	if currLogsFile != "" {
		api.currLogsFile, err = os.Create(currLogsFile)
		if err != nil {
			log.Error("Currency logs file can't be open, Error:", err)
			return
		}
	}
	api.mutex.Lock()

	if accountId == -1 {
		var accInfo map[string]interface{}

		respHttp, err := http.PostForm(FAKE_GENERATE_ACCOUNT_URL, nil)
		if err != nil {
			return nil, err
		}
		body, err := ioutil.ReadAll(respHttp.Body)
		if err != nil {
			return nil, err
		}

		err = json.Unmarshal(body, &accInfo)
		if err != nil {
			return nil, err
		}
		resp, err = api.doRequest("GET", ACCOUNT_INFO_URL, nil)

		log.Info("New account generated:", int(accInfo["accountId"].(float64)))
	} else {
		resp, err = api.doRequest("GET", fmt.Sprintf("%s%d", ACCOUNT_INFO_URL, accountId), nil)
	}

	if err != nil {
		return
	}

	err = json.Unmarshal(resp, &api.account)
	if err != nil {
		return
	}

	api.mutex.Unlock()
	go api.ratesCollector()

	return
}

func (api *Oanda) GetBaseCurrency() string {
	return api.account.AccountCurrency
}

func (api *Oanda) GetCurrencies() []string {
	return api.currencies
}

func (api *Oanda) GetRange(curr string, from, to int64) []*CurrVal {
	api.mutexCurr[curr].Lock()
	defer api.mutexCurr[curr].Unlock()

	if len(api.currencyValues[curr]) <= 1 {
		return nil
	}

	min := 0
	max := len(api.currencyValues[curr])
	for min != max && max-min != 1 {
		c := ((max - min) / 2) + min
		if api.currencyValues[curr][c].Ts > from {
			max = c
		} else {
			min = c
		}
	}

	fromPos := max
	if api.currencyValues[curr][fromPos].Ts >= from {
		fromPos = min
	}

	if to == -1 {
		return api.currencyValues[curr][fromPos:]
	}

	min = fromPos
	max = len(api.currencyValues[curr])
	for min != max && max-min != 1 {
		c := ((max - min) / 2) + min
		if api.currencyValues[curr][c].Ts > to {
			max = c
		} else {
			min = c
		}
	}

	toPos := max
	if api.currencyValues[curr][toPos].Ts >= to {
		toPos = min
	}

	return api.currencyValues[curr][fromPos:toPos]
}

func (api *Oanda) placeMarketOrder(inst string, units int, side string, price float64) (order *Order, err error) {
	var orderInfo orderStruc
	var bound string

	if side == "sell" {
		bound = "lowerBound"
	} else {
		bound = "upperBound"
	}

	resp, err := api.doRequest("POST", fmt.Sprintf(PLACE_ORDER_URL, api.account.AccountId),
		url.Values{
			"instrument": {inst},
			"units":      {fmt.Sprintf("%d", int(units))},
			"side":       {side},
			"type":       {"market"},
			bound:        {fmt.Sprintf("%f", price)},
		})

	if err != nil {
		return
	}

	err = json.Unmarshal(resp, &orderInfo)
	if err != nil || orderInfo.Info == nil {
		log.Error("The response from the server to place an order can't be parsed, Error:", err)
		return
	}
	log.Debug("Values: instrument:", inst, "units", units, "side:", side, "type: market ID:", orderInfo, "\nOrder response:", string(resp))

	order = &Order{
		Id:    orderInfo.Info.Id,
		Price: orderInfo.Price,
		Open:  true,
		Type:  side,
	}

	api.mutex.Lock()
	api.openOrders[order.Id] = order
	api.mutex.Unlock()

	return
}

func (api *Oanda) Buy(currency string, units int, bound float64) (order *Order, err error) {
	return api.placeMarketOrder(currency, units, "buy", bound)
}

func (api *Oanda) Sell(currency string, units int, bound float64) (order *Order, err error) {
	return api.placeMarketOrder(currency, units, "sell", bound)
}

func (api *Oanda) CloseOrder(ord *Order) (err error) {
	resp, err := api.doRequest("DELETE", fmt.Sprintf(CHECK_ORDER_URL, api.account.AccountId, ord.Id), nil)
	if err != nil {
		log.Error("Problem trying to close an open position, Error:", err)
		return
	}
	generic := map[string]float64{}
	json.Unmarshal(resp, &generic)

	ord.CloseRate = generic["price"]
	ord.Profit = generic["profit"]
	log.Debug("Closed Order:", ord.Id, "With rate:", ord.CloseRate, "And Profit:", ord.Profit)
	api.mutex.Lock()
	delete(api.openOrders, ord.Id)
	api.mutex.Unlock()

	return
}

func (api *Oanda) CloseAllOpenOrders() {
	for ordId, _ := range api.openOrders {
		api.CloseOrder(api.openOrders[ordId])
	}
}

func (api *Oanda) ratesCollector() {
	var feeds map[string][]feedStruc

	api.mutex.Lock()
	api.currencyValues = make(map[string][]*CurrVal)
	currExange := make([]string, len(api.currencies))
	lasCurrPriceA := make(map[string]float64)
	lasCurrPriceB := make(map[string]float64)

	log.Debug("Curr:", api.currencies)
	for i, curr := range api.currencies {
		api.currencyValues[curr] = []*CurrVal{}
		currExange[i] = fmt.Sprintf("%s_%s", api.account.AccountCurrency, curr)
		lasCurrPriceA[curr] = 0
		lasCurrPriceB[curr] = 0
		api.mutexCurr[curr] = new(sync.Mutex)
	}
	api.mutex.Unlock()

	feedsUrl := FEEDS_URL + strings.Join(currExange, "%2C")
	log.Info("Parsing currencies from the feeds URL:", feedsUrl)

	c := time.Tick((1000 / COLLECT_BY_SECOND) * time.Millisecond)
	for _ = range c {
		resp, err := api.doRequest("GET", feedsUrl, nil)
		if err != nil {
			log.Error("The feeds URL can't be parsed, Error:", err)
			continue
		}

		if err = json.Unmarshal(resp, &feeds); err != nil {
			log.Error("The feeds response body is not a JSON valid, Error:", err, "Resp:", string(resp))
			continue
		}

		// Ok, all fine, we are going to parse the feeds
		for _, feed := range feeds["prices"] {
			curr := feed.Instrument[len(api.account.AccountCurrency)+1:]
			if lasCurrPriceA[curr] != feed.Ask || lasCurrPriceB[curr] != feed.Bid {
				api.mutexCurr[curr].Lock()
				log.Debug("New price for currency:", curr, "Bid:", feed.Bid, "Ask:", feed.Ask)
				api.currencyValues[curr] = append(api.currencyValues[curr], &CurrVal{
					Ts:  time.Now().UnixNano(),
					Bid: feed.Bid,
					Ask: feed.Ask,
				})

				if api.currLogsFile != nil {
					api.mutex.Lock()
					b, _ := json.Marshal(api.currencyValues[curr][len(api.currencyValues[curr])-1])
					_, err := api.currLogsFile.WriteString(fmt.Sprintf("%s:%s\n", curr, string(b)))
					if err != nil {
						log.Error("Can't write into the currencies logs file, Error:", err)
					}
					api.mutex.Unlock()
				}

				if len(api.currencyValues[curr]) > MAX_RATES_TO_STORE {
					api.currencyValues[curr] = api.currencyValues[curr][1:]
				}
				api.mutexCurr[curr].Unlock()
				lasCurrPriceA[curr] = feed.Ask
				lasCurrPriceB[curr] = feed.Bid
			}
		}
	}
}

func (api *Oanda) doRequest(method string, url string, data url.Values) (body []byte, err error) {
	var req *http.Request
	client := &http.Client{}

	if data != nil {
		req, err = http.NewRequest(method, url, strings.NewReader(data.Encode()))
	} else {
		req, err = http.NewRequest(method, url, nil)
	}
	if err != nil {
		return
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Authorization", "Bearer "+api.authToken)
	resp, err := client.Do(req)
	if err != nil {
		return
	}

	body, err = ioutil.ReadAll(resp.Body)

	return
}
