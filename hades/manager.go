package hades

import (
	"sort"
	"time"

	"github.com/alonsovidales/pit/log"
	"github.com/alonsovidales/v/charont"
	"github.com/alonsovidales/v/hermes"
	"github.com/alonsovidales/v/philoctetes"
)

type Hades struct {
	traders           []hermes.Int
	collector         charont.Int
	lastOpsToConsider int
	tradesThatCanPlay int
	tradersPlaying    []hermes.Int
}

type SortTraders struct {
	Score  float64
	Trader hermes.Int
}

type TradersSortener []*SortTraders

func (a TradersSortener) Len() int           { return len(a) }
func (a TradersSortener) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a TradersSortener) Less(i, j int) bool { return a[i].Score > a[j].Score }

func GetHades(trainer philoctetes.TrainerInt, traders int, from int, collector charont.Int, unitsToUse, samplesToConsiderer, lastOpsToConsider, tradesThatCanPlay, maxSecsToWait int) (hades *Hades) {
	hades = &Hades{
		traders:           make([]hermes.Int, philoctetes.ClustersToUse*len(collector.GetCurrencies())),
		collector:         collector,
		tradesThatCanPlay: tradesThatCanPlay,
		lastOpsToConsider: lastOpsToConsider,
		tradersPlaying:    []hermes.Int{},
	}

	for i, curr := range collector.GetCurrencies() {
		for t := 0; t < philoctetes.ClustersToUse; t++ {
			hades.traders[i+t] = hermes.GetWindowTrader(t, trainer, curr, collector, unitsToUse, samplesToConsiderer, maxSecsToWait)
		}
	}

	go hades.manageTraders()

	return
}

func (hades *Hades) manageTraders() {
	c := time.Tick(100 * time.Millisecond)
	for _ = range c {
		canPlay := TradersSortener{}
		for _, trader := range hades.traders {
			if trader.GetNumOps() >= hades.lastOpsToConsider && trader.GetScore(hades.lastOpsToConsider) > 0 {
				canPlay = append(canPlay, &SortTraders{
					Trader: trader,
					Score:  trader.GetScore(hades.lastOpsToConsider),
				})
			}
		}

		for _, trader := range hades.tradersPlaying {
			trader.StopPlaying()
		}
		hades.tradersPlaying = []hermes.Int{}
		sort.Sort(canPlay)
		if len(canPlay) > hades.tradesThatCanPlay {
			canPlay = canPlay[:hades.tradesThatCanPlay]
		}
		for _, trader := range canPlay {
			log.Debug("Canplay:", trader.Score)
			trader.Trader.StartPlaying()
			hades.tradersPlaying = append(hades.tradersPlaying, trader.Trader)
		}
	}
}
