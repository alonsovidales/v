package hermes

import (
	//"github.com/alonsovidales/pit/log"

	"github.com/alonsovidales/pit/log"
	"github.com/alonsovidales/v/charont"
	"github.com/alonsovidales/v/philoctetes"
)

type windowTrader struct {
	Int

	collector           charont.Int
	curr                string
	windowSize          int64
	ops                 []*charont.Order
	realOps             bool
	opRunning           *charont.Order
	unitsToUse          int
	samplesToConsiderer int
	maxSecToWait        int
	trainer             philoctetes.TrainerInt
}

func GetWindowTrader(trainer philoctetes.TrainerInt, curr string, windowSize int64, collector charont.Int, unitsToUse, samplesToConsiderer, maxSecToWait int) (wt *windowTrader) {
	wt = &windowTrader{
		collector:           collector,
		trainer:             trainer,
		windowSize:          windowSize,
		realOps:             false,
		curr:                curr,
		unitsToUse:          unitsToUse,
		samplesToConsiderer: samplesToConsiderer,
		maxSecToWait:        maxSecToWait,
	}

	collector.AddListerner(curr, wt.NewPrices)

	return
}

func (wt *windowTrader) NewPrices(curr string, ts int64) {
	//rangeToStudy := wt.collector.GetRange(curr, ts-wt.windowSize, ts)
	var askVal *charont.CurrVal
	rangeToStudy := wt.collector.GetRange(curr, 0, ts)

	if len(rangeToStudy) < wt.samplesToConsiderer {
		return
	}

	log.Debug("New price:", curr, "FromTs:", (ts-wt.windowSize)/1000000000, "ToTs:", ts/1000000000, "From:", rangeToStudy[0].Ts/1000000000, "To:", rangeToStudy[len(rangeToStudy)-1].Ts/1000000000, "Window Size:", wt.windowSize/1000000000, "Total:", len(rangeToStudy))

	if wt.opRunning == nil {
		// Check if we can buy
		if wt.trainer.ShouldIBuy(curr, rangeToStudy[len(rangeToStudy)-1], rangeToStudy[:len(rangeToStudy)-1]) {
			//log.Debug("Buy:", len(rangeToStudy), rangeToStudy[len(rangeToStudy)-1].Ts, wt.curr)
			askVal = rangeToStudy[len(rangeToStudy)-1]
			wt.opRunning, _ = wt.collector.Buy(curr, wt.unitsToUse, askVal.Ask, wt.realOps, rangeToStudy[len(rangeToStudy)-1].Ts)
		}
	} else {
		// Check if we can sell
		if wt.trainer.ShouldISell(curr, rangeToStudy[len(rangeToStudy)-1], askVal, rangeToStudy[:len(rangeToStudy)-1]) {
			if err := wt.collector.CloseOrder(wt.opRunning, ts); err == nil {
				wt.ops = append(wt.ops, wt.opRunning)
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
