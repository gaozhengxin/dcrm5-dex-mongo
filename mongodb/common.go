package mongodb

import (
	"github.com/fusion/go-fusion/core/types"
	"gopkg.in/mgo.v2"
	"sync"
)

var LOGPRINTALL bool = false// true: print all log, false: do nothing

var (
	Mongo bool = false
	MongoSlave bool = false
	MongoIP string = "localhost" // default port: 27017
	URL string
	MatchTrade string = ""
	sep9 string = "dcrmsep9"
	dexDataSplit string = "dccpmatchxvc"

	orderInfo sync.Map
	orderCache sync.Map
	orderCacheInfo sync.Map
	blockSyncInfo sync.Map

	coinmap map[string]int = make(map[string]int)
	blockInfo *BlockInfo
)

var (
	database *mgo.Database

	collectionBlock           *mgo.Collection
	collectionTransaction     *mgo.Collection
	collectionAccount         *mgo.Collection
	collectionDcrmAccount     *mgo.Collection
	collectionTransferHistory *mgo.Collection
	collectionLockinHistory   *mgo.Collection
	collectionLockoutHistory  *mgo.Collection
	collectionTxPool          *mgo.Collection
	//dex
	collectionDexBlock   *mgo.Collection
	collectionDexTx      *mgo.Collection
	collectionOrderCache *mgo.Collection
	collectionOrder      *mgo.Collection
)

var (
	dbname string = "fusion"

	tbBlocks          string = "Blocks"
	tbTransactions    string = "Transactions"
	tbAccounts        string = "Accounts"
	tbDcrmAccounts    string = "DcrmAccounts"
	tbTransferHistory string = "Transfers"
	tbLockinHistory   string = "Lockins"
	tbLockoutHistory  string = "Lockouts"
	//dex
	tbDexBlocks   string = "DexBlocks"
	tbDexTxns     string = "DexTxns"
	tbOrderCaches string = "OrderCaches"
	tbOrders      string = "Orders"

	BlockLock         sync.Mutex
	DexBlockLock      sync.Mutex
	OrderCacheLock    sync.Mutex
	OrderLock         sync.Mutex
	DexTxnsLock       sync.Mutex
	OrderAndCacheLock sync.Mutex

	ConfirmBlockNumber uint64 = 6
	ConfirmBlockTime   uint64 = 0
	latestBlockNumber  uint64 = 0
)

const (
	//ETH, ERC20BNB, ERC20GUSD, ERC20MKR, ERC20HT
	DIVE18 = float64(1000000000000000000)
	//BTC, BCH, USDT
	DIVE8 = float64(100000000)
	//TRX, XRP
	DIVE6 = float64(1000000)
	//EVT1
	DIVE5 = float64(100000)
	//EOS
	DIVE4 = float64(10000)
	//ATOM
	DIVE3 = float64(1000)

	MUL1E9 = 1e9
	MUL1E10 = 1e10

	QuantityAccuracy int = 9
)

const (
	txType_TX = iota
	txType_DCRMTX
	txType_LOCKIN
	txType_LOCKOUT
	txType_DEX
	txType_CONFIRMADDRESS
	txType_ORDER
	txType_NEWTRADE
)

const (
	SUCCESS = iota + 1
	FAILURE

	EXIST = iota + 1
	FINISHED

	INSERT = iota + 1
	UPDATE
)

type mgoBlock struct {
	Key              string          `bson:"_id"`
	Number           uint64          `bson:"number"`
	Hash             string          `bson:"hash"`
	ParentHash       string          `bson:"parentHash"`
	Nonce            string          `bson:"nonce"`
	Sha3Uncles       string          `bson:"sha3Uncles"`
	LogsBloom        string          `bson:"logsBloom"`
	TransactionsRoot string          `bson:"transactionsRoot"`
	StateRoot        string          `bson:"stateRoot"`
	Miner            string          `bson:"miner"`
	Difficulty       uint64          `bson:"difficulty"`
	TotalDifficulty  uint64          `bson:"totalDifficulty"`
	Size             float64         `bson:"size"`
	ExtraData        string          `bson:"extraData"`
	GasLimit         uint64          `bson:"gasLimit"`
	GasUsed          uint64          `bson:"gasUsed"`
	Timestamp        uint64          `bson:"timestamp"`
	Uncles           []*types.Header `bson:"uncles"`
	Txns             int             `bson:"txns"`
	AvgGasprice      string          `bson:"avgGasprice"`
	Reward           string          `bson:"reward"`

	ReceiptHash string `bson:"receiptHash"`
	BlockTime   uint64 `bson:"blockTime"`
}

type BlockInfo struct {
	Number          uint64 `bson:"number"`
	TotalDifficulty uint64 `bson:"totalDifficulty"`
	Timestamp       uint64 `bson:"timestamp"`
}

