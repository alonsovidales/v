package philoctetes

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"strings"
	"sync"

	"github.com/alonsovidales/go_matrix"
	"github.com/alonsovidales/pit/log"
	"github.com/alonsovidales/v/charont"
)

const (
	tsMultToSecsCrossCurr             = 1000000000
	winSizeSecsCrossCurr              = 600 * tsMultToSecsCrossCurr
	minPointsInWindowCrossCurr        = 20
	secsToWaitUntilForceSellCrossCurr = 1200
	noPossibleScore                   = -1000000
	TrainersToRun                     = 100

	valsToStudy = 8
)

type charsScore struct {
	chars     []float64
	scoreBuy  *score
	scoreSell *score
}

type charsScoreSort []*charsScore

type charsByCurr struct {
	vMin       float64
	vMax       float64
	vMean      float64
	vMode      float64
	noPossible bool
	ts         int64
}

type TrainerCorrelationsCrossCurr struct {
	TrainerInt

	feeds map[string][]*charont.CurrVal

	charsByCurr   map[string]*charsByCurr
	locksByCurr   *sync.Mutex
	thetasBuy     map[string][][]float64
	thetasSell    map[string][][]float64
	avgCurrMaxWin map[string]float64
	normalization map[string][][3]float64 // Max Min AVG
	currsPos      []string

	lastPostByCurr          map[string]int
	lasKnownScoreBuyByCurr  map[string]float64
	lasKnownScoreSellByCurr map[string]float64
	lasKnownTsByCurr        map[string]int64
}

type score struct {
	w      int64
	d      int64
	maxWin float64
	score  float64
}

func GetTrainerCorrelationsCrossCurr(trainingFile string, TimeRangeToStudySecs int64) TrainerInt {
	log.Debug("Initializing trainer...")

	TimeRangeToStudySecs *= tsMultToSecsCrossCurr
	feedsFile, err := os.Open(trainingFile)
	log.Debug("File:", trainingFile)
	if err != nil {
		log.Fatal("Problem reading the logs file")
	}
	defer feedsFile.Close()

	scanner := bufio.NewScanner(feedsFile)

	feeds := &TrainerCorrelationsCrossCurr{
		feeds:                   make(map[string][]*charont.CurrVal),
		normalization:           make(map[string][][3]float64),
		thetasBuy:               make(map[string][][]float64),
		thetasSell:              make(map[string][][]float64),
		lastPostByCurr:          make(map[string]int),
		locksByCurr:             new(sync.Mutex),
		lasKnownScoreBuyByCurr:  make(map[string]float64),
		lasKnownScoreSellByCurr: make(map[string]float64),
		lasKnownTsByCurr:        make(map[string]int64),
		charsByCurr:             make(map[string]*charsByCurr),
		avgCurrMaxWin:           make(map[string]float64),
	}

	i := 0
	feedsOrder := []string{}
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
		feedsOrder = append(feedsOrder, curr)
		i++
		if i%10000 == 0 {
			log.Debug("Lines:", i)
		}
	}

	currProgress := make(map[string]int)
	feeds.currsPos = []string{}
	for curr, _ := range feeds.feeds {
		currProgress[curr] = 0
		feeds.currsPos = append(feeds.currsPos, curr)
		//log.Debug("Curr:", curr, "Scores:", len(scores))
	}

	log.Debug("Characteristics calculation...")
	scoresByCurr := make(map[string]charsScoreSort)
	for _, curr := range feeds.currsPos {
		scoresByCurr[curr] = []*charsScore{}

		feeds.lasKnownScoreBuyByCurr[curr] = 0
		feeds.lasKnownScoreSellByCurr[curr] = 0
		feeds.lasKnownTsByCurr[curr] = 0
	}
	for i, curr := range feedsOrder {
		rangesByCurr := make(map[string][]*charont.CurrVal)
		for curr, scores := range feeds.feeds {
			rangesByCurr[curr] = scores[:currProgress[curr]]
		}

		chars, noPossible := feeds.getCharacteristics(rangesByCurr, curr, true)
		if !noPossible {
			chars.scoreBuy = feeds.getScore(feeds.feeds[curr][currProgress[curr]:], true)
			chars.scoreSell = feeds.getScore(feeds.feeds[curr][currProgress[curr]:], false)
			scoresByCurr[curr] = append(scoresByCurr[curr], chars)
		}

		currProgress[curr]++
		if i%10000 == 0 {
			log.Debug("Studied:", i, "of:", len(feedsOrder), "-", (float64(i)/float64(len(feedsOrder)))*100)
		}
		/*if i == 400000 {
			break
		}*/
	}
	log.Debug("Scores calculated, normalazing...")
	feeds.normalizeAndCalcScore(scoresByCurr, true)
	feeds.normalizeAndCalcScore(scoresByCurr, false)
	feeds.calcNormalizationParams(scoresByCurr)
	feeds.normalize(scoresByCurr)

	feeds.prepareThetas(scoresByCurr)

	feeds.lastPostByCurr = make(map[string]int)
	feeds.feeds = make(map[string][]*charont.CurrVal)

	return feeds
}

