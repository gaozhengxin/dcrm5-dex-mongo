package main

import (
	"bytes"
	//"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"github.com/fatih/color"
	"github.com/fusion/go-fusion/common"
	"github.com/fusion/go-fusion/common/hexutil"
	"github.com/fusion/go-fusion/core/types"
	cnsl "github.com/fusion/go-fusion/console"
	glog "github.com/fusion/go-fusion/log"
	"github.com/fusion/go-fusion/mongodb"
	"github.com/fusion/go-fusion/rpc"
)

func init() {
	color.NoColor = true
	ipcInit()
	mongodb.MongoInit()
	mongodb.Mongo = true
	glog.Root().SetHandler(glog.LvlFilterHandler(glog.LvlInfo, glog.StreamHandler(os.Stderr, glog.TerminalFormat(true))))
	//glog.Root().SetHandler(glog.LvlFilterHandler(glog.LvlDebug, glog.StreamHandler(os.Stderr, glog.TerminalFormat(true))))
}


func mainxxx() {
	//n, e := blockNumber()
	//fmt.Printf("number: %v, error: %v\n", n, e)

	//block, e := getBlock(12006)
	//fmt.Printf("block: %+v,\nerror: %v\n", block, e)
	//h := block.Header()
	//fmt.Printf("\n==========\n\n")
	//fmt.Printf("ParentHash: %v\n", h.ParentHash.Hex())
	//fmt.Printf("UncleHash: %v\n", h.UncleHash.Hex())
	//fmt.Printf("Coinbase: %v\n", h.Coinbase.Hex())
	//fmt.Printf("Root: %v\n", h.Root.Hex())
	//fmt.Printf("TxHash: %v\n", h.TxHash.Hex())
	//fmt.Printf("ReceiptHash: %v\n", h.ReceiptHash.Hex())
	//fmt.Printf("Bloom: %v\n", h.Bloom)
	//fmt.Printf("Difficulty: %v\n", h.Difficulty)
	//fmt.Printf("Number: %v\n", h.Number)
	//fmt.Printf("GasLimit: %v\n", h.GasLimit)
	//fmt.Printf("GasUsed: %v\n", h.GasUsed)
	//fmt.Printf("Time: %v\n", h.Time)
	//fmt.Printf("Extra: %v\n", hex.EncodeToString(h.Extra))
	//fmt.Printf("MixDigest: %v\n", h.MixDigest.Hex())
	//fmt.Printf("Nonce: %v\n", h.Nonce.Uint64)
	//fmt.Printf("Size: %v\n", h.Size())

	tx, e := getTransactionFromBlock(uint64(432), 0)
	fmt.Printf("transactions: %+v\nerror: %v\n", tx, e)

	//tx, e := getTransactionFromBlock(uint64(12006), 0)
	//fmt.Printf("transactions: %+v\nerror: %v\n", tx, e)
	//tx, e = getTransactionFromBlock(uint64(12006), 1)
	//fmt.Printf("transactions: %+v\nerror: %v\n", tx, e)
	//tx, e = getTransactionFromBlock(uint64(12006), 2)
	//fmt.Printf("transactions: %+v\nerror: %v\n", tx, e)

	//tx, e := getTransaction("0x39b02f073e77167d108b3f0f7418a0c5626365c0baf6f5544d50a2381a4ff5c8")
	//fmt.Printf("transaction: %+v\nerror: %v\n", tx, e)
}

func main() {
	Sync()
}

func Sync() {
	ch := make(chan uint64, 1)

	go func() {
		ticker := time.NewTicker(time.Second)
		for {
			select {
			case <-ticker.C:
				height, e := blockNumber()
				if e == nil {
					ch <- height
				} else {
					log.Print(e.Error())
				}
			}
		}
	}()

	var head uint64 = 0
	var total uint64 = 0
	go func() {
		for {
			fmt.Fprintf(os.Stdout, "%v/%v\r", head, total)
		}
	} ()
	for {
		select {
		case height := <-ch:
			total = height
			for head < height {
				block, e := getBlock(head + 1)
				if e != nil {
					glog.Warn("get block error", "block number", head, "error", e)
					head++
					continue
				}
				glog.Debug("sync", "block number", head, "block", fmt.Sprintf("%+v", block))
				txs, e := getBlockTransactions(head)
				if e != nil {
					glog.Warn("get block transactions error", "block number", head, "error", e)
					head++
					continue
				}
				glog.Debug("sync", "block number", head, "transactions", fmt.Sprintf("%+v", txs))
				if mongodb.Mongo {
					mongodb.LightSync(block, txs)
				}
				head++
			}
		}
	}
}

var endpoint = "/home/ezreal/fsn_mongo/node1/gdcrm.ipc"

