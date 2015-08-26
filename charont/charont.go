package charont

type Price struct {
	Ts   int64
	Buy  float64
	Sell float64
}

type Order struct {
	Id    int64
	Price float64
}

type Int interface {
	GetBaseCurrency() string
	GetCurrencies() []string
	GetRange(currency string, from, to int64) []*Price
	Buy(currency string, units int, bound int64) (order *Order, err error)
	Sell(currency string, units int, bound int64) (order *Order, err error)
	CloseAllOpenOrders()
}

type OrderInt interface {
	Close() (rate float64, profit float64, err error)
}
