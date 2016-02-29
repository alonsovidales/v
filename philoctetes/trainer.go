package philoctetes

import "github.com/alonsovidales/v/charont"

type TrainerInt interface {
	ShouldIOperate(curr string, vals map[string][]*charont.CurrVal, traderID int) (operate bool, typeOper string)
	ShouldIClose(curr string, askVal *charont.CurrVal, vals map[string][]*charont.CurrVal, traderID int, ord *charont.Order) bool
}
