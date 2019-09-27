package orderbook

import (
	"github.com/fusion/go-fusion/common"
	//"github.com/shopspring/decimal"
	"encoding/json"
)

type XvcOrder struct {
	Id        string
	From common.Address
	Trade string //ETH/BTC
	Ordertype string //LimitOrder
	Side      string //Buy/Sell
	Timestamp int64 
	//Quantity  decimal.Decimal
	//Price     decimal.Decimal
	Quantity  string
	Price     string
	Rule string //GTE/IOC
}

func NewXvcOrder(id string, from common.Address,trade string,ordertype string,side string,timestamp int64, quantity string, price string,rule string) *XvcOrder {
	return &XvcOrder{
		Id:        id,
		From:	from,
		Trade:	trade,
		Ordertype:ordertype,
		Side:      side,
		Timestamp: timestamp,
		Quantity:  quantity,
		Price:     price,
		Rule:rule,
	}
}

func (o *XvcOrder) Copy() *XvcOrder {
    return &XvcOrder{
		Id:     o.ID(),
		From:	o.FROM(),
		Trade:	o.TRADE(),
		Ordertype:o.ORDERTYPE(),
		Side:      o.SIDE(),
		Timestamp: o.TIME(),
		Quantity:  o.QUANTITY(),
		Price:     o.PRICE(),
		Rule:	o.RULE(),
	}
}

func (o *XvcOrder) ID() string {
	return o.Id
}

func (o *XvcOrder) FROM() common.Address {
	return o.From
}

func (o *XvcOrder) TRADE() string {
	return o.Trade
}

func (o *XvcOrder) ORDERTYPE() string {
	return o.Ordertype
}

func (o *XvcOrder) SIDE() string {
	return o.Side
}

func (o *XvcOrder) TIME() int64 {
	return o.Timestamp
}

func (o *XvcOrder) QUANTITY() string {
	return o.Quantity
}

func (o *XvcOrder) PRICE() string {
	return o.Price
}

func (o *XvcOrder) RULE() string {
	return o.Rule
}

// MarshalJSON implements json.Marshaler interface
func (o *XvcOrder) MarshalJSON() ([]byte, error) {
	return json.Marshal(
		&struct {
			ID        string          `json:"id"`
			Fr        common.Address  `json:"from"`
			Tr        string          `json:"trade"`
			ODT        string          `json:"ordertype"`
			S         string          `json:"side"`
			Timestamp int64           `json:"timestamp"`
			Quantity  string `json:"quantity"`
			Price     string `json:"price"`
			R         string          `json:"rule"`
		}{
			ID:        o.ID(),
			Fr:        o.FROM(),
			Tr:	   o.TRADE(),
			ODT:	   o.ORDERTYPE(),
			S:         o.SIDE(),
			Timestamp: o.TIME(),
			Quantity:  o.QUANTITY(),
			Price:     o.PRICE(),
			R:        o.RULE(),
		},
	)
}

// UnmarshalJSON implements json.Unmarshaler interface
func (o *XvcOrder) UnmarshalJSON(data []byte) error {
	obj := struct {
			ID        string          `json:"id"`
			Fr        common.Address  `json:"from"`
			Tr        string          `json:"trade"`
			ODT        string          `json:"ordertype"`
			S         string          `json:"side"`
			Timestamp int64           `json:"timestamp"`
			Quantity  string `json:"quantity"`
			Price     string `json:"price"`
			R         string          `json:"rule"`
	}{}

	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}

	o.Id = obj.ID
	o.From = obj.Fr
	o.Trade = obj.Tr
	o.Ordertype = obj.ODT
	o.Side = obj.S
	o.Timestamp = obj.Timestamp
	o.Quantity = obj.Quantity
	o.Price = obj.Price
	o.Rule = obj.R
	return nil
}

