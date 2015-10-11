package charont

type Price struct {
	Ts   int64
	Buy  float64
	Sell float64
}

type Order struct {
	Id        int64
	Price     float64
	Units     int
	Curr      string
	Real      bool
	Type      string
	Open      bool
	Profit    float64
	CloseRate float64
	Ts        int64
}

type CurrVal struct {
	Bid float64 `json:"b"`
	Ask float64 `json:"a"`
	Ts  int64   `json:"t"`
}

type Int interface {
	GetBaseCurrency() string
	GetCurrencies() []string
	GetRange(currency string, from, to int64) []*CurrVal
	AddListerner(currency string, fn func(currency string))
	Buy(currency string, units int, bound float64, realOps bool) (order *Order, err error)
	Sell(currency string, units int, bound float64, realOps bool) (order *Order, err error)
	CloseOrder(ord *Order) (err error)
	CloseAllOpenOrders()
}

type OrderInt interface {
	Close() (rate float64, profit float64, err error)
}
