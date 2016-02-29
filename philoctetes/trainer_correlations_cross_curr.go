package philoctetes

import (
	"bufio"
	"encoding/json"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/alonsovidales/go_matrix"
	"github.com/alonsovidales/pit/log"
	"github.com/alonsovidales/v/charont"
)

const (
	tsMultToSecsCrossCurr             = 1000000000
	winSizeSecsCrossCurr              = 7200 * tsMultToSecsCrossCurr
	minPointsInWindowCrossCurr        = 20
	clustersCrossCurr                 = 20
	clustersToUseCrossCurr            = 10
	secsToWaitUntilForceSellCrossCurr = 3600 * 3
	maxLossCrossCurr                  = -0.03
	cMinLossRangeCrossCurr            = 0.80
	NumCurrencies                     = 14
	tradersByCurr                     = 100
	TrainersToRun                     = tradersByCurr * NumCurrencies

	valsToStudy = 4
)

type charsScore struct {
	chars []float64
	score *score
}

type charsScoreSort []*charsScore

func (a charsScoreSort) Len() int           { return len(a) }
func (a charsScoreSort) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a charsScoreSort) Less(i, j int) bool { return a[i].score.score < a[j].score.score }

type TrainerCorrelationsCrossCurr struct {
	TrainerInt

	feeds          map[string][]*charont.CurrVal
	thetas         map[string][][]float64
	normalization  map[string][][3]float64 // Max Min AVG
	valsChars      map[string]charsScoreSort
	currsPos       []string
	lastPostByCurr map[string]int
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

	scanner := bufio.NewScanner(feedsFile)

	feeds := &TrainerCorrelationsCrossCurr{
		feeds:          make(map[string][]*charont.CurrVal),
		normalization:  make(map[string][][3]float64),
		thetas:         make(map[string][][]float64),
		lastPostByCurr: make(map[string]int),
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
	}
	for i, curr := range feedsOrder {
		rangesByCurr := make(map[string][]*charont.CurrVal)
		for curr, scores := range feeds.feeds {
			rangesByCurr[curr] = scores[:currProgress[curr]]
		}

		chars, noPossible := feeds.getCharacteristics(rangesByCurr, curr, true)
		if !noPossible {
			chars.score = feeds.getScore(feeds.feeds[curr][currProgress[curr]:])
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
	feeds.normalizeAndCalcScore(scoresByCurr)
	feeds.calcNormalizationParams(scoresByCurr)
	feeds.normalize(scoresByCurr)

	for _, curr := range feeds.currsPos {
		sort.Sort(scoresByCurr[curr])
	}
	feeds.prepareThetas(scoresByCurr)

	feeds.lastPostByCurr = make(map[string]int)

	return feeds
}

func (tr *TrainerCorrelationsCrossCurr) prepareThetas(scores map[string]charsScoreSort) {
	for curr, scoresCurr := range scores {
		log.Debug("Calculating Theta for curr:", curr)
		y := make([]float64, len(scoresCurr))
		x := make([][]float64, len(scoresCurr))
		for i, score := range scoresCurr {
			y[i] = score.score.score
			x[i] = make([]float64, len(score.chars)+1)
			x[i][0] = 1
			for f := 1; f < len(score.chars)+1; f++ {
				x[i][f] = score.chars[f-1]
			}
		}

		// theta = (Xtrans * X)^-1 * Xtans * y
		tr.thetas[curr] = mt.Mult(mt.Mult(mt.Inv(mt.Mult(mt.Trans(x), x)), mt.Trans(x)), mt.Trans([][]float64{y}))
		log.Debug("Curr:", curr, "Theta:", tr.thetas[curr])

		j := 0.0
		for i, hY := range mt.Mult(x, tr.thetas[curr]) {
			//log.Debug("hy:", hY[0], "y:", y[i])
			j += (hY[0] - y[i]) * (hY[0] - y[i])
		}
		j /= 2 * float64(len(y))
		log.Debug("Test - Curr:", curr, "Cost:", j)
	}
}

func (tr *TrainerCorrelationsCrossCurr) getValScore(curr string, vals map[string][]*charont.CurrVal) (score float64, noPossible bool) {
	chars, noPossible := tr.getCharacteristics(vals, curr, false)
	if noPossible {
		return 0.0, true
	}
	tr.normalizeScoreCharacteristics(chars, curr)
	//log.Debug("Chars:", chars.chars, "Theta:", tr.thetas[curr])

	charsBias := [][]float64{append([]float64{1}, chars.chars...)}
	scoreMatrix := mt.Mult(charsBias, tr.thetas[curr])
	score = scoreMatrix[0][0]

	return score, false
}

func (tr *TrainerCorrelationsCrossCurr) ShouldIOperate(curr string, vals map[string][]*charont.CurrVal, traderID int) (operate bool, typeOper string) {
	score, noPossible := tr.getValScore(curr, vals)
	if noPossible {
		return false, ""
	}

	boundary := float64((traderID%tradersByCurr)%10) / 10

	log.Debug("Should I Buy, Trader:", traderID, "curr:", curr, "score:", score, "boundary:", boundary)

	return score > boundary, "buy"
}

func (tr *TrainerCorrelationsCrossCurr) ShouldIClose(curr string, askVal *charont.CurrVal, vals map[string][]*charont.CurrVal, traderID int, ord *charont.Order) bool {
	currVal := vals[curr][len(vals[curr])-1]
	score, noPossible := tr.getValScore(curr, vals)
	if noPossible {
		return false
	}

	boundary := float64((traderID%tradersByCurr)/10) / 10

	log.Debug("Should I Sell, Trader:", traderID, "curr:", curr, "score:", score, "boundary:", boundary)

	currentWin := (currVal.Bid / ord.Price) - 1
	secondsUsed := (currVal.Ts - askVal.Ts) / tsMultToSecs
	return (currentWin > 0 && (score < boundary || score < 0)) ||
		(secondsUsed > secsToWaitUntilForceSell/2 && currentWin > 0) ||
		(secondsUsed > secsToWaitUntilForceSell)
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

func (tr *TrainerCorrelationsCrossCurr) normalizeAndCalcScore(scores map[string]charsScoreSort) {
	for _, scoresCurr := range scores {
		maxW := math.Inf(-1)
		minW := math.Inf(1)
		avgW := 0.0
		maxD := math.Inf(-1)
		minD := math.Inf(1)
		avgD := 0.0
		maxWin := math.Inf(-1)
		minWin := math.Inf(1)
		avgWin := 0.0

		scoresUsed := 0
		for _, score := range scoresCurr {
			if score.score.w == 0 {
				continue
			}

			avgW += float64(score.score.w)
			avgD += float64(score.score.d)
			avgWin += float64(score.score.maxWin)
			scoresUsed++
			if float64(score.score.w) > maxW {
				maxW = float64(score.score.w)
			}
			if float64(score.score.w) < minW {
				minW = float64(score.score.w)
			}

			if float64(score.score.d) > maxD {
				maxD = float64(score.score.d)
			}
			if float64(score.score.d) < minD {
				minD = float64(score.score.d)
			}

			if float64(score.score.maxWin) > maxWin {
				maxWin = float64(score.score.maxWin)
			}
			if float64(score.score.maxWin) < minWin {
				minWin = float64(score.score.maxWin)
			}
		}

		avgW /= float64(scoresUsed)
		avgD /= float64(scoresUsed)
		avgWin /= float64(scoresUsed)

		for _, score := range scoresCurr {
			if score.score.w != 0 {
				normW := (((float64(score.score.w) - avgW) / (maxW - minW)) + 1) / 2
				normD := (((float64(score.score.d) - avgD) / (maxD - minD)) + 1) / 2
				normMaxWin := (((float64(score.score.maxWin) - avgWin) / (maxWin - minWin)) + 1) / 2

				score.score.score = (normW * normMaxWin) / normD
			} else {
				score.score.score = score.score.maxWin - 1
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
		if val.Ts-winVal.Ts >= winSizeSecs {
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

func (tr *TrainerCorrelationsCrossCurr) getCharacteristics(rangesByCurr map[string][]*charont.CurrVal, curr string, training bool) (cs *charsScore, noPossible bool) {
	var lastPossUsed int
	cs = &charsScore{
		chars: make([]float64, len(tr.currsPos)*valsToStudy),
	}
	cs.chars[0], cs.chars[1], cs.chars[2], cs.chars[3], noPossible, lastPossUsed = tr.getValuesToStudy(rangesByCurr[curr][tr.lastPostByCurr[curr]:])
	if noPossible {
		return
	}
	if training {
		tr.lastPostByCurr[curr] += lastPossUsed
	}
	i := 1
	for _, currToCompare := range tr.currsPos {
		if currToCompare == curr {
			continue
		}
		vMin, vMax, vMean, vMode, noPossible, lastPossUsed := tr.getValuesToStudy(rangesByCurr[currToCompare][tr.lastPostByCurr[currToCompare]:])
		if noPossible {
			return nil, true
		}
		if training {
			tr.lastPostByCurr[currToCompare] += lastPossUsed
		}
		cs.chars[i*valsToStudy] = cs.chars[0] / vMin
		cs.chars[(i*valsToStudy)+1] = cs.chars[1] / vMax
		cs.chars[(i*valsToStudy)+2] = cs.chars[2] / vMean
		cs.chars[(i*valsToStudy)+3] = cs.chars[3] / vMode
		i++
	}

	return
}

func (tr *TrainerCorrelationsCrossCurr) getScore(currSection []*charont.CurrVal) (result *score) {
	currVal := currSection[0]
	result = &score{
		d:      0,
		w:      0,
		maxWin: 0,
	}

	for i := 1; i < len(currSection); i++ {
		currWin := currVal.Bid / currSection[i].Ask
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

/*type charsScore struct {
	chars []float64
	score float64
}

type TrainerCorrelationsCrossCurr struct {
	feeds     map[string][]*charont.CurrVal
	valsChars map[string][]*charsScore
	currsPos  []string
}*/
