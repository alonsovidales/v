package hermes

import (
	//"github.com/alonsovidales/pit/log"

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
	}

	collector.AddListerner(curr, wt.NewPrices)

	return
}

func (wt *windowTrader) NewPrices(curr string, ts int64) {
	rangeToStudy := wt.collector.GetRange(curr, 0, ts)

	if len(rangeToStudy) < wt.samplesToConsiderer {
		return
	}

	lastVal := rangeToStudy[len(rangeToStudy)-1]
	log.Debug("New price:", curr, "Ask:", lastVal.Ask, "Bid:", lastVal.Bid, "Total Prices:", len(rangeToStudy))

	if wt.opRunning == nil {
		// Check if we can buy
		if wt.trainer.ShouldIBuy(curr, lastVal, rangeToStudy[:len(rangeToStudy)-1], wt.id) {
			log.Debug("Buy:", curr, wt.id, lastVal.Ask)
			wt.opRunning, _ = wt.collector.Buy(curr, wt.unitsToUse, lastVal.Ask, wt.realOps, lastVal.Ts)
			wt.askVal = lastVal
		}
	} else {
		// Check if we can sell
		if wt.trainer.ShouldISell(curr, lastVal, wt.askVal, rangeToStudy[:len(rangeToStudy)-1], wt.id) {
			if err := wt.collector.CloseOrder(wt.opRunning, ts); err == nil {
				wt.ops = append(wt.ops, wt.opRunning)
				log.Debug("Selling:", curr, "Trader:", wt.id, "Profit:", wt.ops[len(wt.ops)-1].Profit, "Time:", float64(lastVal.Ts-wt.askVal.Ts)/tsMultToSecs, "TotalProfit:", wt.getTotalProfit())
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

func (wt *windowTrader) StopPlaying() {
	wt.realOps = false
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

func (wt *windowTrader) getTotalProfit() (profit float64) {
	profit = 1
	for _, op := range wt.ops {
		profit *= op.Profit + 1
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

	score = float64(1)
	for _, op := range toStudy {
		score *= op.Profit / float64(op.Units)
	}

	return
}