var (
	console *cnsl.Console
	printer *bytes.Buffer
	rw sync.RWMutex
)

var (
	NilConsoleErr error = fmt.Errorf("no ipc console")
	UnmarshalRPCBlockErr = func(w string) error {return fmt.Errorf("unmarshal rpc block error: %v", w)}
	RPCBlockHashErr error = fmt.Errorf("rcp block hash not match")
	NoTxErr error = fmt.Errorf("transaction not found")
	UnmarshalTxErr = func(w string) error {return fmt.Errorf("unmarshal transaction error: %v", w)}
	GetBlockTransactionsErr error = fmt.Errorf("get block transactions error")
)

var (
	eth_blockNumber = `eth.blockNumber`
	eth_getBlock = `JSON.stringify(eth.getBlock(%v))`
	eth_getTransactionFromBlock = `JSON.stringify(eth.getTransactionFromBlock("%v", %v))`
	eth_getTransaction = `JSON.stringify(eth.getTransaction("%v"))`
	eth_getBlockTransactionCount =`eth.getBlockTransactionCount(%v)`
)

func ipcInit() {
	client, err := rpc.Dial(endpoint)
	if err != nil {
		log.Fatal(err)
		return
	}

	s := ""
	printer = bytes.NewBufferString(s)

	cfg := cnsl.Config{
		DataDir: "/home/ezreal/fsn_mongo/ipccli",
		DocRoot: "/home/ezreal/fsn_mongo/ipccli",
		Client: client,
		Printer: printer,
	}

	console, err = cnsl.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
}

func getBlockTransactions(number uint64) ([]*types.Transaction, error) {
	count, err := getBlockTransactionCount(number)
	if err != nil {
		return nil, GetBlockTransactionsErr
	}

	var txs []*types.Transaction
	for i := 0; i < count; i++ {
		tx, e := getTransactionFromBlock(number, i)
		if e != nil {
			if err != nil {
				err = fmt.Errorf("%v: tx; %v; %v", err.Error(), strconv.Itoa(i), e.Error())
			} else {
				err = fmt.Errorf("tx; %v; ", strconv.Itoa(i), e.Error())
			}
			continue
		}
		if tx != nil {
			txs = append(txs, tx)
		}
	}
	return txs, err
}

func blockNumber() (uint64, error) {
	if console == nil {
		return 0, NilConsoleErr
	}
	rw.Lock()
	defer rw.Unlock()

	printer.Reset()
	console.Evaluate(eth_blockNumber)
	ret := printer.String()
	printer.Reset()

	ret = strings.TrimSuffix(ret, "\n")
	return strconv.ParseUint(ret, 10, 64)
}

func getBlockTransactionCount(number uint64) (int, error) {
	if console == nil {
		return 0, NilConsoleErr
	}
	rw.Lock()
	defer rw.Unlock()

	printer.Reset()
	code := fmt.Sprintf(eth_getBlockTransactionCount, number)
	console.Evaluate(code)
	ret := printer.String()
	printer.Reset()

	ret = strings.TrimSuffix(ret, "\n")
	return strconv.Atoi(ret)
}

func getBlock(number uint64) (*types.Block, error) {
	if console == nil {
		return nil, NilConsoleErr
	}
	rw.Lock()
	defer rw.Unlock()

	printer.Reset()
	code := fmt.Sprintf(eth_getBlock, number)
	console.Evaluate(code)
	ret := printer.String()
	printer.Reset()

	trimL := func (l rune) bool {return l != '{'}
	trimR := func (l rune) bool {return l != '}'}
	ret = strings.TrimLeftFunc(ret, trimL)
	ret = strings.TrimRightFunc(ret, trimR)
	ret = strings.Replace(ret, "\\", "", -1)

	return UnmarshalRPCBlock(ret)
}

func getTransactionFromBlock(block uint64, number int) (*types.Transaction, error) {
	if console == nil {
		return nil, NilConsoleErr
	}
	rw.Lock()
	defer rw.Unlock()

	printer.Reset()
	code := fmt.Sprintf(eth_getTransactionFromBlock, block, number)
	console.Evaluate(code)
	ret := printer.String()
	printer.Reset()

	trimL := func (l rune) bool {return l != '{'}
	trimR := func (l rune) bool {return l != '}'}
	ret = strings.TrimLeftFunc(ret, trimL)
	ret = strings.TrimRightFunc(ret, trimR)
	ret = strings.Replace(ret, "\\", "", -1)
	return UnmarshalTx(ret)
}