type mgoTransaction struct {
	Key              string   `bson:"_id"`
	Hash             string   `bson:"hash"`
	HashLen          int      `bson:"hashLen"`
	Nonce            uint64   `bson:"nonce"`
	BlockHash        string   `bson:"blockHash"`
	BlockNumber      uint64   `bson:"blockNumber"`
	TransactionIndex uint64   `bson:"transactionIndex"`
	From             string   `bson:"from"`
	To               string   `bson:"to"`
	Value            float64  `bson:"value"`
	GasLimit         uint64   `bson:"gasLimit"`
	GasPrice         string   `bson:"gasPrice"`
	GasUsed          uint64   `bson:"gasUsed"`
	Timestamp        uint64   `bson:"timestamp"`
	Input            string   `bson:"input"`
	//Status		int `bson:"status"`
	IsERC20       int     `bson:"isERC20"`
	TxType        int     `bson:"type"`
	CoinType      string  `bson:"coinType"`
	ContractFrom  string  `bson:"contractFrom"`
	ContractTo    string  `bson:"contractTo"`
	ContractValue float64 `bson:"contractValue"`
}

type mgoAccount struct {
	Key             string  `bson:"_id"`
	Address         string  `bson:"address"`
	Timestamp       uint64  `bson:"timestamp"`
	UpdateTimestamp uint64  `bson:"updateTimestamp"`
	IsConfirm       int     `bson:"isConfirm"`
	Balance         float64 `bson:"balance"`
	BalanceString   string  `bson:"balanceString"`
	PublicKey       string  `bson:"publicKey"`
}

type mgoDcrmAccount struct {
	Key           string  `bson:"_id"`
	Address       string  `bson:"address"`
	DcrmAddress   string  `bson:"dcrmAddress"`
	CoinType      string  `bson:"coinType"`
	Balance       float64 `bson:"balance"`
	BalanceString string  `bson:"balanceString"`
	IsERC20       int     `bson:"isERC20"`
	IsLockin      int     `bson:"isLockin"`
	IsLockout     int     `bson:"isLockout"`
	SortId        int     `bson:"sortId"`
	Timestamp     uint64  `bson:"timestamp"`
}

type mgoHistory struct {
	Key           string  `bson:"key"`
	Hash          string  `bson:"hash"`
	Nonce         uint64  `bson:"nonce"`
	From          string  `bson:"from"`
	To            string  `bson:"to"`
	Value         float64 `bson:"value"`
	GasLimit      uint64  `bson:"gasLimit"`
	GasPrice      string  `bson:"gasPrice"`
	Timestamp     uint64  `bson:"timestamp"`
	Input         string  `bson:"input"`
	Status        int     `bson:"status"`
	OutHash       string  `bson:"outHash"`
	CoinType      string  `bson:"coinType"`
	ContractTo    string  `bson:"contractTo"`
	ContractValue float64 `bson:"contractValue"`
}

type mgoDexBlock struct {
	Hash      string  `bson:"_id"`
	Number    uint64  `bson:"number"`
	Squence   uint64  `bson:"squence"`
	Orders    int     `bson:"orders"`
	Price     float64 `bson:"price"`
	Volumes   float64 `bson:"volumes"`
	Trade     string  `bson:"trade"`
	Timestamp uint64  `bson:"timestamp"`
}

type mgoDexTx struct {
	Key           string  `bson:"_id"`
	Hash          string  `bson:"hash"`
	ParentHash    string  `bson:"parentHash"`
	Height        uint64  `bson:"height"`
	Number        uint64  `bson:"number"`
	From          string  `bson:"from"`
	OrderType     string  `bson:"orderType"`
	Price         float64 `bson:"price"`
	MatchPrice    float64 `bson:"matchPrice"`
	Quantity      float64 `bson:"quantity"`
	TotalQuantity float64 `bson:"total"`
	Completed     float64 `bson:"completed"`
	Rule          string  `bson:"rule"`
	Side          string  `bson:"side"`
	Trade         string  `bson:"trade"`
	Timestamp     uint64  `bson:"timestamp"`
}

type mgoOrderCache struct {
	Key    string  `bson:"_id"`
	Hash   string  `bson:"hash"`
	Price  float64 `bson:"price"`
	Volume float64 `bson:"volume"`
	Total  float64 `bson:"total"`
	Trade  string  `bson:"trade"`
	Side   string  `bson:"side"`
}

//xprotocol
type pv struct {
	Price  string
	Volume string
}

type sbs struct {
	Sells []pv `json:"Sells, omitempty"`
	Buys  []pv
}

type OrderTrade struct {
	TRADE  string
	STATUS string
}

type OrderStatus struct {
	ODS []OrderTrade
}

type mgoOrderInfo struct {
	Hash      string  `bson:"hash"`
}

type mgoOrder struct {
	Key       string  `bson:"_id"`
	Hash      string  `bson:"hash"`
	Value     float64 `bson:"value"`
	Pair      string  `bson:"pair"`
	From      string  `bson:"from"`
	To        string  `bson:"to"`
	Timestamp uint64  `bson:"timestamp"`
	Nonce     uint64  `bson:"nonce"`
	GasPrice  string  `bson:"gasPrice"`
	GasLimit  uint64  `bson:"gasLimit"`
	Price     float64 `bson:"price"`
	Quantity  float64 `bson:"quantity"`
	Completed float64 `bson:"completed"`
	Side      string  `bson:"side"`
	Data      string  `bson:"data"`
	Status    int     `bson:"status"`
}
