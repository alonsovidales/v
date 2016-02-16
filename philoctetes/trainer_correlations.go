package philoctetes

import (
	"bufio"
	"encoding/json"
	"math"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/alonsovidales/pit/log"
	"github.com/alonsovidales/v/charont"
)

const (
	tsMultToSecs             = 1000000000
	winSizeSecs              = 7200 * tsMultToSecs
	minPointsInWindow        = 1000
	clusters                 = 20
	clustersToUse            = 10
	secsToWaitUntilForceSell = 3600 * 3
	maxLoss                  = -0.003
	TrainersToRun            = clustersToUse * 10
)

type TrainerCorrelations struct {
	feeds            map[string][]*charont.CurrVal
	centroidsCurr    map[string][][]float64
	centroidsForAsk  map[string][]int
	maxWinByCentroid []float64
	mutex            sync.Mutex
}

type ScoreCounter struct {
	val *charont.CurrVal
	w   float64
	d   float64

	charAskMin  float64
	charAskMax  float64
	charAskMean float64
	charAskMode float64

	maxWin       float64
	maxWinNoNorm float64
	score        float64
}
type ByScore []*ScoreCounter

func (a ByScore) Len() int           { return len(a) }
func (a ByScore) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByScore) Less(i, j int) bool { return a[i].score > a[j].score }

