package charont

import (
	"github.com/alonsovidales/pit/log"
	"sync"
)

const (
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
	collectionMutex sync.Mutex
	authToken       string
}

func InitOandaApi(authToken string, accountId int, currencies []string) (api *Oanda, err error) {
	var resp *http.Response

	api = &Oanda{
		authToken: authToken,
	}

	if accountId == -1 {
		var accInfo map[string]interface{}

		resp, err = http.PostForm(FAKE_GENERATE_ACCOUNT_URL, nil)
		if err != nil {
			return
		}
		body, errRead := ioutil.ReadAll(resp.Body)
		if errRead != nil {
			err = errRead
			return
		}

		err = json.Unmarshal(body, &accInfo)
		if err != nil {
			return
		}
		resp, err = api.doRequest("GET", ACCOUNT_INFO_URL, nil)
		pass = accInfo["password"].(string)

		log.Info("New account generated:", int(accInfo["accountId"].(float64)))
	} else {
		resp, err = api.doRequest("GET", fmt.Sprintf("%s%d", ACCOUNT_INFO_URL, accountId), nil)
	}

	if err != nil {
		return
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}

	err = json.Unmarshal(body, &api.acc)
	if err != nil {
		return
	}
	api.curr = curr

	go api.ratesCollector()

	return
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

	body, err := ioutil.ReadAll(resp.Body)

	return
}
