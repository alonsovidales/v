package hermes

import (
	"github.com/alonsovidales/v/charont"
	"math"
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
}

func GetWindowTrader(curr string, windowSize int64, collector charont.Int, unitsToUse, samplesToConsiderer, maxSecToWait int) (wt *windowTrader) {
	wt = &windowTrader{
		collector:           collector,
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
	rangeToStudy := wt.collector.GetRange(curr, ts-wt.windowSize, ts)

	if len(rangeToStudy) < wt.samplesToConsiderer {
		return
	}

	//log.Debug("New price:", curr, "FromTs:", (ts-wt.windowSize)/1000000000, "ToTs:", ts/1000000000, "From:", rangeToStudy[0].Ts/1000000000, "To:", rangeToStudy[len(rangeToStudy)-1].Ts/1000000000, "Window Size:", wt.windowSize/1000000000, "Total:", len(rangeToStudy))
	avgValue := 0.0
	maxPrice := 0.0
	minPrice := math.Inf(1)
	for _, val := range rangeToStudy {
		if wt.opRunning == nil {
			avgValue += val.Ask
			if maxPrice < val.Ask {
				maxPrice = val.Ask
			}
			if minPrice > val.Ask {
				minPrice = val.Ask
			}
		} else {
			avgValue += val.Bid
			if maxPrice < val.Bid {
				maxPrice = val.Bid
			}
			if minPrice > val.Bid {
				minPrice = val.Bid
			}
		}
	}

	lastThrend := 0.0
	prevVal := 0.0
	lastRange := rangeToStudy[len(rangeToStudy)-wt.samplesToConsiderer/2:]
	for i, val := range lastRange {
		if i != 0 {
			lastThrend = val.Ask - prevVal
		}
		prevVal = val.Ask
	}
	//lastThrend /= float64(len(lastRange)) - 1
	avgValue /= float64(len(rangeToStudy))
	if wt.opRunning == nil {
		// Check if we can buy
		lastAskPrice := rangeToStudy[len(rangeToStudy)-1].Ask
		if lastAskPrice < avgValue && lastThrend > 0 {
			//log.Debug("Buy:", len(rangeToStudy), rangeToStudy[len(rangeToStudy)-1].Ts, wt.curr)
			wt.opRunning, _ = wt.collector.Buy(curr, wt.unitsToUse, lastAskPrice, wt.realOps, rangeToStudy[len(rangeToStudy)-1].Ts)
		}
	} else {
		// Check if we can sell
		lastSellPrice := rangeToStudy[len(rangeToStudy)-1].Bid
		prevPrice := rangeToStudy[len(rangeToStudy)-2].Bid
		ts := rangeToStudy[len(rangeToStudy)-1].Ts
		if prevPrice == maxPrice && prevPrice > lastSellPrice && (wt.opRunning.Price < lastSellPrice || (int(ts-wt.opRunning.BuyTs)/1000000000) > wt.maxSecToWait) {
			err := wt.collector.CloseOrder(wt.opRunning, ts)
			if err == nil {
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
