package charont

type Price struct {
	Ts   int64
	Buy  float64
	Sell float64
}

type Order struct {
	Id        int64
	Price     float64
	Type      string
	Open      bool
	Profit    float64
	CloseRate float64
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
	Buy(currency string, units int, bound float64) (order *Order, err error)
	Sell(currency string, units int, bound float64) (order *Order, err error)
	CloseOrder(ord *Order) (err error)
	CloseAllOpenOrders()
}

type OrderInt interface {
	Close() (rate float64, profit float64, err error)
}
