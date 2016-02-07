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
	winSizeSecs       = 900 * tsMultToSecs
	minPointsInWindow = 1000
	clusters          = 20
)

type TrainerCorrelations struct {
	feeds           map[string][]*charont.CurrVal
	centroidsCurr   map[string][][]float64
	centroidsForAsk map[string]map[int]bool
}

type ScoreCounter struct {
	val *charont.CurrVal
	w   float64
	d   float64

	charAskMin  float64
	charAskMax  float64
	charAskMean float64
	charAskMode float64

	maxWin   float64
	score    float64
	centroid int
}
type ByScore []*ScoreCounter

func (a ByScore) Len() int           { return len(a) }
func (a ByScore) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByScore) Less(i, j int) bool { return a[i].score > a[j].score }

func GetTrainerCorrelations(trainingFile string, TimeRangeToStudySecs int64) TrainerInt {
	log.Debug("Initializing trainer...")

	TimeRangeToStudySecs *= tsMultToSecs
	feedsFile, err := os.Open(trainingFile)
	if err != nil {
		log.Fatal("Problem reading the logs file")
	}

	scanner := bufio.NewScanner(feedsFile)

	feeds := &TrainerCorrelations{
		feeds:           make(map[string][]*charont.CurrVal),
		centroidsCurr:   make(map[string][][]float64),
		centroidsForAsk: make(map[string]map[int]bool),
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
		lastWindowFirstPosUsed := 0
	pointToStudyLoop:
		for i, val := range vals {
			if i%1000 == 0 {
				log.Debug("Processed:", i, "points for currency:", curr)
			}
			charAskMin, charAskMax, charAskMean, charAskMode, noPossibleToStudy, firstWindowPos := tr.getPointCharacteristics(val, vals[lastWindowFirstPosUsed:i])
			if noPossibleToStudy {
				continue pointToStudyLoop
			}
			lastWindowFirstPosUsed = firstWindowPos

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
				valsForScore[curr] = append(valsForScore[curr], &ScoreCounter{
					val:    val,
					w:      0,
					d:      1,
					maxWin: 0,

					charAskMin:  charAskMin,
					charAskMax:  charAskMax,
					charAskMean: charAskMean,
					charAskMode: charAskMode,
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

		// Init the clusters centroids
		log.Debug("Moving centroids")
		for c := 0; c < clusters; c++ {
			pos := c * (len(valsForScore[curr]) / (clusters - 1))
			tr.centroidsCurr[curr] = append(tr.centroidsCurr[curr], []float64{
				valsForScore[curr][pos].charAskMin,  // ask min relation
				valsForScore[curr][pos].charAskMax,  // ask max relation
				valsForScore[curr][pos].charAskMean, // ask mean relation
				valsForScore[curr][pos].charAskMode, // ask mode relation
			})
		}

		modified := true
		for modified {
			scoresByCentroid := make([][]*ScoreCounter, clusters)
			for _, score := range valsForScore[curr] {
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
		scoresByCentroid := make([]int, clusters)
		for _, score := range valsForScore[curr] {
			centroid := tr.getClosestCentroid(score, curr)
			avgScoreCent[centroid] += score.score
			scoresByCentroid[centroid]++
		}
		maxScoreCentroid := -1.0
		centroidToUse := 0
		for c := 0; c < clusters; c++ {
			log.Debug("Centroid:", c, "Items:", scoresByCentroid[c], "Score", avgScoreCent[c]/float64(scoresByCentroid[c]), "CentroidPos:", tr.centroidsCurr[curr][c])
			if maxScoreCentroid < avgScoreCent[c]/float64(scoresByCentroid[c]) {
				maxScoreCentroid = avgScoreCent[c] / float64(scoresByCentroid[c])
				centroidToUse = c
			}
		}

		log.Debug("Centroid to use:", centroidToUse)

		tr.centroidsForAsk[curr] = map[int]bool{
			centroidToUse: true,
		}
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
		log.Debug("Dist To cent:", curr, score, ci, centroid, distToCentroid)
		if distToCentroid < minDist {
			minDist = distToCentroid
			c = ci
		}
	}
	log.Debug("Centroid to use:", c)

	return
}

func (tr *TrainerCorrelations) ShouldIBuy(curr string, val *charont.CurrVal, vals []*charont.CurrVal) bool {
	charAskMin, charAskMax, charAskMean, charAskMode, noPossibleToStudy, _ := tr.getPointCharacteristics(val, vals)
	if !noPossibleToStudy {
		return false
	}

	centroid := tr.getClosestCentroid(&ScoreCounter{
		val:         val,
		charAskMin:  charAskMin,
		charAskMax:  charAskMax,
		charAskMean: charAskMean,
		charAskMode: charAskMode,
	}, curr)

	_, ok := tr.centroidsForAsk[curr][centroid]

	return ok
}

func (tr *TrainerCorrelations) ShouldISell(curr string, currVal, askVal *charont.CurrVal, vals []*charont.CurrVal) bool {
	return askVal.Ask < currVal.Bid && vals[len(vals)-1].Bid > currVal.Bid
}

func (tr *TrainerCorrelations) getPointCharacteristics(val *charont.CurrVal, vals []*charont.CurrVal) (charAskMin, charAskMax, charAskMean, charAskMode float64, noPossibleToStudy bool, firstWindowPos int) {
	// Get all the previous points inside the defined
	// window size that will define this point:
	var pointsInRange []*charont.CurrVal

	for i, winVal := range vals {
		if val.Ts-winVal.Ts >= winSizeSecs {
			pointsInRange = vals[i:]
			firstWindowPos = i
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