func (tr *TrainerCorrelationsCrossCurr) prepareThetas(scores map[string]charsScoreSort) {
	for curr, scoresCurr := range scores {
		log.Debug("Calculating Theta for curr:", curr)
		y := make([]float64, len(scoresCurr))
		ySell := make([]float64, len(scoresCurr))
		x := make([][]float64, len(scoresCurr))
		posScores := 0
		negScores := 0
		for i, score := range scoresCurr {
			y[i] = score.scoreBuy.score
			ySell[i] = score.scoreSell.score
			if y[i] > 0 {
				posScores++
			} else {
				negScores++
			}
			x[i] = make([]float64, len(score.chars)+1)
			x[i][0] = 1
			for f := 1; f < len(score.chars)+1; f++ {
				x[i][f] = score.chars[f-1]
			}
		}

		xStr, _ := json.Marshal(x)
		yStr, _ := json.Marshal(y)

		ioutil.WriteFile("tf_training/x_"+curr+".json", xStr, 0644)
		ioutil.WriteFile("tf_training/y_"+curr+".json", yStr, 0644)

		log.Debug("Training dataset for currency:", curr, "dumped")

		// theta = (Xtrans * X)^-1 * Xtans * y

		/*log.Debug("Matrix:", curr, "- X Vals:", x)
		log.Debug("Matrix:", curr, "- Mult TransItself:", mt.Mult(mt.Trans(x), x))
		log.Debug("Matrix:", curr, "- Inv Mult TransItself:", mt.Inv(mt.Mult(mt.Trans(x), x)))
		log.Debug("Matrix:", curr, "- Inv Mult TransItself trans:", mt.Mult(mt.Inv(mt.Mult(mt.Trans(x), x)), mt.Trans(x)))*/

		/*tr.thetasBuy[curr] = mt.Mult(mt.Mult(mt.Inv(mt.Mult(mt.Trans(x), x)), mt.Trans(x)), mt.Trans([][]float64{y}))
		tr.thetasSell[curr] = mt.Mult(mt.Mult(mt.Inv(mt.Mult(mt.Trans(x), x)), mt.Trans(x)), mt.Trans([][]float64{ySell}))
		log.Debug("Curr:", curr, "Pos Scores:", posScores, "NegScores:", negScores)
		log.Debug("Curr:", curr, "ThetaBuy:", tr.thetasBuy[curr])
		log.Debug("Curr:", curr, "ThetaSell:", tr.thetasSell[curr])*/

		// Get the cost for sell and buy just to determine the precission of the model
		/*j := 0.0
		for i, hY := range mt.Mult(x, tr.thetasBuy[curr]) {
			//log.Debug("hy:", hY[0], "y:", y[i])
			j += (hY[0] - y[i]) * (hY[0] - y[i])
		}
		j /= 2 * float64(len(y))
		log.Debug("Test - Curr:", curr, "CostBuy:", j)

		j = 0.0
		for i, hY := range mt.Mult(x, tr.thetasSell[curr]) {
			//log.Debug("hy:", hY[0], "y:", y[i])
			j += (hY[0] - ySell[i]) * (hY[0] - ySell[i])
		}
		j /= 2 * float64(len(ySell))
		log.Debug("Test - Curr:", curr, "CostSell:", j)*/
	}
}

