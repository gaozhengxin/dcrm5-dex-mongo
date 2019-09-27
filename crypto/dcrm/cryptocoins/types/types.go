package types

import "math/big"

type TxOutput struct {
	ToAddress string
	Amount *big.Int
}

type Value struct {
	Cointype string
	Val *big.Int
}

type Balance struct {
	CoinBalance Value
	TokenBalance Value
}

