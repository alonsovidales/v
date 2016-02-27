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
	tradersPlaying    map[int]hermes.Int
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
		traders:           make([]hermes.Int, philoctetes.TrainersToRun*len(collector.GetCurrencies())),
		collector:         collector,
		tradesThatCanPlay: tradesThatCanPlay,
		lastOpsToConsider: lastOpsToConsider,
		tradersPlaying:    make(map[int]hermes.Int),
	}

	for i, curr := range collector.GetCurrencies() {
		for t := 0; t < philoctetes.TrainersToRun; t++ {
			log.Debug("Launching trader:", curr, "Id:", t, "TotalToLaunch:", len(hades.traders), i*t)
			hades.traders[i*philoctetes.TrainersToRun+t] = hermes.GetWindowTrader(t, trainer, curr, collector, unitsToUse, samplesToConsiderer, maxSecsToWait)
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
			//log.Debug("Checking trader to play:", i)
			if trader.GetNumOps() >= hades.lastOpsToConsider && trader.GetScore(10000) > 1 {
				canPlay = append(canPlay, &SortTraders{
					Trader: trader,
					Score:  trader.GetScore(10000),
				})
			}
		}
		sort.Sort(canPlay)

		toStop := []int{}
		for id, trader := range hades.tradersPlaying {
			if trader.StopPlaying() {
				toStop = append(toStop, id)
			}
		}

		for _, id := range toStop {
			delete(hades.tradersPlaying, id)
		}

	addTradersLoop:
		for _, newTrader := range canPlay {
			if _, ok := hades.tradersPlaying[newTrader.Trader.GetID()]; !ok {
				hades.tradersPlaying[newTrader.Trader.GetID()] = newTrader.Trader
				newTrader.Trader.StartPlaying()
			}

			if len(hades.tradersPlaying) > hades.tradesThatCanPlay {
				break addTradersLoop
			}
		}
	}
}
