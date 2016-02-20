package philoctetes

import "github.com/alonsovidales/v/charont"

type TrainerInt interface {
	studyCurrencies(TimeRangeToStudySecs int64)
	ShouldIOperate(curr string, val *charont.CurrVal, vals []*charont.CurrVal, traderID int) (operate bool, typeOper string)
	ShouldIClose(curr string, currVal, askVal *charont.CurrVal, vals []*charont.CurrVal, traderID int, ord *charont.Order) bool
}
