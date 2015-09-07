package charont

import (
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/alonsovidales/pit/log"
	"io/ioutil"
	"os"
	"strings"
	"sync"
	"time"
)

type Mock struct {
	mutex          sync.Mutex
	feedsBySecond  int
	currencyValues map[string][]*CurrVal
	mutexCurr      map[string]*sync.Mutex
	openOrders     map[int64]*Order
	currLogsFile   *os.File
	listeners      map[string][]func(currency string)
	orders         int64
}

func GetMock(feedsFile string, feedsBySecond int) (mock *Mock) {
	var err error

	mock = &Mock{
		orders:        0,
		feedsBySecond: feedsBySecond,
		mutexCurr:     make(map[string]*sync.Mutex),
	}

	if feedsFile != "" {
		mock.currLogsFile, err = os.Open(feedsFile)
		if err != nil {
			log.Error("Currency logs file can't be open, Error:", err)
			return
		}
	}
	mock.mutex.Lock()

	go mock.ratesCollector()

	return
}

func (mock *Mock) GetBaseCurrency() string {
	return "EUR"
}

func (mock *Mock) GetCurrencies() []string {
	return []string{}
}

func (mock *Mock) GetRange(curr string, from, to int64) []*CurrVal {
	mock.mutexCurr[curr].Lock()
	defer mock.mutexCurr[curr].Unlock()

	if len(mock.currencyValues[curr]) <= 1 {
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
	if mock.currencyValues[curr][toPos].Ts >= to {
		toPos = min
	}

	return mock.currencyValues[curr][fromPos:toPos]
}

func (mock *Mock) placeMarketOrder(inst string, units int, side string, price float64) (order *Order, err error) {
	// TODO Place market order
	orderID := mock.orders
	mock.orders++
	mock.openOrders[orderID] = &Order{
		Id:    orderID,
		Price: price,
		Units: units,
		Curr:  inst,
		Type:  side,
		Open:  true,
	}

	return mock.openOrders[orderID], nil
}

func (mock *Mock) Buy(currency string, units int, bound float64) (order *Order, err error) {
	return mock.placeMarketOrder(currency, units, "buy", bound)
}

func (mock *Mock) Sell(currency string, units int, bound float64) (order *Order, err error) {
	return mock.placeMarketOrder(currency, units, "sell", bound)
}

func (mock *Mock) CloseOrder(ord *Order) (err error) {
	currVals := mock.currencyValues[ord.Curr]
	ord.CloseRate = currVals[len(currVals)-1].Bid
	ord.Profit = ord.CloseRate / ord.Price
	log.Debug("Closed Order:", ord.Id, "With rate:", ord.CloseRate, "And Profit:", ord.Profit)
	mock.mutex.Lock()
	delete(mock.openOrders, ord.Id)
	mock.mutex.Unlock()

	return
}

func (mock *Mock) CloseAllOpenOrders() {
	for ordId, _ := range mock.openOrders {
		mock.CloseOrder(mock.openOrders[ordId])
	}
}

func (mock *Mock) ratesCollector() {
	var feeds map[string][]feedStruc

	mock.mutex.Lock()
	mock.currencyValues = make(map[string][]*CurrVal)
	mock.mutex.Unlock()

	scanner := bufio.NewScanner(mock.currLogsFile)
	log.Info("Parsing currencies from the mock file...")

	c := time.Tick(time.Duration(1000/mock.feedsBySecond) * time.Millisecond)
	for _ = range c {
		var cv CurrVal

		scanner.Scan()
		lineParts := strings.Split(scanner.Text(), ":")
		curr := lineParts[0]
		if err := json.Unmarshal([]byte(lineParts[1]), &cv); err != nil {
			log.Error("The feeds response body is not a JSON valid, Error:", err)
			continue
		}

		// Ok, all fine, we are going to parse the feeds
		for _, feed := range feeds["prices"] {
			mock.mutexCurr[curr].Lock()
			log.Debug("New price for currency:", curr, "Bid:", feed.Bid, "Ask:", feed.Ask)
			mock.currencyValues[curr] = append(mock.currencyValues[curr], &CurrVal{
				Ts:  time.Now().UnixNano(),
				Bid: feed.Bid,
				Ask: feed.Ask,
			})

			if listeners, ok := mock.listeners[curr]; ok {
				for _, listener := range listeners {
					listener(curr)
				}
			}
			if len(mock.currencyValues[curr]) > MAX_RATES_TO_STORE {
				mock.currencyValues[curr] = mock.currencyValues[curr][1:]
			}
			mock.mutexCurr[curr].Unlock()
		}
	}
}

func (mock *Mock) AddListerner(currency string, fn func(currency string)) {
	mock.mutex.Lock()
	if _, ok := mock.listeners[currency]; !ok {
		mock.listeners[currency] = []func(currency string){}
	}
	mock.listeners[currency] = append(mock.listeners[currency], fn)
	mock.mutex.Unlock()
}
