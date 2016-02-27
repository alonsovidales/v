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
	maxLoss                  = -0.03
	TrainersToRun            = clustersToUse * 10 * 2
	cMinLossRange            = 0.80
)

type TrainerCorrelations struct {
	feeds                 map[string][]*charont.CurrVal
	centroidsCurr         map[string][][]float64
	centroidsCurrSell     map[string][][]float64
	centroidsForAsk       map[string][]int
	centroidsForSell      map[string][]int
	maxWinByCentroid      map[string][]float64
	maxLossByCentroid     map[string][]float64
	maxWinByCentroidSell  map[string][]float64
	maxLossByCentroidSell map[string][]float64
	mutex                 sync.Mutex
}

type ScoreCounter struct {
	val *charont.CurrVal
	w   float64
	d   float64

	charAskMin  float64
	charAskMax  float64
	charAskMean float64
	charAskMode float64

	maxWin        float64
	maxWinNoNorm  float64
	maxLossNoNorm float64
	score         float64
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
		feeds:                 make(map[string][]*charont.CurrVal),
		centroidsCurr:         make(map[string][][]float64),
		centroidsCurrSell:     make(map[string][][]float64),
		centroidsForAsk:       make(map[string][]int),
		centroidsForSell:      make(map[string][]int),
		maxWinByCentroid:      make(map[string][]float64),
		maxLossByCentroid:     make(map[string][]float64),
		maxWinByCentroidSell:  make(map[string][]float64),
		maxLossByCentroidSell: make(map[string][]float64),
	}

	i := 0
	for {
		var feed *charont.CurrVal

		if !scanner.Scan() {
			break
		}
		lineParts := strings.SplitN(scanner.Text(), ":", 2)
		curr := lineParts[0]
		if len(lineParts) < 2 {
			log.Error("The line:", i, "can't be parsed")
			continue
		}
		if err := json.Unmarshal([]byte(lineParts[1]), &feed); err != nil {
			log.Error("The feeds response body is not a JSON valid, Error:", err, "Line:", i)
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

			// Params to Buy
			minMaxWin := [2]float64{math.Inf(1), math.Inf(-1)}
			minMaxD := [2]float64{math.Inf(1), math.Inf(-1)}
			minMaxW := [2]float64{math.Inf(1), math.Inf(-1)}
			lastWindowFirstPosUsed := 0

			// Params to Sell
			valsForScoreSell := ByScore{}
			minMaxWinSell := [2]float64{math.Inf(1), math.Inf(-1)}
			minMaxDSell := [2]float64{math.Inf(1), math.Inf(-1)}
			minMaxWSell := [2]float64{math.Inf(1), math.Inf(-1)}

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

				// Get the range to Buy
				found := int64(-1)
				w := -1.0
				maxWin := -1.0
				maxLoss := 10.0
			winningRangStudy:
				for _, futureVal := range vals[i+1:] {
					currWin := futureVal.Bid / val.Ask
					if currWin > maxWin {
						maxWin = currWin
					}
					if currWin < maxLoss {
						maxLoss = currWin
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

				// Get the range to Sell
				foundSell := int64(-1)
				wSell := -1.0
				maxWinSell := -1.0
				maxLossSell := 10.0
			winningRangStudySell:
				for _, futureVal := range vals[i+1:] {
					currWin := val.Bid / futureVal.Ask
					if currWin > maxWinSell {
						maxWinSell = currWin
					}
					if currWin < maxLossSell {
						maxLossSell = currWin
					}

					if foundSell != -1 {
						if val.Bid < futureVal.Ask {
							wSell = float64(futureVal.Ts - foundSell)
							break winningRangStudySell
						}
					} else {
						if currWin > 1 {
							foundSell = futureVal.Ts
						}
					}
				}

				// Calculate the scores for Buy
				maxWinFlo := maxWin - 1
				maxLossFlo := maxLoss - 1
				if found != -1 {
					if w != -1 {
						//log.Debug("New Range:", curr, w/1000000000, float64(found-val.Ts)/1000000000, maxWin)
						dFlo := float64(found - val.Ts)
						valsForScore = append(valsForScore, &ScoreCounter{
							val:           val,
							w:             w,
							d:             dFlo,
							maxWin:        maxWinFlo,
							maxWinNoNorm:  maxWinFlo,
							maxLossNoNorm: maxLossFlo,

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
						// We don't want to know the
						// max loss for bad points,
						// since it is going to be used
						// as boundary to sell
						maxLossNoNorm: 0,

						charAskMin:  charAskMin,
						charAskMax:  charAskMax,
						charAskMean: charAskMean,
						charAskMode: charAskMode,
					})
				}

				// Calculate the scores for Sell
				maxWinFloSell := maxWinSell - 1
				maxLossFloSell := maxLossSell - 1
				if foundSell != -1 {
					if wSell != -1 {
						//log.Debug("New Range:", curr, w/1000000000, float64(found-val.Ts)/1000000000, maxWin)
						dFloSell := float64(foundSell - val.Ts)
						valsForScoreSell = append(valsForScoreSell, &ScoreCounter{
							val:           val,
							w:             wSell,
							d:             dFloSell,
							maxWin:        maxWinFloSell,
							maxWinNoNorm:  maxWinFloSell,
							maxLossNoNorm: maxLossFloSell,

							charAskMin:  charAskMin,
							charAskMax:  charAskMax,
							charAskMean: charAskMean,
							charAskMode: charAskMode,
						})

						if wSell < minMaxWSell[0] {
							minMaxWSell[0] = wSell
						}
						if wSell > minMaxWSell[1] {
							minMaxWSell[1] = wSell
						}
						if dFloSell < minMaxDSell[0] {
							minMaxDSell[0] = dFloSell
						}
						if dFloSell > minMaxDSell[1] {
							minMaxDSell[1] = dFloSell
						}
						if maxWinFloSell < minMaxWinSell[0] {
							minMaxWinSell[0] = maxWinFloSell
						}
						if maxWinFloSell > minMaxWinSell[1] {
							minMaxWinSell[1] = maxWinFloSell
						}
					}
				} else {
					// This is a bad point, we need to keep track of this also
					valsForScore = append(valsForScore, &ScoreCounter{
						val:           val,
						w:             0,
						d:             1,
						maxWin:        maxWinFloSell,
						maxWinNoNorm:  maxWinFloSell,
						maxLossNoNorm: 0,

						charAskMin:  charAskMin,
						charAskMax:  charAskMax,
						charAskMean: charAskMean,
						charAskMode: charAskMode,
					})
				}
			}

			// Now normalise and prepare the scores to Buy
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

			// Now normalise and prepare the scores to Sell
			log.Debug("Normalise:", minMaxWinSell)
			log.Debug("ValsForSell:", valsForScoreSell[:10])
			for i := 0; i < len(valsForScoreSell); i++ {
				if valsForScoreSell[i].w != 0 {
					//log.Debug("Before Normalise:", valsForScoreSell[i])
					valsForScoreSell[i].maxWin = (valsForScoreSell[i].maxWin - minMaxWinSell[0]) / (minMaxWinSell[1] - minMaxWinSell[0])
					valsForScoreSell[i].d = (valsForScoreSell[i].d - minMaxDSell[0]) / (minMaxDSell[1] - minMaxDSell[0])
					valsForScoreSell[i].w = (valsForScoreSell[i].w - minMaxWSell[0]) / (minMaxWSell[1] - minMaxWSell[0])
					if valsForScoreSell[i].maxWin != 0 && valsForScoreSell[i].d != 0 && valsForScoreSell[i].w != 0 {
						valsForScoreSell[i].score = (valsForScoreSell[i].maxWin * valsForScoreSell[i].w) / valsForScoreSell[i].d
						//log.Debug("After Normalise:", valsForScoreSell[i])
					} else {
						valsForScoreSell[i].score = -1
					}
				}
			}

			tr.mutex.Lock()
			tr.centroidsCurr[curr], tr.maxWinByCentroid[curr], tr.maxLossByCentroid[curr], tr.centroidsForAsk[curr] = tr.getCentroids(valsForScore)
			log.Debug("Centroides to buy:", curr, tr.centroidsForAsk[curr], "Max loss:", tr.maxLossByCentroid[curr])
			tr.centroidsCurrSell[curr], tr.maxWinByCentroidSell[curr], tr.maxLossByCentroidSell[curr], tr.centroidsForSell[curr] = tr.getCentroids(valsForScoreSell)
			log.Debug("Centroides to sell:", curr, tr.centroidsForSell[curr], "Max loss:", tr.maxLossByCentroidSell[curr])
			tr.mutex.Unlock()

			currsTrained++
		}(curr, vals)
	}

	for currsTrained < len(tr.feeds) {
		time.Sleep(time.Second)
	}
}

func (tr *TrainerCorrelations) getCentroids(valsForScore ByScore) (centroidsCurr [][]float64, maxWinByCentroid []float64, maxLossByCentroid []float64, centroidsForAsk []int) {
	sort.Sort(valsForScore)

	// Using k-means try to find 2 centroids:
	//  - Buy
	//  - Don't buy
	// Identify what centroid is what
	// Check the Precission and recall obtained using this 2 centroids

	// Init the clusters centroids
	log.Debug("Moving centroids")
	for c := 0; c < clusters; c++ {
		pos := c * ((len(valsForScore) - 1) / (clusters - 1))
		centroidsCurr = append(centroidsCurr, []float64{
			valsForScore[pos].charAskMin,  // ask min relation
			valsForScore[pos].charAskMax,  // ask max relation
			valsForScore[pos].charAskMean, // ask mean relation
			valsForScore[pos].charAskMode, // ask mode relation
		})
	}

	modified := true
	for modified {
		scoresByCentroid := make([][]*ScoreCounter, clusters)
		for _, score := range valsForScore {
			centroid := tr.getClosestCentroid(score, centroidsCurr)
			scoresByCentroid[centroid] = append(scoresByCentroid[centroid], score)
		}

		for i := 0; i < clusters; i++ {
			log.Debug("Items centroid:", i, len(scoresByCentroid[i]))
		}

		// Move the centroids
		modified = false
		for c := 0; c < clusters; c++ {
			log.Debug("scoresByCentroid:", c, len(scoresByCentroid[c]))
			oldCentroid := centroidsCurr[c]
			centroidsCurr[c] = []float64{
				0.0,
				0.0,
				0.0,
				0.0,
			}
			scoresCentroid := float64(len(scoresByCentroid[c]))
			for _, score := range scoresByCentroid[c] {
				centroidsCurr[c][0] += score.charAskMin / scoresCentroid
				centroidsCurr[c][1] += score.charAskMax / scoresCentroid
				centroidsCurr[c][2] += score.charAskMean / scoresCentroid
				centroidsCurr[c][3] += score.charAskMode / scoresCentroid
			}

			// Check if the centroid was moved or not
			for i := 0; i < len(oldCentroid); i++ {
				if oldCentroid[i] != centroidsCurr[c][i] {
					modified = true
				}
			}
			log.Debug("Centroids:", c, oldCentroid, centroidsCurr[c])
		}
	}

	// With the clsters initted, try to estimate the score by centroid
	avgScoreCent := make([]float64, clusters)
	avgMaxWin := make([]float64, clusters)
	maxWinByCentroid = make([]float64, clusters)
	maxLossByCentroid = make([]float64, clusters)
	scoresByCentroid := make(map[int][]*ScoreCounter)
	for _, score := range valsForScore {
		centroid := tr.getClosestCentroid(score, centroidsCurr)
		avgScoreCent[centroid] += score.score
		avgMaxWin[centroid] += score.maxWinNoNorm
		maxWinByCentroid[centroid] += score.maxWinNoNorm

		if maxLossByCentroid[centroid] > score.maxLossNoNorm {
			maxLossByCentroid[centroid] = score.maxLossNoNorm
		}
		if _, ok := scoresByCentroid[centroid]; ok {
			scoresByCentroid[centroid] = append(scoresByCentroid[centroid], score)
		} else {
			scoresByCentroid[centroid] = []*ScoreCounter{score}
		}
	}

	// Try to reduce the max loss until contain the cMinLossRange points
	// inside the range
	for c, scores := range scoresByCentroid {
		usedScores := len(scores)
		valsToInclude := int(float64(len(scores)) * cMinLossRange)
		for usedScores != valsToInclude && usedScores > valsToInclude {
			maxLossByCentroid[c] *= 0.99
			usedScores = 0
			for _, score := range scores {
				if maxLossByCentroid[c] < score.maxLossNoNorm {
					usedScores++
				}
			}
		}

		log.Debug("Used Scores for MinLoss:", usedScores, "of:", len(scores), "Perc:", float64(usedScores)/float64(len(scores)), "Max Loss:", maxLossByCentroid[c])
	}

	scores := make([]float64, clusters)
	for c := 0; c < clusters; c++ {
		maxWinByCentroid[c] /= math.Abs(float64(len(scoresByCentroid[c])))
		avgScoreCent[c] /= float64(len(scoresByCentroid[c]))
		avgMaxWin[c] /= float64(len(scoresByCentroid[c]))
		scores[c] = avgMaxWin[c]
		log.Debug("Centroid:", c, "Items:", len(scoresByCentroid[c]), "Score", avgScoreCent[c], "Avg MaxWin:", avgMaxWin[c], "CentroidPos:", centroidsCurr[c])
	}

	sort.Float64s(scores)

	centroidsForAsk = []int{}
	for _, score := range scores[clusters-clustersToUse:] {
		for k, v := range avgMaxWin {
			if v == score {
				centroidsForAsk = append(centroidsForAsk, k)
			}
		}
	}

	return
}

func (tr *TrainerCorrelations) getClosestCentroid(score *ScoreCounter, centroidsCurr [][]float64) (c int) {
	c = 0
	minDist := math.Inf(1)
	for ci, centroid := range centroidsCurr {
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

func (tr *TrainerCorrelations) ShouldIOperate(curr string, val *charont.CurrVal, vals []*charont.CurrVal, traderID int) (operate bool, typeOper string) {
	charAskMin, charAskMax, charAskMean, charAskMode, noPossibleToStudy, _ := tr.getPointCharacteristics(val, vals)
	if noPossibleToStudy {
		return false, ""
	}

	if traderID >= TrainersToRun/2 {
		// Buy?
		traderCentroid := (traderID - (TrainersToRun / 2)) / clustersToUse
		centroid := tr.getClosestCentroid(&ScoreCounter{
			val:         val,
			charAskMin:  charAskMin,
			charAskMax:  charAskMax,
			charAskMean: charAskMean,
			charAskMode: charAskMode,
		}, tr.centroidsCurrSell[curr])

		if tr.centroidsForAsk[curr][traderCentroid] == centroid {
			return true, "buy"
		}
	} else {
		// Sell?
		traderCentroid := traderID / clustersToUse
		centroid := tr.getClosestCentroid(&ScoreCounter{
			val:         val,
			charAskMin:  charAskMin,
			charAskMax:  charAskMax,
			charAskMean: charAskMean,
			charAskMode: charAskMode,
		}, tr.centroidsCurr[curr])

		if tr.centroidsForSell[curr][traderCentroid] == centroid {
			return true, "sell"
		}
	}

	return false, ""
}

func (tr *TrainerCorrelations) ShouldIClose(curr string, currVal, askVal *charont.CurrVal, vals []*charont.CurrVal, traderID int, ord *charont.Order) bool {
	var centroid, traderCentroid int
	var currentWin float64

	traderAvgDiv := float64(traderID % clustersToUse)
	secondsUsed := (currVal.Ts - askVal.Ts) / tsMultToSecs

	if secondsUsed > secsToWaitUntilForceSell {
		// Out of time...
		log.Debug("Selling by time: Centroid:", traderCentroid, secondsUsed, secsToWaitUntilForceSell)
		return true
	}

	if ord.Type == "buy" {
		traderCentroid = (traderID - (TrainersToRun / 2)) / clustersToUse
		currentWin = (currVal.Bid / ord.Price) - 1
		centroid = tr.centroidsForAsk[curr][traderCentroid]

		if currentWin > tr.maxWinByCentroid[curr][centroid]/traderAvgDiv {
			// More than the AVG profit/3
			log.Debug("Selling by profit > avg/", traderAvgDiv, ", Centroid:", traderCentroid, "Profit:", currentWin, "Avg:", tr.maxWinByCentroid[curr][tr.centroidsForAsk[curr][traderCentroid]])
			return true
		}

		if currentWin < tr.maxLossByCentroid[curr][traderCentroid] {
			log.Debug("Selling by max loss, loss:", currentWin, "Centroid:", traderCentroid, "Max Loss:", maxLoss, "Max loss Avg by centroid:", tr.maxLossByCentroid[curr][traderCentroid])
			return true
		}
	} else {
		traderCentroid = traderID / clustersToUse
		currentWin = (ord.CloseRate / currVal.Ask) - 1
		centroid = tr.centroidsForSell[curr][traderCentroid]

		if currentWin > tr.maxWinByCentroidSell[curr][centroid]/traderAvgDiv {
			// More than the AVG profit/3
			log.Debug("Selling by profit > avg/", traderAvgDiv, ", Centroid:", traderCentroid, "Profit:", currentWin, "Avg:", tr.maxWinByCentroidSell[curr][tr.centroidsForAsk[curr][traderCentroid]])
			return true
		}

		if currentWin < tr.maxLossByCentroidSell[curr][traderCentroid] {
			log.Debug("Selling by max loss, loss:", currentWin, "Centroid:", traderCentroid, "Max Loss:", maxLoss, "Max loss Avg by centroid:", tr.maxLossByCentroidSell[curr][traderCentroid])
			return true
		}
	}

	if secondsUsed > secsToWaitUntilForceSell/2 && currentWin > 0 {
		// More than the half of the time and some profit
		log.Debug("Selling by profit > 1 and time > totalTime/2, secs used", secondsUsed, "Centroid:", traderCentroid, "Secs to wait:", secsToWaitUntilForceSell/2, "Profit:", currentWin, "Avg:", tr.maxWinByCentroid[curr][centroid])
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
