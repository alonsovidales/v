package philoctetes

import "github.com/alonsovidales/v/charont"

type TrainerInt interface {
	studyCurrencies(TimeRangeToStudySecs int64)
	GetInfoSection(prices []charont.CurrVal, ask bool) (thrend, avg, min, max, variance, covariance float64)
	ShouldIBuy(curr string, threndOnBuy, averageBuy, priceOnBuy float64) bool
	GetPredictionToSell(Profit float64, Time int64, ThrendOnBuy, ThrendOnSell, AverageBuy, AverageSell, PriceOnBuy, PriceOnSell float64) (pred float64)
}
