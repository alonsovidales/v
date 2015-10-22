package philoctetes

/**
 * Info to get by currency:
 * 	- Best window size
 *	- Best price accoring to the window average to buy and to sell
 *	- Best price variation to buy and sell
 *
 *	// TODO: Do something with the previous value???
 *	// TODO: Study by time range
 */

import (
	"bufio"
	"encoding/json"
	"github.com/alonsovidales/go_ml"
	"github.com/alonsovidales/pit/log"
	"github.com/alonsovidales/v/charont"
	"math"
	"os"
	"strings"
	"time"
)

const (
	cToStart = 100
)

type Trainer struct {
	feeds            map[string][]charont.CurrVal
	windowSize       int
	logRegModelsBuy  map[string]*ml.Regression
	logRegModelsSell map[string]*ml.Regression
}

func GetTrainer(trainingFile string, TimeRangeToStudySecs int64, windowSize int) (feeds *Trainer) {
	log.Debug("Initializing trainer...")

	TimeRangeToStudySecs *= 1000000000
	feedsFile, err := os.Open(trainingFile)
	if err != nil {
		log.Fatal("Problem reading the logs file")
	}

	scanner := bufio.NewScanner(feedsFile)

	feeds = &Trainer{
		feeds:            make(map[string][]charont.CurrVal),
		windowSize:       windowSize,
		logRegModelsBuy:  make(map[string]*ml.Regression),
		logRegModelsSell: make(map[string]*ml.Regression),
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

	feeds.studyCurrencies(TimeRangeToStudySecs)

	return
}

func (fd *Trainer) studyCurrencies(TimeRangeToStudySecs int64) {
	log.Debug("Studing currencies...")
	toFinish := len(fd.feeds)
	for curr, prices := range fd.feeds {
		// Prepare the dataset for the logixtic regression estimation
		fd.logRegModelsBuy[curr] = &ml.Regression{
			LinearReg: false,
			X:         [][]float64{},
			Y:         []float64{},
		}
		fd.logRegModelsSell[curr] = &ml.Regression{
			LinearReg: false,
			X:         [][]float64{},
			Y:         []float64{},
		}
		go func(curr string, prices []charont.CurrVal) {
			log.Debug("Studing currency:", curr)
			studiesPointsSell := make(map[int]bool)

			for i, price := range prices[cToStart:] {
				maxBenef := math.Inf(-1)
				maxBenefPoint := 0
				minBenef := math.Inf(1)
				minBenefPoint := 0
				for j := i + 1; j < len(prices) && price.Ts+TimeRangeToStudySecs > prices[j].Ts; j++ {
					//benefTime := (price.Bid / prices[j].Ask) / (float64(prices[j].Ts-price.Ts) / 1000000000)
					benefTime := (price.Bid / prices[j].Ask)

					if benefTime < minBenef {
						minBenef = benefTime
						minBenefPoint = j
					}
					if benefTime > maxBenef {
						maxBenef = benefTime
						maxBenefPoint = j
					}
				}

				if _, ok := studiesPointsSell[minBenefPoint]; ok {
					continue
				}
				if _, ok := studiesPointsSell[maxBenefPoint]; ok {
					continue
				}
				studiesPointsSell[minBenefPoint] = true
				studiesPointsSell[maxBenefPoint] = true

				askThrend, askAvg, askMin, askMax, askVar, askCovar := fd.GetInfoSection(prices[i:cToStart+i], true)
				sellThrend, sellAvg, sellkMin, sellMax, sellVar, sellCovar := fd.GetInfoSection(prices[maxBenefPoint:cToStart+maxBenefPoint], false)
				minSellThrend, minSellAvg, minSellkMin, minSellMax, minSellVar, minSellCovar := fd.GetInfoSection(prices[minBenefPoint:cToStart+minBenefPoint], false)
				fd.logRegModelsBuy[curr].X = append(
					fd.logRegModelsBuy[curr].X,
					[]float64{
						price.Ask / askAvg, askThrend, askAvg, askMin, askMax, askVar, askCovar,
					},
				)
				fd.logRegModelsSell[curr].X = append(
					fd.logRegModelsSell[curr].X,
					[]float64{
						price.Ask / askAvg, askThrend, askAvg, askMin, askMax, askVar, askCovar,
						price.Bid / sellAvg, sellThrend, sellAvg, sellkMin, sellMax, sellVar, sellCovar,
					},
				)
				fd.logRegModelsSell[curr].Y = append(
					fd.logRegModelsSell[curr].Y,
					1,
				)
				fd.logRegModelsSell[curr].X = append(
					fd.logRegModelsSell[curr].X,
					[]float64{
						price.Ask / askAvg, askThrend, askAvg, askMin, askMax, askVar, askCovar,
						price.Bid / minSellAvg, minSellThrend, minSellAvg, minSellkMin, minSellMax, minSellVar, minSellCovar,
					},
				)
				fd.logRegModelsSell[curr].Y = append(
					fd.logRegModelsSell[curr].Y,
					0,
				)
				if maxBenef > 0 {
					// We have a winner!
					fd.logRegModelsBuy[curr].Y = append(
						fd.logRegModelsBuy[curr].Y,
						1,
					)
				} else {
					// Nope :'(
					fd.logRegModelsBuy[curr].Y = append(
						fd.logRegModelsBuy[curr].Y,
						0,
					)
				}
			}

			log.Debug("Trainig model for currency:", curr, "Buy:", len(fd.logRegModelsBuy[curr].X), len(fd.logRegModelsBuy[curr].Y), "Sell:", len(fd.logRegModelsSell[curr].X), len(fd.logRegModelsSell[curr].Y))
			fd.logRegModelsSell[curr].InitializeTheta()
			fd.logRegModelsBuy[curr].InitializeTheta()
			ml.Fmincg(fd.logRegModelsSell[curr], 0.0, 1000, true)
			ml.Fmincg(fd.logRegModelsBuy[curr], 0.0, 1000, true)
			log.Debug("Model trained for currency:", curr)

			toFinish--
		}(curr, prices)
	}

	for toFinish > 0 {
		time.Sleep(time.Second)
	}
}

func (fd *Trainer) GetInfoSection(prices []charont.CurrVal, ask bool) (thrend, avg, min, max, variance, covariance float64) {
	var val, prev float64
	points := float64(len(prices))
	max = math.Inf(1)

	for i, p := range prices {
		if ask {
			val = p.Ask
		} else {
			val = p.Bid
		}

		if i != 0 {
			thrend += val / prev
		}
		avg += val
		if min > val {
			min = val
		}
		if max < val {
			max = val
		}
		avg += val

		prev = val
	}

	avg /= points
	thrend /= points - 1

	for _, p := range prices {
		if ask {
			val = p.Ask
		} else {
			val = p.Bid
		}

		variance += math.Pow(val-avg, 2)
	}

	covariance = math.Sqrt(variance)

	return
}

func (fd *Trainer) ShouldIBuy(curr string, threndOnBuy, averageBuy, priceOnBuy float64) bool {
	//hip := fd.logRegModelsBuy[curr].LogisticHipotesis([]float64{1, averageBuy / priceOnBuy, threndOnBuy})
	//log.Debug("Hip:", curr, hip)

	//return hip > 0.5
	return false
}

func (fd *Trainer) GetPredictionToSell(Profit float64, Time int64, ThrendOnBuy, ThrendOnSell, AverageBuy, AverageSell, PriceOnBuy, PriceOnSell float64) (pred float64) {
	return
}