func (tr *TrainerCorrelationsCrossCurr) getValScore(curr string, vals map[string][]*charont.CurrVal) (scoreBuy, scoreSell float64, noPossible bool) {
	// Use a small cache in order to don't need to recalculate the same
	// scores for the same statuses
	tr.locksByCurr.Lock()
	lastValTs := vals[curr][len(vals[curr])-1].Ts
	if lastValTs == tr.lasKnownTsByCurr[curr] {
		scoreBuy = tr.lasKnownScoreBuyByCurr[curr]
		scoreSell = tr.lasKnownScoreSellByCurr[curr]
		tr.locksByCurr.Unlock()
	} else {
		tr.lasKnownTsByCurr[curr] = lastValTs
		tr.locksByCurr.Unlock()

		chars, noPossible := tr.getCharacteristics(vals, curr, false)
		if noPossible {
			tr.locksByCurr.Lock()
			tr.lasKnownScoreBuyByCurr[curr] = noPossibleScore
			tr.lasKnownScoreSellByCurr[curr] = noPossibleScore
			tr.lasKnownTsByCurr[curr] = lastValTs
			tr.locksByCurr.Unlock()

			return 0.0, 0.0, true
		}
		tr.normalizeScoreCharacteristics(chars, curr)
		//log.Debug("Chars:", chars.chars, "Theta:", tr.thetas[curr])

		// Get the current score to buy
		charsBias := [][]float64{append([]float64{1}, chars.chars...)}
		scoreMatrix := mt.Mult(charsBias, tr.thetasBuy[curr])
		scoreBuy = scoreMatrix[0][0]

		// Get the current score to sell
		scoreMatrix = mt.Mult(charsBias, tr.thetasSell[curr])
		scoreSell = scoreMatrix[0][0]

		tr.locksByCurr.Lock()
		tr.lasKnownScoreBuyByCurr[curr] = scoreBuy
		tr.lasKnownScoreSellByCurr[curr] = scoreSell
		tr.lasKnownTsByCurr[curr] = lastValTs
		tr.locksByCurr.Unlock()
	}

	return scoreBuy, scoreSell, scoreBuy == noPossibleScore
}

func (tr *TrainerCorrelationsCrossCurr) ShouldIOperate(curr string, vals map[string][]*charont.CurrVal, traderID int) (operate bool, typeOper string) {
	scoreBuy, scoreSell, noPossible := tr.getValScore(curr, vals)
	if noPossible {
		return false, ""
	}

	boundary := float64((traderID%TrainersToRun)%10) / 10

	if scoreBuy > scoreSell {
		buy := scoreBuy > boundary
		if buy {
			log.Debug("Should I Buy, Trader:", traderID, "curr:", curr, "score:", scoreBuy, "boundary:", boundary)
		}

		return buy, "buy"
	}

	sell := scoreSell > boundary
	if sell {
		log.Debug("Should I Sell, Trader:", traderID, "curr:", curr, "score:", scoreSell, "boundary:", boundary)
	}

	return sell, "sell"
}

