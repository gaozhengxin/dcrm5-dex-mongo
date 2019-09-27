package dao

import (
	"encoding/json"
	"strconv"
)

func GetTx(txhash string) *Transaction {
	data := Get(txhash)
	var tx Transaction
	err := json.Unmarshal([]byte(data), &tx)
	if err != nil {
		return nil
	}
	return &tx
}

func PutTx(tx *Transaction) {
	Put(tx.TxHash, tx.String())
}

// 一个EOS transaction可以包含多个transfer, 对应TxOutputs
type Transaction struct {
	TxHash string
	FromAddress string
	TxOutputs []TxOutput
	Fee uint32
	Confirmed bool
}

type TxOutput struct {
	Seq float64
	ToAddress string
	Amount string
}

func (tx *Transaction) String() string {
	b, _ := json.Marshal(tx)
	return string(b)
}

func PutSeq(seq float64) {
	key := strconv.FormatFloat(seq, 'f', -1, 64)
	Put(key, "1")
}

func GetSeq(seq float64) string {
	key := strconv.FormatFloat(seq, 'f', -1, 64)
	return Get(key)
}
