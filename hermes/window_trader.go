package hermes

import (
	//"github.com/alonsovidales/pit/log"

	"sync"

	"github.com/alonsovidales/pit/log"
	"github.com/alonsovidales/v/charont"
	"github.com/alonsovidales/v/philoctetes"
)

const (
	tsMultToSecs = 1000000000
)

type windowTrader struct {
	Int

	collector           charont.Int
	curr                string
	ops                 []*charont.Order
	realOps             bool
	opRunning           *charont.Order
	unitsToUse          int
	samplesToConsiderer int
	maxSecToWait        int
	trainer             philoctetes.TrainerInt
	askVal              *charont.CurrVal
	id                  int
	mutex               *sync.Mutex
}

func GetWindowTrader(id int, trainer philoctetes.TrainerInt, curr string, collector charont.Int, unitsToUse, samplesToConsiderer, maxSecToWait int) (wt *windowTrader) {
	wt = &windowTrader{
		collector:           collector,
		trainer:             trainer,
		realOps:             false,
		curr:                curr,
		unitsToUse:          unitsToUse,
		samplesToConsiderer: samplesToConsiderer,
		maxSecToWait:        maxSecToWait,
		id:                  id,
		mutex:               new(sync.Mutex),
	}

	collector.AddListerner(curr, wt.NewPrices)

	return
}

func (wt *windowTrader) GetID() int {
	return wt.id
}

func (wt *windowTrader) NewPrices(curr string, ts int64) {
	var realOpsStr string

	wt.mutex.Lock()
	defer wt.mutex.Unlock()

	if wt.realOps {
		realOpsStr = "Real"
	} else {
		realOpsStr = "Simulation"
	}
	currVals := wt.collector.GetAllCurrVals()
	lastVal := currVals[curr][len(currVals[curr])-1]
	if wt.opRunning == nil {
		// Check if we can buy
		if should, typeOper := wt.trainer.ShouldIOperate(curr, currVals, wt.id); should {
			log.Debug("Buy:", curr, "ID:", wt.id, "Price:", lastVal.Ask, "Type:", realOpsStr)
			if typeOper == "buy" {
				wt.opRunning, _ = wt.collector.Buy(curr, wt.unitsToUse, lastVal.Ask, wt.realOps, lastVal.Ts)
			} else {
				wt.opRunning, _ = wt.collector.Sell(curr, wt.unitsToUse, lastVal.Bid, wt.realOps, lastVal.Ts)
			}
			wt.askVal = lastVal
		}
	} else {
		// Check if we can sell
		if wt.trainer.ShouldIClose(curr, wt.askVal, currVals, wt.id, wt.opRunning) {
			scoreBefSell := wt.GetScore(3)
			totalProfitBefSell := wt.GetTotalProfit()
			if err := wt.collector.CloseOrder(wt.opRunning, lastVal.Ts); err == nil {
				wt.ops = append(wt.ops, wt.opRunning)
				log.Debug("Selling:", curr, "Trader:", wt.id, "Profit:", wt.ops[len(wt.ops)-1].Profit, "Time:", float64(lastVal.Ts-wt.askVal.Ts)/tsMultToSecs, "TotalProfit:", wt.GetTotalProfit(), "Score:", wt.GetScore(3), "scoreBefSell:", scoreBefSell, "totalProfitBefSell:", totalProfitBefSell, "Real:", realOpsStr)
				wt.opRunning = nil
			}
		}
	}
}

func (wt *windowTrader) GetNumOps() int {
	return len(wt.ops)
}

func (wt *windowTrader) IsPlaying() bool {
	return wt.realOps
}

func (wt *windowTrader) StartPlaying() {
	wt.realOps = true
}

func (wt *windowTrader) StopPlaying() bool {
	if wt.opRunning != nil {
		return false
	}

	wt.realOps = false
	return true
}

func (wt *windowTrader) GetMicsecsBetweenOps(lastOps int) float64 {
	var toStudy []*charont.Order

	if len(wt.ops) < lastOps {
		toStudy = wt.ops
	} else {
		toStudy = wt.ops[len(wt.ops)-lastOps:]
	}

	fromTs := toStudy[0].SellTs
	distance := int64(0)
	for _, op := range toStudy[1:] {
		distance += op.SellTs - fromTs
		fromTs = op.SellTs
	}

	return (float64(distance) / float64(len(toStudy)-1))
}

func (wt *windowTrader) GetTotalProfit() (profit float64) {
	profit = 1
	for _, op := range wt.ops {
		if op != nil {
			profit *= op.Profit + 1
		}
	}

	return
}

func (wt *windowTrader) GetScore(lastOps int) (score float64) {
	var toStudy []*charont.Order

	if len(wt.ops) < lastOps {
		toStudy = wt.ops
	} else {
		toStudy = wt.ops[len(wt.ops)-lastOps:]
	}

	score = 1
	for _, op := range toStudy {
		score *= op.Profit + 1
	}

	return
}
