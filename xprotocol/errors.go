package xprotocol 

// OrderBook erros
var (
	ErrInvalidQuantity      = `{Code:77,Error:"orderbook: invalid order quantity."}`
	ErrInvalidPrice         = `{Code:78,Error:"orderbook: invalid order price."}`
	ErrOrderExists          = `{Code:79,Error:"orderbook: order already exists."}`
	ErrOrderNotExists       = `{Code:80,Error:"orderbook: order does not exist."}`
	ErrInsufficientQuantity = `{Code:81,Error:"orderbook: insufficient quantity to calculate price."}`
	ErrOrderObjectNil = `{Code:82,Error:"orderbook: order object is nil."}`
	ErrOrderQueueObjectNil = `{Code:83,Error:"orderbook: order queue object is nil."}`
	ErrCompressDataNil = `{Code:84,Error:"orderbook: compress data is nil."}`
	ErrCompressNewWriterFail = `{Code:85,Error:"orderbook: get compress writer fail."}`
	ErrUnCompressDataNil = `{Code:86,Error:"orderbook: uncompress data is nil."}`
	ErrUnCompressNewReaderFail = `{Code:87,Error:"orderbook: get uncompress reader fail."}`
	ErrMatchOrderBookFail = `{Code:88,Error:"orderbook: match fail."}`
	ErrOrderBookObjectNil = `{Code:89,Error:"orderbook: order book object is nil."}`
	ErrOrderInsufficient = `{Code:90,Error:"Insufficient Order Balance."}`
	ErrGetODBError = `{Code:91,Error:"get orderbook error."}`
	ErrMatchResNil = `{Code:92,Error:"match result is nil."}`
	ErrNotODBTxData = `{Code:93,Error:"not ODB tx data."}`
	ErrAlreadyUpdateMatchRes = `{Code:94,Error:"match result has already been updated."}`
	ErrBlockNumMiss = `{Code:95,Error:"block number miss."}`
	ErrNotInGroup = `{Code:96,Error:"node not in group."}`
	ErrGetMatchResFail = `{Code:97,Error:"get match result fail."}`
	ErrEncodeMatchResFail = `{Code:98,Error:"encode match result fail."}`
	ErrCompressMatchResFail = `{Code:99,Error:"compress mach result fail."}`
	ErrNewMatchResTxFail = `{Code:100,Error:"new match result tx fail."}`
)

