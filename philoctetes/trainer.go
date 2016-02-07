package philoctetes

import "github.com/alonsovidales/v/charont"

type TrainerInt interface {
	studyCurrencies(TimeRangeToStudySecs int64)
	ShouldIBuy(curr string, val *charont.CurrVal, vals []*charont.CurrVal) bool
	ShouldISell(curr string, currVal, askVal *charont.CurrVal, vals []*charont.CurrVal) bool
}
