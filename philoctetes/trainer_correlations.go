package philoctetes

import (
	"bufio"
	"encoding/json"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/alonsovidales/pit/log"
	"github.com/alonsovidales/v/charont"
)

const (
	tsMultToSecs      = 1000000000
	winSizeSecs       = 3600 * tsMultToSecs
	minPointsInWindow = 1000
)

type TrainerCorrelations struct {
	feeds map[string][]*charont.CurrVal
}

func GetTrainerCorrelations(trainingFile string, TimeRangeToStudySecs int64) TrainerInt {
	log.Debug("Initializing trainer...")

	TimeRangeToStudySecs *= tsMultToSecs
	feedsFile, err := os.Open(trainingFile)
	if err != nil {
		log.Fatal("Problem reading the logs file")
	}

	scanner := bufio.NewScanner(feedsFile)

	feeds := &TrainerCorrelations{
		feeds: make(map[string][]*charont.CurrVal),
	}

	i := 0
	for {
		var feed *charont.CurrVal

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
			feeds.feeds[curr] = []*charont.CurrVal{}
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
	val *charont.CurrVal
	w   float64
	d   float64

	charAskMin float64
	charAskMax float64
	charAskAvg float64

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
		if curr != "USD" {
			continue
		}

		valsForScore[curr] = ByScore{}
		minMaxWin := [2]float64{math.Inf(1), math.Inf(-1)}
		minMaxD := [2]float64{math.Inf(1), math.Inf(-1)}
		minMaxW := [2]float64{math.Inf(1), math.Inf(-1)}
	pointToStudyLoop:
		for i, val := range vals {
			charAskMin, charAskMax, charAskAvg, noPossibleToStudy := tr.getPointCharacteristics(val, vals[:i])
			if noPossibleToStudy {
				continue pointToStudyLoop
			}

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

						charAskMin: charAskMin,
						charAskMax: charAskMax,
						charAskAvg: charAskAvg,
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

					charAskMin: charAskMin,
					charAskMax: charAskMax,
					charAskAvg: charAskAvg,
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
		log.Debug("Total Positive Scores", scoresUsed, "Total points:", len(valsForScore[curr]))
		for _, score := range valsForScore[curr][:10] {
			log.Debug("Curr:", curr, "Score:", score)
		}

		// Using k-means try to find 2 centroids:
		//  - Buy
		//  - Don't buy
		// Identify what centroid is what
		// Check the Precission and recall obtained using this 2 centroids
	}
}

func (tr *TrainerCorrelations) ShouldIBuy(curr string, val *charont.CurrVal, vals []*charont.CurrVal) bool {
	/*charAskMin, charAskMax, charAskAvg, noPossibleToStudy := tr.getPointCharacteristics(val, vals)
	if !noPossibleToStudy {
		return false
	}*/

	return true
}

func (tr *TrainerCorrelations) ShouldISell(curr string, currVal, askVal *charont.CurrVal, vals []*charont.CurrVal) bool {
	return true
}

func (tr *TrainerCorrelations) getPointCharacteristics(val *charont.CurrVal, vals []*charont.CurrVal) (charAskMin, charAskMax, charAskAvg float64, noPossibleToStudy bool) {
	// Get all the previous points inside the defined
	// window size that will define this point:
	pointsInRange := []*charont.CurrVal{}
	for _, winVal := range vals {
		if val.Ts-winVal.Ts >= winSizeSecs {
			pointsInRange = append(pointsInRange, winVal)
		}
	}
	if len(pointsInRange) < minPointsInWindow {
		noPossibleToStudy = true
		return
	}
	noPossibleToStudy = false

	minMaxAsk := [2]float64{math.Inf(1), math.Inf(-1)}
	avgValAsk := 0.0
	for _, point := range pointsInRange {
		avgValAsk += point.Ask
		if point.Ask > minMaxAsk[1] {
			minMaxAsk[1] = point.Ask
		}
		if point.Ask < minMaxAsk[0] {
			minMaxAsk[0] = point.Ask
		}
	}
	avgValAsk /= float64(len(pointsInRange))

	charAskMin = val.Ask / minMaxAsk[0]
	charAskMax = val.Ask / minMaxAsk[1]
	charAskAvg = val.Ask / avgValAsk

	return
}
