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
	"strings"
)

const (
	trainingFile         = "../test_data/20150907.log"
	timeRangeToStudySecs = 3600 * 2 * 1000000000
)

type feedsStr struct {
	feeds map[string][]charont.CurrVal
}

func main() {
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
	for curr, prices := range fd.feeds {
		log.Debug("Studing currency:", curr)
		for i, price := range prices {
			for j := i + 1; j < len(prices) && price.Ts+timeRangeToStudySecs > prices[j].Ts; j++ {
				benef := prices[j].Bid/price.Ask - 1
				if benef > 0 {
					rangeSecs := float64(prices[j].Ts-price.Ts) / 1000000000

					if maxBenef < benef {
						maxBenef = benef
					}
					//log.Debug("Benef in time:", curr, i, j, "Secs:", rangeSecs, "=", benef, "T:", benef/rangeSecs, price, prices[j])
					// The benef by time is going to be the score
					benef /= rangeSecs
					if maxBenefTime < benef {
						maxBenefTime = benef
					}
				}
			}
		}
	}

	log.Debug("Max Benef:", maxBenef)
	log.Debug("Max Benef Time:", maxBenefTime)
}