func (tr *TrainerCorrelationsCrossCurr) ShouldIClose(curr string, askVal *charont.CurrVal, vals map[string][]*charont.CurrVal, traderID int, ord *charont.Order) bool {
	var score, currentWin float64

	closeOrder := false
	currVal := vals[curr][len(vals[curr])-1]
	scoreBuy, scoreSell, noPossible := tr.getValScore(curr, vals)

	secondsUsed := (currVal.Ts - askVal.Ts) / tsMultToSecs

	if ord.Type == "buy" {
		currentWin = (currVal.Bid / ord.Price) - 1
		score = scoreBuy
	} else {
		currentWin = (ord.CloseRate / currVal.Ask) - 1
		score = scoreSell
	}

	if !noPossible {
		boundary := float64((traderID%TrainersToRun)/10) / 10

		closeOrder = (currentWin > 0 && score < boundary) ||
			(currentWin > tr.avgCurrMaxWin[curr]) ||
			(currentWin < 0 && secondsUsed > secsToWaitUntilForceSellCrossCurr*4) ||
			(secondsUsed > secsToWaitUntilForceSellCrossCurr*8)

		if closeOrder {
			var reason string
			if currentWin > 0 && score < boundary {
				reason = "currentWin > 0 && score < boundary"
			}
			if currentWin > tr.avgCurrMaxWin[curr] {
				reason = "currentWin > tr.avgCurrMaxWin[curr]"
			}
			if currentWin < 0 && secondsUsed > secsToWaitUntilForceSellCrossCurr*4 {
				reason = "currentWin < 0 && secondsUsed > secsToWaitUntilForceSellCrossCurr*4"
			}
			if secondsUsed > secsToWaitUntilForceSellCrossCurr*8 {
				reason = "secondsUsed > secsToWaitUntilForceSellCrossCurr*8"
			}
			log.Debug("Should I Close, Type:", ord.Type, "Trader:", traderID, "curr:", curr, "score:", score, "boundary:", boundary, "Reason:", reason)
		}
	} else {
		closeOrder = (currentWin > tr.avgCurrMaxWin[curr]) ||
			(currentWin < 0 && secondsUsed > secsToWaitUntilForceSellCrossCurr*4) ||
			(secondsUsed > secsToWaitUntilForceSellCrossCurr*8)
		if closeOrder {
			var reason string
			if currentWin > tr.avgCurrMaxWin[curr] {
				reason = "currentWin > tr.avgCurrMaxWin[curr]"
			}
			if currentWin < 0 && secondsUsed > secsToWaitUntilForceSellCrossCurr*4 {
				reason = "currentWin < 0 && secondsUsed > secsToWaitUntilForceSellCrossCurr*4"
			}
			if secondsUsed > secsToWaitUntilForceSellCrossCurr*8 {
				reason = "secondsUsed > secsToWaitUntilForceSellCrossCurr*8"
			}
			log.Debug("Should I Close, Type:", ord.Type, "Trader:", traderID, "curr:", curr, "score:", score, "NoPossible", "Reason:", reason)
		}
	}

	return closeOrder
}

func (tr *TrainerCorrelationsCrossCurr) calcNormalizationParams(scores map[string]charsScoreSort) {
	for _, curr := range tr.currsPos {
		tr.normalization[curr] = make([][3]float64, valsToStudy*len(tr.currsPos))
		// Calc the params to normalize
		for i := 0; i < len(tr.normalization[curr]); i++ {
			tr.normalization[curr][i][0] = math.Inf(-1) // Max
			tr.normalization[curr][i][1] = math.Inf(1)  // Min
		}
		for _, score := range scores[curr] {
			for i := 0; i < len(score.chars); i++ {
				if score.chars[i] > tr.normalization[curr][i][0] {
					tr.normalization[curr][i][0] = score.chars[i]
				}
				if score.chars[i] < tr.normalization[curr][i][1] {
					tr.normalization[curr][i][1] = score.chars[i]
				}
				tr.normalization[curr][i][2] += score.chars[i]
			}
		}
		for i := 0; i < len(tr.normalization[curr]); i++ {
			tr.normalization[curr][i][2] /= float64(len(scores[curr]))
		}
	}
}

func (tr *TrainerCorrelationsCrossCurr) normalizeScoreCharacteristics(score *charsScore, curr string) {
	for i := 0; i < len(score.chars); i++ {
		score.chars[i] = (score.chars[i] - tr.normalization[curr][i][2]) / (tr.normalization[curr][i][0] - tr.normalization[curr][i][1])
	}
}

func (tr *TrainerCorrelationsCrossCurr) normalize(scores map[string]charsScoreSort) {
	// Normalize
	for _, curr := range tr.currsPos {
		for _, score := range scores[curr] {
			tr.normalizeScoreCharacteristics(score, curr)
		}
	}
}

func (tr *TrainerCorrelationsCrossCurr) normalizeValue(val *charsScore) {
	/*type charsScore struct {
		chars []float64
		score *score
	}*/
}