func getTransaction(hash string) (*types.Transaction, error) {
	if console == nil {
		return  nil, NilConsoleErr
	}
	rw.Lock()
	defer rw.Unlock()

	printer.Reset()
	code := fmt.Sprintf(eth_getTransaction, hash)

	console.Evaluate(code)
	ret := printer.String()
	printer.Reset()

	trimL := func (l rune) bool {return l != '{'}
	trimR := func (l rune) bool {return l != '}'}
	ret = strings.TrimLeftFunc(ret, trimL)
	ret = strings.TrimRightFunc(ret, trimR)
	ret = strings.Replace(ret, "\\", "", -1)
	return UnmarshalTx(ret)
}

func UnmarshalTx(input string) (tx *types.Transaction, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = UnmarshalTxErr(fmt.Sprintf("%+v", r))
		}
	}()

	if input == "" {
		return nil, NoTxErr
	}

	tmp := make(map[string]interface{})
	err = json.Unmarshal([]byte(input), &tmp)
	if err != nil {
		return nil, UnmarshalTxErr(err.Error() + "    " + input)
	}
	if tmp["gas"] != nil {
		gas := tmp["gas"].(float64)
		tmp["gas"] = hexutil.EncodeUint64(uint64(gas))
	}
	if tmp["gasPrice"] != nil {
		gasPrice := tmp["gasPrice"].(string)
		pbig, _ := new(big.Int).SetString(gasPrice, 10)
		tmp["gasPrice"] = hexutil.EncodeBig(pbig)
	}
	if tmp["nonce"] != nil {
		nonce := tmp["nonce"].(float64)
		tmp["nonce"] = hexutil.EncodeUint64(uint64(nonce))
	}
	if tmp["value"] != nil {
		value := tmp["value"].(string)
		vf, _ := strconv.ParseFloat(value, 64)
		vs := strconv.FormatFloat(vf, 'f', -1, 64)
		vbig, _ := new(big.Int).SetString(vs, 10)
		tmp["value"] = hexutil.EncodeBig(vbig)
	}
	tmp2, err := json.Marshal(tmp)
	if err != nil {
		return nil, UnmarshalTxErr(err.Error() + "    " + input)
	}

	input = string(tmp2)

	tx = new(types.Transaction)
	err = tx.UnmarshalJSON([]byte(input))
	if err != nil {
		return nil, UnmarshalTxErr(err.Error() + "    " + input)
	}
	return tx, nil
}

func UnmarshalRPCBlock(input string) (blk *types.Block, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = UnmarshalRPCBlockErr(fmt.Sprintf("%+v", r))
		}
	}()

	rpcblk := new(RPCBlock)
	err = json.Unmarshal([]byte(input), rpcblk)
	if err != nil {
		err = UnmarshalRPCBlockErr(err.Error() + "    " + input)
		return nil, err
	}

	header := &types.Header{
		Number: big.NewInt(rpcblk.Number),
		ParentHash: rpcblk.ParentHash,
		MixDigest: rpcblk.MixHash,
		UncleHash: rpcblk.Sha3Uncles,
		Bloom: rpcblk.LogsBloom,
		Root: rpcblk.StateRoot,
		Coinbase: rpcblk.Miner,
		Extra: []byte(*rpcblk.ExtraData),
		GasLimit: rpcblk.GasLimit,
		GasUsed: rpcblk.GasUsed,
		Time: big.NewInt(rpcblk.Timestamp),
		TxHash: rpcblk.TransactionsRoot,
		ReceiptHash: rpcblk.ReceiptsRoot,
	}

	difficulty, ok := new(big.Int).SetString(rpcblk.Difficulty, 10)
	if !ok {
		err = UnmarshalRPCBlockErr("parse difficulty number error")
	}
	header.Difficulty = difficulty

	nonce := new(types.BlockNonce)
	err = nonce.UnmarshalText([]byte(rpcblk.Nonce))
	if err != nil {
		err = UnmarshalRPCBlockErr("nonce unmarshal text: " + err.Error())
		return
	}
	header.Nonce = *nonce

	//blk = types.NewBlock(header, nil, nil, nil)
	blk = types.NewBlockWithHeader(header)

	if hash := blk.Hash(); hash != rpcblk.Hash {
		fmt.Println(hash.Hex())
		fmt.Println(rpcblk.Hash.Hex())
		err = RPCBlockHashErr
	}
	return
}

type RPCBlock struct {
	Number            int64
	Hash              common.Hash
	ParentHash        common.Hash
	Nonce             string
	MixHash           common.Hash
	Sha3Uncles        common.Hash
	LogsBloom         types.Bloom
	StateRoot         common.Hash
	Miner             common.Address
	Difficulty        string
	ExtraData         *hexutil.Bytes
	Size              uint64
	GasLimit          uint64
	GasUsed           uint64
	Timestamp         int64
	TransactionsRoot  common.Hash
	ReceiptsRoot      common.Hash
	Transactions      []common.Hash
	Uncles            []common.Hash
}
