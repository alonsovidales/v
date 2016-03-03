package charont

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/alonsovidales/pit/log"
)

const (
	tsMultToSecs = 1000000000
)

type Mock struct {
	mutex          sync.Mutex
	feedsBySecond  int
	currencyValues map[string][]*CurrVal
	mutexCurr      map[string]*sync.Mutex
	openOrders     map[int64]*Order
	ordersByCurr   map[string][]*Order
	currLogsFile   *os.File
	listeners      map[string][]func(currency string, ts int64)
	orders         int64
	currencies     []string
	currentWin     float64
}

type currOpsInfo struct {
	Prices []*CurrVal
	Orders []*Order
}

func GetMock(feedsFile string, feedsBySecond int, currencies []string, httpPort int) (mock *Mock) {
	var err error

	mock = &Mock{
		orders:        0,
		currentWin:    0,
		feedsBySecond: feedsBySecond,
		mutexCurr:     make(map[string]*sync.Mutex),
		ordersByCurr:  make(map[string][]*Order),
		currencies:    currencies,
		listeners:     make(map[string][]func(currency string, ts int64)),
		openOrders:    make(map[int64]*Order),
	}

	for _, curr := range currencies {
		mock.ordersByCurr[curr] = []*Order{}
	}

	if feedsFile != "" {
		mock.currLogsFile, err = os.Open(feedsFile)
		if err != nil {
			log.Error("Currency logs file can't be open, Error:", err)
			return
		}
	}

	go mock.ratesCollector()

	http.HandleFunc("/get_curr_values_orders", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "application/json")
		curr := r.FormValue("curr")
		info, _ := json.Marshal(&currOpsInfo{
			Prices: mock.currencyValues[curr],
			Orders: mock.ordersByCurr[curr],
		})
		w.Write(info)
	})
	go http.ListenAndServe(fmt.Sprintf(":%d", httpPort), nil)
	log.Info("Mock HTTP server listening on:", httpPort)

	return
}

func (mock *Mock) GetBaseCurrency() string {
	return "EUR"
}

func (mock *Mock) GetCurrencies() []string {
	//return []string{"USD"}
	return mock.currencies
}

func (mock *Mock) GetAllCurrVals() (result map[string][]*CurrVal) {
	//mock.mutex.Lock()
	//defer mock.mutex.Unlock()

	result = make(map[string][]*CurrVal)

	for curr, values := range mock.currencyValues {
		result[curr] = values
	}

	return
}

func (mock *Mock) GetRange(curr string, from, to int64) []*CurrVal {
	mock.mutexCurr[curr].Lock()
	defer mock.mutexCurr[curr].Unlock()
	if len(mock.currencyValues[curr]) <= 2 {
		return nil
	}

	min := 0
	max := len(mock.currencyValues[curr])
	for min != max && max-min != 1 {
		c := ((max - min) / 2) + min
		if mock.currencyValues[curr][c].Ts > from {
			max = c
		} else {
			min = c
		}
	}

	fromPos := max
	if fromPos >= len(mock.currencyValues[curr]) {
		fromPos--
	}
	if mock.currencyValues[curr][fromPos].Ts >= from {
		fromPos = min
	}

	if to == -1 {
		return mock.currencyValues[curr][fromPos:]
	}

	min = fromPos
	max = len(mock.currencyValues[curr])
	for min != max && max-min != 1 {
		c := ((max - min) / 2) + min
		if mock.currencyValues[curr][c].Ts > to {
			max = c
		} else {
			min = c
		}
	}

	toPos := max
	if toPos >= len(mock.currencyValues[curr]) {
		toPos--
	}
	if mock.currencyValues[curr][toPos].Ts >= to {
		toPos = min
	}

	return mock.currencyValues[curr][fromPos:toPos]
}

func (mock *Mock) getCurrentRealProfit() (profit float64) {
	profit = 1
	for _, ord := range mock.openOrders {
		if ord.Real {
			profit *= ord.Profit
		}
	}

	return
}

func (mock *Mock) placeMarketOrder(inst string, units int, side string, price float64, realOps bool, ts int64) (order *Order, err error) {
	// TODO Place market order
	mock.mutex.Lock()
	defer mock.mutex.Unlock()
	orderID := mock.orders
	mock.orders++
	mock.openOrders[orderID] = &Order{
		Id:    orderID,
		Real:  realOps,
		Units: units,
		Curr:  inst,
		Type:  side,
		Open:  true,
		BuyTs: ts,
	}

	if side == "buy" {
		mock.openOrders[orderID].Price = price
	} else {
		mock.openOrders[orderID].CloseRate = price
	}
	return mock.openOrders[orderID], nil
}