func GetTrainerCorrelations(trainingFile string, TimeRangeToStudySecs int64) TrainerInt {
	log.Debug("Initializing trainer...")

	TimeRangeToStudySecs *= tsMultToSecs
	feedsFile, err := os.Open(trainingFile)
	log.Debug("File:", trainingFile)
	if err != nil {
		log.Fatal("Problem reading the logs file")
	}

	scanner := bufio.NewScanner(feedsFile)

	feeds := &TrainerCorrelations{
		feeds:           make(map[string][]*charont.CurrVal),
		centroidsCurr:   make(map[string][][]float64),
		centroidsForAsk: make(map[string][]int),
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

	for curr, scores := range feeds.feeds {
		log.Debug("Curr:", curr, "Scores:", len(scores))
	}

	feeds.studyCurrencies(TimeRangeToStudySecs)

	return feeds
}

func (tr *TrainerCorrelations) studyCurrencies(TimeRangeToStudySecs int64) {
	log.Debug(len(tr.feeds["USD"]), tr.feeds["USD"][0])
	currsTrained := 0

	for curr, vals := range tr.feeds {
		/*if curr != "USD" {
			continue
		}*/
		go func(curr string, vals []*charont.CurrVal) {
			valsForScore := ByScore{}
			minMaxWin := [2]float64{math.Inf(1), math.Inf(-1)}
			minMaxD := [2]float64{math.Inf(1), math.Inf(-1)}
			minMaxW := [2]float64{math.Inf(1), math.Inf(-1)}
			lastWindowFirstPosUsed := 0
		pointToStudyLoop:
			for i, val := range vals {
				if i%1000 == 0 {
					log.Debug("Processed:", i, "points for currency:", curr)
				}
				charAskMin, charAskMax, charAskMean, charAskMode, noPossibleToStudy, firstWindowPos := tr.getPointCharacteristics(val, vals[lastWindowFirstPosUsed:i])
				//log.Debug("Study - Val:", val, "lastWindowFirstPosUsed:", lastWindowFirstPosUsed, "Vals:", len(vals[lastWindowFirstPosUsed:i]), charAskMin, charAskMax, charAskMean, charAskMode, noPossibleToStudy, firstWindowPos)
				if noPossibleToStudy {
					log.Debug("No study, no possible")
					continue pointToStudyLoop
				}
				lastWindowFirstPosUsed += firstWindowPos

				found := int64(-1)
				w := -1.0
				maxWin := -1.0
			winningRangStudy:
				for _, futureVal := range vals[i+1:] {
					currWin := futureVal.Bid / val.Ask
					if currWin > maxWin {
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

				maxWinFlo := maxWin - 1
				if found != -1 {
					if w != -1 {
						//log.Debug("New Range:", curr, w/1000000000, float64(found-val.Ts)/1000000000, maxWin)
						dFlo := float64(found - val.Ts)
						valsForScore = append(valsForScore, &ScoreCounter{
							val:          val,
							w:            w,
							d:            dFlo,
							maxWin:       maxWinFlo,
							maxWinNoNorm: maxWinFlo,

							charAskMin:  charAskMin,
							charAskMax:  charAskMax,
							charAskMean: charAskMean,
							charAskMode: charAskMode,
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
					valsForScore = append(valsForScore, &ScoreCounter{
						val:          val,
						w:            0,
						d:            1,
						maxWin:       maxWinFlo,
						maxWinNoNorm: maxWinFlo,

						charAskMin:  charAskMin,
						charAskMax:  charAskMax,
						charAskMean: charAskMean,
						charAskMode: charAskMode,
					})
				}
			}

			// Now normalise and prepare the scores
			for i := 0; i < len(valsForScore); i++ {
				if valsForScore[i].w != 0 {
					//log.Debug("Before Normalise:", valsForScore[i])
					valsForScore[i].maxWin = (valsForScore[i].maxWin - minMaxWin[0]) / (minMaxWin[1] - minMaxWin[0])
					valsForScore[i].d = (valsForScore[i].d - minMaxD[0]) / (minMaxD[1] - minMaxD[0])
					valsForScore[i].w = (valsForScore[i].w - minMaxW[0]) / (minMaxW[1] - minMaxW[0])
					if valsForScore[i].maxWin != 0 && valsForScore[i].d != 0 && valsForScore[i].w != 0 {
						valsForScore[i].score = (valsForScore[i].maxWin * valsForScore[i].w) / valsForScore[i].d
						//log.Debug("After Normalise:", valsForScore[i])
					} else {
						valsForScore[i].score = -1
					}
				}
			}

			sort.Sort(valsForScore)
			log.Debug("Error:", curr, len(valsForScore))
			for _, score := range valsForScore[:10] {
				log.Debug("Curr:", curr, "Score:", score)
			}

			// Using k-means try to find 2 centroids:
			//  - Buy
			//  - Don't buy
			// Identify what centroid is what
			// Check the Precission and recall obtained using this 2 centroids

			// Init the clusters centroids
			log.Debug("Moving centroids")
			for c := 0; c < clusters; c++ {
				pos := c * ((len(valsForScore) - 1) / (clusters - 1))
				tr.mutex.Lock()
				tr.centroidsCurr[curr] = append(tr.centroidsCurr[curr], []float64{
					valsForScore[pos].charAskMin,  // ask min relation
					valsForScore[pos].charAskMax,  // ask max relation
					valsForScore[pos].charAskMean, // ask mean relation
					valsForScore[pos].charAskMode, // ask mode relation
				})
				tr.mutex.Unlock()
			}

			modified := true
			for modified {
				scoresByCentroid := make([][]*ScoreCounter, clusters)
				for _, score := range valsForScore {
					centroid := tr.getClosestCentroid(score, curr)
					scoresByCentroid[centroid] = append(scoresByCentroid[centroid], score)
				}

				for i := 0; i < clusters; i++ {
					log.Debug("Items centroid:", i, len(scoresByCentroid[i]))
				}

				// Move the centroids
				modified = false
				for c := 0; c < clusters; c++ {
					log.Debug("scoresByCentroid:", c, len(scoresByCentroid[c]))
					oldCentroid := tr.centroidsCurr[curr][c]
					tr.centroidsCurr[curr][c] = []float64{
						0.0,
						0.0,
						0.0,
						0.0,
					}
					scoresCentroid := float64(len(scoresByCentroid[c]))
					for _, score := range scoresByCentroid[c] {
						tr.centroidsCurr[curr][c][0] += score.charAskMin / scoresCentroid
						tr.centroidsCurr[curr][c][1] += score.charAskMax / scoresCentroid
						tr.centroidsCurr[curr][c][2] += score.charAskMean / scoresCentroid
						tr.centroidsCurr[curr][c][3] += score.charAskMode / scoresCentroid
					}

					// Check if the centroid was moved or not
					for i := 0; i < len(oldCentroid); i++ {
						if oldCentroid[i] != tr.centroidsCurr[curr][c][i] {
							modified = true
						}
					}
					log.Debug("Centroids:", c, oldCentroid, tr.centroidsCurr[curr][c])
				}
			}

			// With the clsters initted, try to estimate the score by centroid
			avgScoreCent := make([]float64, clusters)
			avgMaxWin := make([]float64, clusters)
			scoresByCentroid := make([]int, clusters)
			tr.maxWinByCentroid = make([]float64, clusters)
			for _, score := range valsForScore {
				centroid := tr.getClosestCentroid(score, curr)
				avgScoreCent[centroid] += score.score
				avgMaxWin[centroid] += score.maxWinNoNorm
				tr.maxWinByCentroid[centroid] += score.maxWinNoNorm
				scoresByCentroid[centroid]++
				if centroid == 10 {
					log.Debug("Score on master centroid:", score)
					log.Debug("Score on master centroid-:", score.score)
				}
			}

			scores := make([]float64, clusters)
			for c := 0; c < clusters; c++ {
				tr.maxWinByCentroid[c] /= float64(scoresByCentroid[c])
				avgScoreCent[c] /= float64(scoresByCentroid[c])
				avgMaxWin[c] /= float64(scoresByCentroid[c])
				scores[c] = avgMaxWin[c]
				log.Debug("Centroid:", c, "Items:", scoresByCentroid[c], "Score", avgScoreCent[c], "Avg MaxWin:", avgMaxWin[c], "CentroidPos:", tr.centroidsCurr[curr][c])
			}

			sort.Float64s(scores)
			scores = scores[clusters-clustersToUse:]

			tr.centroidsForAsk[curr] = []int{}
			for _, score := range scores {
				for k, v := range avgMaxWin {
					if v == score {
						tr.centroidsForAsk[curr] = append(tr.centroidsForAsk[curr], k)
					}
				}
			}

			log.Debug("Centroides to use:", curr, tr.centroidsForAsk[curr])
			currsTrained++
		}(curr, vals)
	}

	for currsTrained < len(tr.feeds) {
		time.Sleep(time.Second)
	}
}

func (tr *TrainerCorrelations) getClosestCentroid(score *ScoreCounter, curr string) (c int) {
	c = 0
	minDist := math.Inf(1)
	for ci, centroid := range tr.centroidsCurr[curr] {
		distToCentroid := (score.charAskMin - centroid[0]) * (score.charAskMin - centroid[0])
		distToCentroid += (score.charAskMax - centroid[1]) * (score.charAskMax - centroid[1])
		distToCentroid += (score.charAskMean - centroid[2]) * (score.charAskMean - centroid[2])
		distToCentroid += (score.charAskMode - centroid[3]) * (score.charAskMode - centroid[3])
		//log.Debug("Dist To cent:", curr, score, ci, centroid, distToCentroid)
		if distToCentroid < minDist {
			minDist = distToCentroid
			c = ci
		}
	}

	return
}

func (tr *TrainerCorrelations) ShouldIBuy(curr string, val *charont.CurrVal, vals []*charont.CurrVal, traderID int) bool {
	traderCentroid := traderID / clustersToUse

	charAskMin, charAskMax, charAskMean, charAskMode, noPossibleToStudy, _ := tr.getPointCharacteristics(val, vals)
	if noPossibleToStudy {
		return false
	}

	centroid := tr.getClosestCentroid(&ScoreCounter{
		val:         val,
		charAskMin:  charAskMin,
		charAskMax:  charAskMax,
		charAskMean: charAskMean,
		charAskMode: charAskMode,
	}, curr)
	log.Debug("Point chars - charAskMin:", charAskMin, "charAskMax:", charAskMax, "charAskMean:", charAskMean, "charAskMode:", charAskMode, "traderID:", traderID, "Centroid:", centroid)

	return tr.centroidsForAsk[curr][traderCentroid] == centroid
}

func (tr *TrainerCorrelations) ShouldISell(curr string, currVal, askVal *charont.CurrVal, vals []*charont.CurrVal, traderID int) bool {
	traderCentroid := traderID / clustersToUse
	traderAvgDiv := float64(traderID % clustersToUse)

	secondsUsed := (currVal.Ts - askVal.Ts) / tsMultToSecs

	switch {
	case (secondsUsed > secsToWaitUntilForceSell/2 && currVal.Bid/askVal.Ask > 1):
		// More than the half of the time and some profit
		log.Debug("Selling by profit > 1 and time > totalTime/2, secs used", secondsUsed, "Centroid:", traderCentroid, "Secs to wait:", secsToWaitUntilForceSell/2, "Profit:", currVal.Bid/askVal.Ask, "Avg:", tr.maxWinByCentroid[tr.centroidsForAsk[curr][traderCentroid]])
		return true
	case currVal.Bid/askVal.Ask-1 < maxLoss:
		log.Debug("Selling by max loss, loss:", currVal.Bid/askVal.Ask-1, "Centroid:", traderCentroid, "Max Loss:", maxLoss)
		return true
	case currVal.Bid/askVal.Ask-1 > tr.maxWinByCentroid[tr.centroidsForAsk[curr][traderCentroid]]/traderAvgDiv:
		// More than the AVG profit/3
		log.Debug("Selling by profit > avg/", traderAvgDiv, ", Centroid:", traderCentroid, "Profit:", currVal.Bid/askVal.Ask, "Avg:", tr.maxWinByCentroid[tr.centroidsForAsk[curr][traderCentroid]])
		return true
	case secondsUsed > secsToWaitUntilForceSell:
		// Out of time...
		log.Debug("Selling by time: Centroid:", traderCentroid, secondsUsed, secsToWaitUntilForceSell)
		return true
	}

	return false
}

func (tr *TrainerCorrelations) getPointCharacteristics(val *charont.CurrVal, vals []*charont.CurrVal) (charAskMin, charAskMax, charAskMean, charAskMode float64, noPossibleToStudy bool, firstWindowPos int) {
	// Get all the previous points inside the defined
	// window size that will define this point:
	var pointsInRange []*charont.CurrVal

	for i, winVal := range vals {
		if val.Ts-winVal.Ts >= winSizeSecs {
			pointsInRange = vals[i:]
			firstWindowPos = i
		} else {
			break
		}
	}
	if len(pointsInRange) < minPointsInWindow {
		noPossibleToStudy = true
		return
	}
	noPossibleToStudy = false

	minMaxAsk := [2]float64{math.Inf(1), math.Inf(-1)}
	meanValAsk := 0.0
	modeValAsk := make(map[float64]int)
	for _, point := range pointsInRange {
		meanValAsk += point.Ask
		if point.Ask > minMaxAsk[1] {
			minMaxAsk[1] = point.Ask
		}
		if point.Ask < minMaxAsk[0] {
			minMaxAsk[0] = point.Ask
		}

		if _, ok := modeValAsk[point.Ask]; ok {
			modeValAsk[point.Ask]++
		} else {
			modeValAsk[point.Ask] = 0
		}
	}
	maxTimes := 0
	for ask, times := range modeValAsk {
		if maxTimes < times {
			charAskMode = ask
			maxTimes = times
		}
	}
	meanValAsk /= float64(len(pointsInRange))

	charAskMin = val.Ask / minMaxAsk[0]
	charAskMax = val.Ask / minMaxAsk[1]
	charAskMean = val.Ask / meanValAsk
	charAskMode = val.Ask / charAskMode

	return
}