func (tr *TrainerCorrelationsCrossCurr) normalizeAndCalcScore(scores map[string]charsScoreSort, buyScores bool) {
	var scoreToUse *score

	for curr, scoresCurr := range scores {
		maxW := math.Inf(-1)
		minW := math.Inf(1)
		maxD := math.Inf(-1)
		minD := math.Inf(1)
		maxWin := math.Inf(-1)
		minWin := math.Inf(1)
		avgW := 0.0
		avgD := 0.0
		avgWin := 0.0

		scoresUsed := float64(0)
		for _, score := range scoresCurr {
			if buyScores {
				scoreToUse = score.scoreBuy
			} else {
				scoreToUse = score.scoreSell
			}
			if scoreToUse.w == 0 {
				continue
			}

			scoresUsed++
			if float64(scoreToUse.w) > maxW {
				maxW = float64(scoreToUse.w)
			}
			if float64(scoreToUse.w) < minW {
				minW = float64(scoreToUse.w)
			}

			if float64(scoreToUse.d) > maxD {
				maxD = float64(scoreToUse.d)
			}
			if float64(scoreToUse.d) < minD {
				minD = float64(scoreToUse.d)
			}

			if float64(scoreToUse.maxWin) > maxWin {
				maxWin = float64(scoreToUse.maxWin)
			}
			if float64(scoreToUse.maxWin) < minWin {
				minWin = float64(scoreToUse.maxWin)
			}

			avgW += float64(scoreToUse.w)
			avgD += float64(scoreToUse.d)
			avgWin += float64(scoreToUse.maxWin)
		}

		avgW /= scoresUsed
		avgD /= scoresUsed
		avgWin /= scoresUsed
		tr.avgCurrMaxWin[curr] = avgWin
		fmt.Println("AVGS, Curr:", curr, "W:", avgW, "D:", avgD, "Win:", avgWin)

		for _, score := range scoresCurr {
			if buyScores {
				scoreToUse = score.scoreBuy
			} else {
				scoreToUse = score.scoreSell
			}

			if scoreToUse.w != 0 {
				normW := (((float64(scoreToUse.w) - avgW) / (maxW - minW)) + 1) / 2
				normD := (((float64(scoreToUse.d) - avgD) / (maxD - minD)) + 1) / 2
				normMaxWin := (((float64(scoreToUse.maxWin) - avgWin) / (maxWin - minWin)) + 1) / 2

				scoreToUse.score = (normW * normMaxWin) / normD
			} else {
				scoreToUse.score = scoreToUse.maxWin - 1
			}
		}
	}
}