func (mock *Mock) Buy(currency string, units int, bound float64, realOps bool, ts int64) (order *Order, err error) {
	return mock.placeMarketOrder(currency, units, "buy", bound, realOps, ts)
}

func (mock *Mock) Sell(currency string, units int, bound float64, realOps bool, ts int64) (order *Order, err error) {
	return mock.placeMarketOrder(currency, units, "sell", bound, realOps, ts)
}

func (mock *Mock) CloseOrder(ord *Order, ts int64) (err error) {
	var realOrder string

	currVals := mock.currencyValues[ord.Curr]
	if ord.Type == "buy" {
		ord.CloseRate = currVals[len(currVals)-1].Bid
	} else {
		ord.Price = currVals[len(currVals)-1].Ask
	}
	ord.Profit = ord.CloseRate/ord.Price - 1
	ord.SellTs = ts

	mock.mutex.Lock()
	mock.ordersByCurr[ord.Curr] = append(mock.ordersByCurr[ord.Curr], ord)

	delete(mock.openOrders, ord.Id)
	mock.mutex.Unlock()

	if ord.Real {
		mock.currentWin += ord.Profit * float64(ord.Units)
		realOrder = "Real"
	} else {
		realOrder = "Simultaion"
	}

	log.Debug("Closed Order:", ord.Id, "TypeOrd:", ord.Type, "BuyTs:", time.Unix(ord.BuyTs/tsMultToSecs, 0), "TimeToSell:", (ord.SellTs-ord.BuyTs)/tsMultToSecs, "Curr:", ord.Curr, "OpenRate:", ord.Price, "Close rate:", ord.CloseRate, "And Profit:", ord.Profit, "Current Win:", mock.currentWin, "Type:", realOrder)
	return
}

func (mock *Mock) CloseAllOpenOrders() {
	for ordId, _ := range mock.openOrders {
		mock.CloseOrder(mock.openOrders[ordId], 0)
	}
}

func (mock *Mock) ratesCollector() {
	mock.mutex.Lock()
	mock.currencyValues = make(map[string][]*CurrVal)
	for _, curr := range mock.currencies {
		mock.mutexCurr[curr] = new(sync.Mutex)
	}
	mock.mutex.Unlock()

	scanner := bufio.NewScanner(mock.currLogsFile)
	scanner.Scan()
	log.Info("Parsing currencies from the mock file...")

	c := time.Tick(time.Duration(1000/mock.feedsBySecond) * time.Millisecond)
	i := 0
	lastWinVal := mock.currentWin
	for _ = range c {
		var feed CurrVal

		scanner.Scan()
		lineParts := strings.SplitN(scanner.Text(), ":", 2)
		curr := lineParts[0]
		if err := json.Unmarshal([]byte(lineParts[1]), &feed); err != nil {
			log.Error("The feeds response body is not a JSON valid, Error:", err)
			continue
		}

		mock.mutexCurr[curr].Lock()
		//log.Debug("New price for currency:", curr, "Bid:", feed.Bid, "Ask:", feed.Ask)
		mock.currencyValues[curr] = append(mock.currencyValues[curr], &CurrVal{
			Ts:  feed.Ts,
			Bid: feed.Bid,
			Ask: feed.Ask,
		})

		if listeners, ok := mock.listeners[curr]; ok {
			for _, listener := range listeners {
				go listener(curr, feed.Ts)
			}
		}
		if len(mock.currencyValues[curr]) > MAX_RATES_TO_STORE {
			mock.currencyValues[curr] = mock.currencyValues[curr][1:]
		}
		mock.mutexCurr[curr].Unlock()

		if lastWinVal != mock.currentWin {
			log.Info("CurrentWin:", i, mock.currentWin)
			lastWinVal = mock.currentWin
		}
		i++
	}
}

func (mock *Mock) AddListerner(currency string, fn func(currency string, ts int64)) {
	mock.mutex.Lock()
	if _, ok := mock.listeners[currency]; !ok {
		mock.listeners[currency] = []func(currency string, ts int64){}
	}
	mock.listeners[currency] = append(mock.listeners[currency], fn)
	mock.mutex.Unlock()
}
