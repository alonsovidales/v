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
	"os"
	//"sort"
	"strings"
	"time"
)

type Result struct {
	ProfitByTime float64
	Profit       float64
	Time         int64
	ThrendOnBuy  float64
	ThrendOnSell float64
	AverageBuy   float64
	AverageSell  float64
	PriceOnBuy   float64
	PriceOnSell  float64
}

type resultsByCurrencyTy []*Result

func (a resultsByCurrencyTy) Len() int           { return len(a) }
func (a resultsByCurrencyTy) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a resultsByCurrencyTy) Less(i, j int) bool { return a[i].ProfitByTime > a[j].ProfitByTime }

type Trainer struct {
	feeds map[string][]charont.CurrVal
	//results          map[string]resultsByCurrencyTy
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
	//fd.results = make(map[string]resultsByCurrencyTy)
	toFinish := len(fd.feeds)
	for curr, prices := range fd.feeds {
		//fd.results[curr] = resultsByCurrencyTy{}
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

			for i, price := range prices {

				// Calculate the threads and previous conditions before
				// buy
				threndOnBuy := 0.0
				avgOnBuy := 0.0
				var prevPrice charont.CurrVal
				from := i - fd.windowSize
				if from < 0 {
					from = 0
				}
				windowRange := prices[from:i]
				for i, priceWindow := range windowRange {
					avgOnBuy += priceWindow.Ask
					if i != 0 {
						threndOnBuy += priceWindow.Ask / prevPrice.Ask
					}
					prevPrice = priceWindow
				}
				threndOnBuy /= float64(len(windowRange) - 1)
				avgOnBuy /= float64(len(windowRange))

				maxBenef := 0.0
				maxBenefPoint := 0
				minBenef := 0.0
				minBenefPoint := 0
				for j := i + 1; j < len(prices) && price.Ts+TimeRangeToStudySecs > prices[j].Ts; j++ {
					benef := prices[j].Bid/price.Ask - 1
					if benef < 0 && minBenef > benef {
						minBenef = benef
						minBenefPoint = j
					}
					if benef > 0 && maxBenef < benef {
						maxBenef = benef
						maxBenefPoint = j
					}
				}

				if minBenefPoint != 0 {
					fd.addWindowInfo(curr, prices, price, minBenefPoint, threndOnBuy, avgOnBuy)
				}
				if maxBenefPoint != 0 {
					fd.addWindowInfo(curr, prices, price, maxBenefPoint, threndOnBuy, avgOnBuy)
				}
			}

			log.Debug("Trainig model for currency:", curr)
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

	/*for curr, values := range fd.results {
		sort.Sort(values)
		for _, v := range values[:10] {
			log.Debug("CURR:", curr, v.ProfitByTime)
		}
	}*/
}

func (fd *Trainer) addWindowInfo(curr string, prices []charont.CurrVal, price charont.CurrVal, j int, threndOnBuy, avgOnBuy float64) {
	benef := prices[j].Bid/price.Ask - 1
	//rangeSecs := float64(prices[j].Ts-price.Ts) / 1000000000

	//log.Debug("Benef in Time:", curr, i, j, "Secs:", rangeSecs, "=", benef, "T:", benef/rangeSecs, price, prices[j])
	// The benef by Time is going to be the score

	//benefTime := benef / rangeSecs

	var prevPrice charont.CurrVal

	threndOnSell := 0.0
	avgOnSell := 0.0
	first := true
	fromSell := j - fd.windowSize
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

	fd.logRegModelsBuy[curr].X = append(
		fd.logRegModelsBuy[curr].X,
		[]float64{1.0000, avgOnBuy / price.Ask, threndOnBuy},
	)
	fd.logRegModelsSell[curr].X = append(
		fd.logRegModelsSell[curr].X,
		[]float64{1.0000, avgOnSell / prices[j].Bid, avgOnSell / avgOnBuy, threndOnBuy, threndOnSell},
	)
	valToAdd := 0.0
	if benef > 0 {
		valToAdd = 1
	}
	fd.logRegModelsBuy[curr].Y = append(
		fd.logRegModelsBuy[curr].Y,
		valToAdd,
	)
	fd.logRegModelsSell[curr].Y = append(
		fd.logRegModelsSell[curr].Y,
		valToAdd,
	)

	//log.Debug(benefTime)
	/*fd.results[curr] = append(fd.results[curr], &Result{
		Profit:       benef,
		ProfitByTime: benefTime,
		Time:         int64(rangeSecs) * 1000000000,
		PriceOnBuy:   price.Ask,
		PriceOnSell:  prices[j].Bid,
		ThrendOnBuy:  threndOnBuy,
		AverageBuy:   avgOnBuy,
		ThrendOnSell: threndOnSell,
		AverageSell:  avgOnSell,
	})*/
}

func (fd *Trainer) ShouldIBuy(curr string, threndOnBuy, averageBuy, priceOnBuy float64) bool {
	hip := fd.logRegModelsBuy[curr].LogisticHipotesis([]float64{1, averageBuy / priceOnBuy, threndOnBuy})
	log.Debug("Hip:", curr, hip)

	return hip > 0.5
}

func (fd *Trainer) GetPredictionToSell(Profit float64, Time int64, ThrendOnBuy, ThrendOnSell, AverageBuy, AverageSell, PriceOnBuy, PriceOnSell float64) (pred float64) {
	return
}
