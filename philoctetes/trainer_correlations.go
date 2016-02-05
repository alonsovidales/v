package philoctetes

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/alonsovidales/pit/log"
	"github.com/alonsovidales/v/charont"
)

type TrainerCorrelations struct {
	feeds map[string][]charont.CurrVal
}

func GetTrainerCorrelations(trainingFile string, TimeRangeToStudySecs int64) TrainerInt {
	log.Debug("Initializing trainer...")

	TimeRangeToStudySecs *= 1000000000
	feedsFile, err := os.Open(trainingFile)
	if err != nil {
		log.Fatal("Problem reading the logs file")
	}

	scanner := bufio.NewScanner(feedsFile)

	feeds := &TrainerCorrelations{
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

	feeds.studyCurrencies(TimeRangeToStudySecs)

	return feeds
}

type ScoreCounter struct {
	val    charont.CurrVal
	w      float64
	d      float64
	maxWin float64
	score  float64
}
type ByScore []*ScoreCounter

func (a ByScore) Len() int           { return len(a) }
func (a ByScore) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByScore) Less(i, j int) bool { return a[i].score > a[j].score }

func (tr *TrainerCorrelations) studyCurrencies(TimeRangeToStudySecs int64) {
	log.Debug(len(tr.feeds["USD"]), tr.feeds["USD"][0])
	valsForScore := make(map[string]ByScore)

	for curr, vals := range tr.feeds {
		valsForScore[curr] = ByScore{}
		minMaxWin := [2]float64{math.Inf(1), math.Inf(-1)}
		minMaxD := [2]float64{math.Inf(1), math.Inf(-1)}
		minMaxW := [2]float64{math.Inf(1), math.Inf(-1)}
		for i, val := range vals {
			found := int64(-1)
			w := -1.0
			maxWin := -1.0
		winningRangStudy:
			for _, futureVal := range vals[i+1:] {
				currWin := futureVal.Bid / val.Ask
				if currWin > 1 && currWin > maxWin {
					maxWin = futureVal.Bid / val.Ask
				}

				if found != -1 {
					if futureVal.Bid < val.Ask {
						w = float64(futureVal.Ts - found)
						break winningRangStudy
					}
				} else {
					if currWin > 1 {
						found = futureVal.Ts
					}
				}
			}

			if found != -1 {
				if w != -1 {
					//log.Debug("New Range:", curr, w/1000000000, float64(found-val.Ts)/1000000000, maxWin)
					maxWinFlo := maxWin - 1
					dFlo := float64(found - val.Ts)
					valsForScore[curr] = append(valsForScore[curr], &ScoreCounter{
						val:    val,
						w:      w,
						d:      dFlo,
						maxWin: maxWinFlo,
					})

					if w < minMaxW[0] {
						minMaxW[0] = w
					}
					if w > minMaxW[1] {
						minMaxW[1] = w
					}
					if dFlo < minMaxD[0] {
						minMaxD[0] = dFlo
					}
					if dFlo > minMaxD[1] {
						minMaxD[1] = dFlo
					}
					if maxWinFlo < minMaxWin[0] {
						minMaxWin[0] = maxWinFlo
					}
					if maxWinFlo > minMaxWin[1] {
						minMaxWin[1] = maxWinFlo
					}
				}
			} else {
				// This is a bad point, we need to keep track of this also
				valsForScore[curr] = append(valsForScore[curr], &ScoreCounter{
					val:    val,
					w:      0,
					d:      1,
					maxWin: 0,
				})
			}
		}

		// Now normalise and prepare the scores
		scoresUsed := 0
		for i := 0; i < len(valsForScore[curr]); i++ {
			if valsForScore[curr][i].maxWin != 0 {
				//log.Debug("Before Normalise:", valsForScore[curr][i])
				valsForScore[curr][i].maxWin = (valsForScore[curr][i].maxWin - minMaxWin[0]) / (minMaxWin[1] - minMaxWin[0])
				valsForScore[curr][i].d = (valsForScore[curr][i].d - minMaxD[0]) / (minMaxD[1] - minMaxD[0])
				valsForScore[curr][i].w = (valsForScore[curr][i].w - minMaxW[0]) / (minMaxW[1] - minMaxW[0])
				if valsForScore[curr][i].maxWin != 0 && valsForScore[curr][i].d != 0 && valsForScore[curr][i].w != 0 {
					valsForScore[curr][i].score = (valsForScore[curr][i].maxWin * valsForScore[curr][i].w) / valsForScore[curr][i].d
					scoresUsed++
					//log.Debug("After Normalise:", valsForScore[curr][i])
				}
			}
		}

		sort.Sort(valsForScore[curr])
		fmt.Println(scoresUsed, len(valsForScore[curr]))
		for _, score := range valsForScore[curr][:10] {
			fmt.Println(curr, score)
		}
	}
}

func (tr *TrainerCorrelations) GetInfoSection(prices []charont.CurrVal, ask bool) (thrend, avg, min, max, variance, covariance float64) {
	return
}

func (tr *TrainerCorrelations) ShouldIBuy(curr string, threndOnBuy, averageBuy, priceOnBuy float64) bool {
	return true
}

func (tr *TrainerCorrelations) GetPredictionToSell(Profit float64, Time int64, ThrendOnBuy, ThrendOnSell, AverageBuy, AverageSell, PriceOnBuy, PriceOnSell float64) (pred float64) {
	return
}