func (tr *TrainerCorrelationsCrossCurr) getValuesToStudy(vals []*charont.CurrVal) (charAskMin, charAskMax, charAskMean, charAskMode float64, noPossibleToStudy bool, firstWindowPos int) {
	if len(vals) == 0 {
		noPossibleToStudy = true
		return
	}
	val := vals[len(vals)-1]
	vals = vals[:len(vals)]

	// Get all the previous points inside the defined
	// window size that will define this point:
	var pointsInRange []*charont.CurrVal

	for i, winVal := range vals {
		if val.Ts-winVal.Ts >= winSizeSecsCrossCurr {
			pointsInRange = vals[i:]
			firstWindowPos = i
		} else {
			break
		}
	}
	if len(pointsInRange) < minPointsInWindowCrossCurr {
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

		modePoint := math.Floor(point.Ask*1000) / 1000
		if _, ok := modeValAsk[modePoint]; ok {
			modeValAsk[modePoint]++
		} else {
			modeValAsk[modePoint] = 1
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

func (tr *TrainerCorrelationsCrossCurr) getCharacteristics(rangesByCurr map[string][]*charont.CurrVal, curr string, training bool) (cs *charsScore, noPossible bool) {
	var lastPossUsed int
	cs = &charsScore{
		chars: make([]float64, len(tr.currsPos)*valsToStudy),
	}
	if len(rangesByCurr[curr]) == 0 {
		noPossible = true
		return
	}

	tr.locksByCurr.Lock()
	var chars *charsByCurr
	var ok bool
	if chars, ok = tr.charsByCurr[curr]; ok && chars.ts == rangesByCurr[curr][len(rangesByCurr[curr])-1].Ts {
		noPossible = chars.noPossible
		lastPossUsed = 0
	} else {
		chars = &charsByCurr{
			ts: rangesByCurr[curr][len(rangesByCurr[curr])-1].Ts,
		}
		chars.vMin, chars.vMax, chars.vMean, chars.vMode, chars.noPossible, lastPossUsed = tr.getValuesToStudy(rangesByCurr[curr][tr.lastPostByCurr[curr]:])
		tr.charsByCurr[curr] = chars
	}

	cs.chars[0] = chars.vMin
	cs.chars[2] = chars.vMax
	cs.chars[4] = chars.vMean
	cs.chars[6] = chars.vMode
	cs.applyMultFeats(0)
	cs.applyMultFeats(2)
	cs.applyMultFeats(4)
	cs.applyMultFeats(6)

	tr.locksByCurr.Unlock()

	if noPossible {
		return
	}
	if training {
		tr.lastPostByCurr[curr] += lastPossUsed
	}
	i := 1
	for _, currToCompare := range tr.currsPos {
		var vMin, vMax, vMean, vMode float64

		if currToCompare == curr {
			continue
		}

		if len(rangesByCurr[currToCompare]) == 0 {
			noPossible = true
			return
		}
		tr.locksByCurr.Lock()
		if chars, ok := tr.charsByCurr[currToCompare]; ok && chars.ts == rangesByCurr[currToCompare][len(rangesByCurr[currToCompare])-1].Ts {
			vMin = chars.vMin
			vMax = chars.vMax
			vMean = chars.vMean
			vMode = chars.vMode
			noPossible = chars.noPossible
			lastPossUsed = 0
		} else {
			vMin, vMax, vMean, vMode, noPossible, lastPossUsed = tr.getValuesToStudy(rangesByCurr[currToCompare][tr.lastPostByCurr[currToCompare]:])

			tr.charsByCurr[currToCompare] = &charsByCurr{
				vMin:       vMin,
				vMax:       vMax,
				vMean:      vMean,
				vMode:      vMode,
				noPossible: noPossible,
				ts:         rangesByCurr[currToCompare][len(rangesByCurr[currToCompare])-1].Ts,
			}
		}
		tr.locksByCurr.Unlock()

		if noPossible {
			return nil, true
		}
		if training {
			tr.lastPostByCurr[currToCompare] += lastPossUsed
		}
		cs.chars[i*valsToStudy] = cs.chars[0] / vMin
		cs.chars[(i*valsToStudy)+2] = cs.chars[2] / vMax
		cs.chars[(i*valsToStudy)+4] = cs.chars[4] / vMean
		cs.chars[(i*valsToStudy)+6] = cs.chars[6] / vMode

		cs.applyMultFeats(i * valsToStudy)
		cs.applyMultFeats((i * valsToStudy) + 2)
		cs.applyMultFeats((i * valsToStudy) + 4)
		cs.applyMultFeats((i * valsToStudy) + 6)
		i++
	}
	//log.Debug("Curr:", curr, "Chars:", i, len(cs.chars), valsToStudy, len(tr.currsPos), cs.chars)

	return
}

func (cs *charsScore) applyMultFeats(pos int) {
	cs.chars[pos+1] = cs.chars[pos] * cs.chars[pos]
	//cs.chars[pos+2] = math.Sqrt(cs.chars[pos])
	//cs.chars[pos+2] = math.Log(cs.chars[pos])
}

func (tr *TrainerCorrelationsCrossCurr) getScore(currSection []*charont.CurrVal, getForBuy bool) (result *score) {
	var currWin float64

	currVal := currSection[0]
	result = &score{
		d:      0,
		w:      0,
		maxWin: 0,
	}

	for i := 1; i < len(currSection); i++ {
		if getForBuy {
			currWin = currVal.Bid / currSection[i].Ask
		} else {
			currWin = currSection[i].Bid / currVal.Ask
		}
		if currWin > 1 {
			if result.d == 0 {
				result.d = (currSection[i].Ts - currVal.Ts) / tsMultToSecsCrossCurr
			}
		} else {
			currW := ((currSection[i].Ts - currVal.Ts) / tsMultToSecsCrossCurr) - result.d
			if result.d != 0 {
				result.w = currW
				break
			}
			if currW > secsToWaitUntilForceSellCrossCurr {
				break
			}
		}
		if currWin > result.maxWin {
			result.maxWin = currWin
		}
	}

	if result.d != 0 {
		result.w = ((currSection[len(currSection)-1].Ts - currVal.Ts) / tsMultToSecsCrossCurr) - result.d
	}

	return
}
