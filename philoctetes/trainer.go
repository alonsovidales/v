package main

/**
 * Info to get by currency:
 * 	- Best window size
 *	- Best price accoring to the window average to buy and to sell
 *	- Best price variation to buy and sell
 */

import (
	"bufio"
	"encoding/json"
	"github.com/alonsovidales/pit/log"
	"github.com/alonsovidales/v/charont"
	"os"
	"runtime"
	"strings"
)

const (
	trainingFile         = "../test_data/20150907.log"
	timeRangeToStudySecs = 3600 * 2 * 1000000000
	windowSize           = 100
)

type result struct {
	profitByTime float64
	profit       float64
	time         int64
	threndOnBuy  float64
	threndOnSell float64
	averageBuy   float64
	averageSell  float64
	priceOnBuy   float64
	priceOnSell  float64
}

type feedsStr struct {
	feeds map[string][]charont.CurrVal
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	feedsFile, err := os.Open(trainingFile)
	if err != nil {
		log.Fatal("Problem reading the logs file")
	}

	scanner := bufio.NewScanner(feedsFile)

	feeds := &feedsStr{
		feeds: make(map[string][]charont.CurrVal),
	}

	i := 0
	for {
		var feed charont.CurrVal

		if !scanner.Scan() {
			break
		}
		lineParts := strings.SplitN(scanner.Text(), ":", 2)
		curr := lineParts[0]
		if err := json.Unmarshal([]byte(lineParts[1]), &feed); err != nil {
			log.Error("The feeds response body is not a JSON valid, Error:", err)
			continue
		}

		if _, ok := feeds.feeds[curr]; !ok {
			feeds.feeds[curr] = []charont.CurrVal{}
		}
		feeds.feeds[curr] = append(feeds.feeds[curr], feed)
		i++
		if i%10000 == 0 {
			log.Debug("Lines:", i)
		}
	}

	feeds.studyCurrencies()
}

func (fd *feedsStr) studyCurrencies() {
	log.Debug("Studing currencies...")
	maxBenef := 0.0
	maxBenefTime := 0.0
	resultsByCurrency := make(map[string][]*result)
	for curr, prices := range fd.feeds {
		resultsByCurrency[curr] = []*result{}
		log.Debug("Studing currency:", curr)
		for i, price := range prices {

			threndOnBuy := 0.0
			avgOnBuy := 0.0
			first := true
			var prevPrice charont.CurrVal
			from := i - windowSize
			if from < 0 {
				from = 0
			}
			windowRange := prices[from:i]
			for _, priceWindow := range windowRange {
				avgOnBuy += priceWindow.Ask
				if !first {
					threndOnBuy += priceWindow.Ask / prevPrice.Ask
					prevPrice = priceWindow
				}
				first = false
			}
			threndOnBuy /= float64(len(windowRange) - 1)
			avgOnBuy /= float64(len(windowRange))

			for j := i + 1; j < len(prices) && price.Ts+timeRangeToStudySecs > prices[j].Ts; j++ {
				benef := prices[j].Bid/price.Ask - 1
				if benef > 0 {
					rangeSecs := float64(prices[j].Ts-price.Ts) / 1000000000

					if maxBenef < benef {
						maxBenef = benef
					}
					//log.Debug("Benef in time:", curr, i, j, "Secs:", rangeSecs, "=", benef, "T:", benef/rangeSecs, price, prices[j])
					// The benef by time is going to be the score

					benefTime := benef / rangeSecs
					if maxBenefTime < benef {
						maxBenefTime = benef
					}

					threndOnSell := 0.0
					avgOnSell := 0.0
					first := true
					var prevPrice charont.CurrVal
					fromSell := j - windowSize
					if fromSell < 0 {
						fromSell = 0
					}
					windowRange := prices[fromSell:j]
					for _, priceWindow := range windowRange {
						avgOnSell += priceWindow.Bid
						if !first {
							threndOnSell += priceWindow.Bid / prevPrice.Bid
							prevPrice = priceWindow
						}
						first = false
					}
					threndOnSell /= float64(len(windowRange) - 1)
					avgOnSell /= float64(len(windowRange))

					resultsByCurrency[curr] = append(resultsByCurrency[curr], &result{
						profit:       benef,
						profitByTime: benefTime,
						time:         int64(rangeSecs) * 1000000000,
						priceOnBuy:   price.Ask,
						priceOnSell:  prices[j].Bid,
						threndOnBuy:  threndOnBuy,
						averageBuy:   avgOnBuy,
						threndOnSell: threndOnSell,
						averageSell:  avgOnSell,
					})
				}
			}
		}
	}

	log.Debug("Max Benef:", maxBenef)
	log.Debug("Max Benef Time:", maxBenefTime)
}
