package mongodb

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fusion/go-fusion/common"
	"github.com/fusion/go-fusion/consensus/clique"
	"github.com/fusion/go-fusion/core/state"
	"github.com/fusion/go-fusion/core/types"
	"github.com/fusion/go-fusion/crypto"
	"github.com/fusion/go-fusion/crypto/dcrm/cryptocoins"
	"github.com/fusion/go-fusion/log"
	"github.com/fusion/go-fusion/p2p/layer2"
	//"github.com/fusion/go-fusion/xprotocol/orderbook"
)

func init() {
	for i, ct := range cryptocoins.Cointypes {
		coinmap[ct] = i
	}
	layer2.RegisterUpdateOrderCacheCallback(UpdateOrderCacheFromShare)
	blockInfo = new(BlockInfo)
	//getAddressByTxHash("0xe34d08e9c6c929e8642a30a9404fea9035f78e1cc574cb216ad1674e1e4fbfc8", "ERC20BNB")
}

func RevSync(block *types.Block, receipts types.Receipts) {
	logPrintAll("mongodb ==== call RevSync() ====", "block", block.NumberU64(), "blockHash", block.Hash().String())
	if !Mongo || MongoSlave {
		return
	}
	go revSync(block, receipts)
}

func revSync(block *types.Block, receipts types.Receipts) {
	logPrintAll("ready ==== revSync() ====", "block", block.NumberU64(), "blockHash", block.Hash().String())
	BlockLock.Lock()
	defer BlockLock.Unlock()
	bsi, _ := blockSyncInfo.Load(block.Hash().String())
	if bsi != EXIST {
		logPrintAll("==== revSync() ====", "not exist block", block.NumberU64(), "blockHash", block.Hash().String())
		return
	}
	blockSyncInfo.Delete(block.Hash().String())
	log.Info("mongodb ==== RevSync() ====", "block", block.NumberU64(), "blockHash", block.Hash().String())
	//RevSync block
	mb := new(mgoBlock)
	mb.Number = block.NumberU64()
	mgoInsertBlock(mb, true)
	//RevSync transaction
	mgoParseTransaction(block, receipts, true, nil)
	//RevSync account
	mgoParseAccount(block, receipts, true, nil)
}

func LightSync(block *types.Block, txs []*types.Transaction) {
	log.Info("mongodb ==== call Sync() ====", "block", block.NumberU64(), "blockHash", block.Hash().String())
	if !Mongo || MongoSlave {
		return
	}
	if database == nil {
		mongoInit()
	}
	go lightSyncBlock(block, txs)
}

func Sync(block *types.Block, receipts types.Receipts, state *state.StateDB) {
	log.Info("mongodb ==== call Sync() ====", "block", block.NumberU64(), "blockHash", block.Hash().String())
	if !Mongo || MongoSlave {
		return
	}
	if database == nil {
		mongoInit()
	}
	go syncBlock(block, receipts, state)
}

func lightSyncBlock(block *types.Block, txs []*types.Transaction) {
	log.Info("mongodb ready ==== syncBlock() ====", "block", block.NumberU64(), "blockHash", block.Hash().String())
	BlockLock.Lock()
	defer BlockLock.Unlock()
	mgoParseBlock(block, false)
	simpleMgoParseTransaction(block, txs, false)
}

func syncBlock(block *types.Block, receipts types.Receipts, state *state.StateDB) {
	log.Info("mongodb ready ==== syncBlock() ====", "block", block.NumberU64(), "blockHash", block.Hash().String())
	BlockLock.Lock()
	defer BlockLock.Unlock()
	bsi, _ := blockSyncInfo.Load(block.Hash().String())
	if bsi == EXIST {
		logPrintAll("==== syncBlock() ====", "exist block", block.NumberU64(), "blockHash", block.Hash().String())
		return
	}
	blockSyncInfo.Store(block.Hash().String(), EXIST)
	log.Info("mongodb ==== syncBlock() ====", "block", block.NumberU64(), "blockHash", block.Hash().String())
	mgoParseBlock(block, false)
	//sync transaction
	mgoParseTransaction(block, receipts, false, state)
	//sync account
	mgoParseAccount(block, receipts, false, state)
}

func SyncGenesisBlock(block *types.Block) {
	if !Mongo {
		return
	}
	if database == nil {
		mongoInit()
	}
	if FindBlock(tbBlocks, 0) != nil {
		return
	}
	mgoParseBlock(block, false)
}

func SyncGenesisAccount(address string, time uint64, balance string) {
	if !Mongo {
		return
	}
	if FindAccount(tbAccounts, address) != nil {
		return
	}
	ma := new(mgoAccount)
	ma.Address = address
	ma.Key = ma.Address
	ma.Timestamp = time
	valuef, _ := strconv.ParseFloat(balance, 64)
	ma.Balance = valuef / getCointypeDiv("FSN")
	logPrintAll("==== SyncGenesisAccount() ====", "address", address, "balance", balance, "ma.Balance", ma.Balance)
	mgoInsertAccount(ma)
}

func mgoParseAccount(block *types.Block, receipts types.Receipts, reverse bool, state *state.StateDB) {
	logPrintAll("==== mgoParseAccount() ====", "blockNumber", block.NumberU64())
	if block.Time().Uint64() == 0 {
		return
	}
	ma := new(mgoAccount)
	ma.Address = block.Coinbase().String()
	UpdateBalance4State(ma, state, block.Time().Uint64())
}

func recoverERC20Type(CoinType string, ERC20 int) string {
	cointype := CoinType
	if ERC20 == 1 {
		if !strings.HasPrefix(cointype, "ERC20") {
			cointype1 := append([]byte("ERC20"), CoinType...)
			cointype = string(cointype1)
		}
	}
	return cointype
}

func getFeeCointype(CoinType string, ERC20 int) (string, error) {
	cointype := recoverERC20Type(CoinType, ERC20)

	if cointype == "ERC20BNB" ||
		cointype == "ERC20MKR" ||
		cointype == "ERC20GUSD" ||
		cointype == "ERC20HT" ||
		cointype == "ERC20RMBT" ||
		cointype == "ERC20BNT" {
		return "ETH", nil
	}

	if cointype == "USDT" {
		return "BTC", nil
	}
	if cointype == "EVT1001" {
		return "EVT1", nil
	}

	if cointype == "FSN" ||
		cointype == "ETH" ||
		cointype == "BTC" ||
		cointype == "EOS" ||
		cointype == "TRX" ||
		cointype == "XRP" ||
		cointype == "EVT1" {
		return cointype, nil
	}

	return "", errors.New("cointype is not support")
}

func UpdateBalance4State(ma *mgoAccount, state *state.StateDB, time uint64) {
	logPrintAll("==== UpdateBalance4State() ====", "ma", ma)
	go func() {
		//checkAccount(ma, time)
		if state == nil {
			return
		}
		cointype := "FSN"
		ret := state.GetBalance(common.HexToAddress(ma.Address), strings.ToUpper(cointype))

		logPrintAll("==== UpdateBalance4State() ====", "address", ma.Address, "balance", ret, "cointype", cointype)
		ma.BalanceString = fmt.Sprintf("%v", ret)
		ma.Balance = ConvertStringToFloat64(ma.BalanceString, cointype)
		UpdateBalance(tbAccounts, ma.Address, ma.BalanceString, ma.Balance)
	}()
}

func UpdateDcrmBalance4State(mda *mgoDcrmAccount, state *state.StateDB) {
	logPrintAll("==== UpdateDcrmBalance4State() ====", "mda", mda)
	go func() {
		//checkDcrmAccount(mda)
		if state == nil {
			return
		}
		cointype := recoverERC20Type(mda.CoinType, mda.IsERC20)

		ret := state.GetBalance(common.HexToAddress(mda.Address), strings.ToUpper(cointype))
		logPrintAll("UpdateDcrmBalance4State", "address", mda.Address, "balance", ret, "cointype", mda.CoinType)
		mda.BalanceString = fmt.Sprintf("%v", ret)
		mda.Balance = ConvertStringToFloat64(mda.BalanceString, mda.CoinType)
		UpdateDcrmBalance(tbDcrmAccounts, mda.Address, mda.CoinType, mda.BalanceString, mda.Balance)
	}()
}

func checkDcrmAccount(mda *mgoDcrmAccount) {
	account := FindDcrmAccount(tbDcrmAccounts, mda.Address, mda.CoinType)
	if account == nil {
		mda.BalanceString = "0"
		mda.Balance = ConvertStringToFloat64(mda.BalanceString, mda.CoinType)
		mgoInsertDcrmAccount(mda)
	}
}

func mogUpdateDcrmBalance(address string, value string, time uint64, add bool, fee string, cointype string) {
	mda := new(mgoDcrmAccount)
	mda.Address = address
	_, mda.CoinType = getERC20Info(cointype)
	mda.Key = (crypto.Keccak256Hash([]byte(mda.Address), []byte(cointype))).String()
	UpdateDcrmAccount(mda, value, time, add, fee, cointype)
}

func UpdateDcrmAccount(mda *mgoDcrmAccount, value string, time uint64, add bool, fee string, cointype string) {
	logPrintAll("==== UpdateDcrmAccount() ====", "add", add, "mda.Address", mda.Address, "value", value, "cointype", cointype, "fee", fee)
	if value == "" {
		value = "0"
	}
	if fee == "" {
		fee = "0"
	}
	account := FindDcrmAccount(tbDcrmAccounts, mda.Address, cointype)
	if account == nil {
		if add {
			mda.BalanceString = value
			mda.Balance = ConvertStringToFloat64(value, cointype)
		} else {
			//mda.Balance = account[0].Balance - balance
			return
		}
		mda.Timestamp = time
		mgoInsertDcrmAccount(mda)
	} else {
		balanceBigInt, _ := new(big.Int).SetString(account[0].BalanceString, 0)
		if len(account[0].BalanceString) == 0 {
			balanceBigInt = big.NewInt(0)
		}
		valueBigInt, _ := new(big.Int).SetString(value, 0)
		if len(value) == 0 {
			valueBigInt = big.NewInt(0)
		}
		logPrintAll("==== UpdateDcrmAccount() ====", "balanceBigInt", balanceBigInt, "valueBigInt", valueBigInt)
		if add {
			feeBigInt, _ := new(big.Int).SetString(fee, 0)
			balanceB := new(big.Int).Add(balanceBigInt, valueBigInt)
			mda.BalanceString = fmt.Sprintf("%v", new(big.Int).Add(balanceB, feeBigInt))
			mda.Balance = ConvertStringToFloat64(mda.BalanceString, cointype)
		} else {
			feeBigInt, _ := new(big.Int).SetString(fee, 0)
			balanceB := new(big.Int).Sub(balanceBigInt, valueBigInt)
			mda.BalanceString = fmt.Sprintf("%v", new(big.Int).Sub(balanceB, feeBigInt))
			mda.Balance = ConvertStringToFloat64(mda.BalanceString, cointype)
			if mda.Balance < 0 {
				fmt.Errorf("UpdateDcrmAccount, insufficient balance, address: %+v\n", mda.Address)
				return
			}
		}
		UpdateDcrmBalance(tbDcrmAccounts, mda.Address, cointype, mda.BalanceString, mda.Balance)
	}
}

func ConvertStringToFloat64(value string, cointype string) float64 {
	valuef, _ := strconv.ParseFloat(value, 64)
	valuedf := valuef / getCointypeDiv(cointype)
	return valuedf
}

func checkAccount(ma *mgoAccount, time uint64) {
	account := FindAccount(tbAccounts, ma.Address)
	if account == nil {
		ma.BalanceString = "0"
		ma.Balance = ConvertStringToFloat64(ma.BalanceString, "FSN")
		ma.Timestamp = time
		mgoInsertAccount(ma)
	}
}
func UpdateAccount(ma *mgoAccount, value string, time uint64, add bool, fee string) {
	logPrintAll("==== UpdateAccount() ====", "add", add, "ma.Address", ma.Address, "value", value, "fee", fee)
	if value == "" {
		value = "0"
	}
	if fee == "" {
		fee = "0"
	}
	cointype := "FSN"
	account := FindAccount(tbAccounts, ma.Address)
	if account == nil {
		if add {
			ma.BalanceString = value
			ma.Balance = ConvertStringToFloat64(value, cointype)
		} else {
			//ma.Balance = account[0].Balance - balance
			return
		}
		ma.Key = ma.Address
		ma.Timestamp = time
		mgoInsertAccount(ma)
	} else {
		balanceBigInt, _ := new(big.Int).SetString(account[0].BalanceString, 0)
		if balanceBigInt == nil {
			balanceBigInt = big.NewInt(0)
		}
		valueBigInt, _ := new(big.Int).SetString(value, 0)
		logPrintAll("==== UpdateAccount() ====", "balanceBigInt", balanceBigInt, "valueBigInt", valueBigInt)
		if add {
			ma.BalanceString = fmt.Sprintf("%v", new(big.Int).Add(balanceBigInt, valueBigInt))
			ma.Balance = ConvertStringToFloat64(ma.BalanceString, cointype)
		} else {
			feeBigInt, _ := new(big.Int).SetString(fee, 0)
			balanceB := new(big.Int).Sub(balanceBigInt, valueBigInt)
			ma.BalanceString = fmt.Sprintf("%v", new(big.Int).Sub(balanceB, feeBigInt))
			ma.Balance = ConvertStringToFloat64(ma.BalanceString, cointype)
			if ma.Balance < 0 {
				fmt.Errorf("UpdateAccount, insufficient balance, address: %+v\n", ma.Address)
				return
			}
		}
		UpdateBalance(tbAccounts, ma.Address, ma.BalanceString, ma.Balance)
	}
}

func simpleMgoParseTransaction(block *types.Block, txs []*types.Transaction, reverse bool) {
	logPrintAll("==== simpleMgoParseTransaction() ====", "block", block.NumberU64())
	var wg sync.WaitGroup
	for i, tx := range txs {
		wg.Add(1)
		go simpleProcessTx(i, tx, block, reverse, &wg)
	}
	wg.Wait()
}

func mgoParseTransaction(block *types.Block, receipts types.Receipts, reverse bool, state *state.StateDB) {
	logPrintAll("==== mgoParseTransaction() ====", "block", block.NumberU64())
	txs := block.Transactions()
	var wg sync.WaitGroup
	for i, tx := range txs {
		wg.Add(1)
		go processTx(i, tx, block, receipts, reverse, state, &wg)
	}
	wg.Wait()
}

func simpleProcessTx(i int, tx *types.Transaction, block *types.Block, reverse bool, wg *sync.WaitGroup) {
	// 不要 receipts state
	defer wg.Done()
	logPrintAll("==== simpleProcessTx() ====", "index", i, "txHash", tx.Hash().String())
	mt := new(mgoTransaction)

	mt.Hash = tx.Hash().String()
	mt.Key = mt.Hash
	mt.Nonce = tx.Nonce()
	mt.BlockHash = block.Hash().String()
	mt.BlockNumber = block.NumberU64()
	mt.TransactionIndex = uint64(i)
	mt.From = getTxFrom(tx).String()
	mt.To = tx.To().String()
	value := tx.Value().String()
	valuef, _ := strconv.ParseFloat(value, 64)
	mt.Value = valuef / getCointypeDiv("FSN")
	mt.GasPrice = tx.GasPrice().String()
	mt.GasLimit = tx.Gas()
	mt.Timestamp = block.Time().Uint64()
	mt.CoinType = "FSN"
	mt.TxType = getTxType(tx)
	txData := string(tx.Data())
	logPrintAll("==== mgoParseTransaction() ====", "mt", mt)
	if len(txData) > 0 {
		logPrintAll("==== mgoParseTransaction() ====", "mt.Hash", mt.Hash, "txData", txData)
		//mt.Input = fmt.Sprintf("%x", txData)
		if strings.HasPrefix(txData, "ODB") { // dex
			txDataSlice := strings.Split(txData, dexDataSplit)
			if txDataSlice[0] == "ODB" {
				logPrintAll("mongodb ==== mgoParseTransaction() ====", "mt.Hash", mt.Hash, "txType", "ODB")
				mr := GetMrFromMatch(txDataSlice[2])
				if mr != nil {
					logPrintAll("==== mgoParseTransaction() ====", "xprotocol.UnCompress, mr", mr)
					trade := txDataSlice[3]
					dtost := mr.Price
					mrPrice, _ := strconv.ParseFloat(dtost, 64)
					vol := mr.CurVolume

					oi := new(ODBInfo)
					oi.Trade = trade
					oi.Price = mr.Price
					oi.Volumes = mr.Volumes
					oi.Orders = mr.Orders
					mt.Input = fmt.Sprintf("%x", oi)

					for i, done := range mr.Done {
						logPrintAll("==== mgoParseTransaction() ====", "done", done)
						dtos := done.Quantity
						dtos = customerString(dtos)
						quantity, _ := strconv.ParseFloat(dtos, 64)
						//TODO have done from xprotocol/exchange.go
						//mgoUpdateOrder(done.Id, quantity)
						//mgoUpdateOrderCaches(done.Id, quantity)
						//TODO
						mdt := new(mgoDexTx)
						mdt.Hash = done.Id
						//errc := CheckOrderCacheTxForODB(tbOrders, done.Id)
						//if errc == false {
						//	log.Warn("==== mgoParseTransaction() ====", "CheckOrderTxForODB not exist hash", tx.Hash().String())
						//}
						mdt.ParentHash = tx.Hash().String()
						mdt.Height, _ = strconv.ParseUint(txDataSlice[1], 10, 64)
						mdt.Number = block.NumberU64()
						height := make([]byte, 20)
						binary.BigEndian.PutUint64(height, mdt.Height)
						mdt.Key = (crypto.Keccak256Hash([]byte(mdt.Hash), []byte(height))).String()
						mdt.From = done.From.String()
						mdt.OrderType = done.Ordertype
						dtos = done.Price
						mdt.Price, _ = strconv.ParseFloat(dtos, 64)
						mdt.MatchPrice = mrPrice
						mdt.Quantity = quantity
						//mdt.TotalQuantity = getTotalQuantity(mdt.Hash)
						//mdt.Completed = getCompleted(mdt.Hash)
						mdt.TotalQuantity, _ = strconv.ParseFloat(vol[i].Vol, 64)
						mdt.Completed = quantity
						mdt.Rule = done.Rule
						mdt.Side = strings.ToLower(done.Side)
						mdt.Trade = done.Trade
						mdt.Timestamp = uint64(done.Timestamp)
						logPrintAll("==== mgoParseTransaction() ====", "mdt", mdt)
						mgoInsertDexTx(mdt, reverse)

						//update Balance
						tradeSlice := strings.Split(done.Trade, "/")
						Trade0 := tradeSlice[0]
						Trade1 := tradeSlice[1]
						From := done.From.String()
						Timestamp := uint64(done.Timestamp)
						if strings.ToLower(mdt.Side) == "buy" {
							// "A/B"
							// A: quan*UA - quan*UA*3/1000
							// B: -price*quan*UB
							if Trade0 == "FSN" {
								ma := new(mgoAccount)
								ma.Address = From
								//UpdateBalance4State(ma, state, block.Time().Uint64())
							} else {
								mda := new(mgoDcrmAccount)
								mda.Address = From
								mda.CoinType = Trade0
								mda.Key = (crypto.Keccak256Hash([]byte(mda.Address), []byte(mda.CoinType))).String()
								mda.Timestamp = Timestamp
								//UpdateDcrmBalance4State(mda, state)
							}
							if Trade1 == "FSN" {
								ma := new(mgoAccount)
								//to
								ma.Address = From
								//UpdateBalance4State(ma, state, block.Time().Uint64())
							} else {
								mda := new(mgoDcrmAccount)
								mda.Address = From
								mda.CoinType = Trade1
								mda.Key = (crypto.Keccak256Hash([]byte(mda.Address), []byte(mda.CoinType))).String()
								//UpdateDcrmBalance4State(mda, state)
							}
						} else if strings.ToLower(mdt.Side) == "sell" {
							if Trade0 == "FSN" {
								ma := new(mgoAccount)
								ma.Address = From
								//UpdateBalance4State(ma, state, block.Time().Uint64())
							} else {
								mda := new(mgoDcrmAccount)
								mda.Address = From
								mda.CoinType = Trade0
								mda.Key = (crypto.Keccak256Hash([]byte(mda.Address), []byte(mda.CoinType))).String()
								mda.Timestamp = Timestamp
								//UpdateDcrmBalance4State(mda, state)
							}
							if Trade1 == "FSN" {
								ma := new(mgoAccount)
								ma.Address = From
								//UpdateBalance4State(ma, state, block.Time().Uint64())
							} else {
								mda := new(mgoDcrmAccount)
								mda.Address = From
								mda.CoinType = Trade1
								mda.Key = (crypto.Keccak256Hash([]byte(mda.Address), []byte(mda.CoinType))).String()
								//UpdateDcrmBalance4State(mda, state)
							}
						}
					}
					mt.HashLen = len(mr.Done)
					mt.CoinType = trade
					mgoUpdateTransaction(mt, reverse)

					mdb := new(mgoDexBlock)
					mdb.Hash = tx.Hash().String()
					Squence, _ := strconv.ParseInt(txDataSlice[1], 10, 64)
					mdb.Squence = uint64(Squence)
					mdb.Number = block.NumberU64()
					mdb.Orders = mr.Orders
					dtos := mr.Price
					mdb.Price, _ = strconv.ParseFloat(dtos, 64)
					mdb.Trade = trade
					mdb.Timestamp = block.Time().Uint64()
					dtos = mr.Volumes
					mdb.Volumes, _ = strconv.ParseFloat(dtos, 64)
					mgoInsertDexBlock(mdb, reverse)
					return //do not do mgoUpdateTransaction again
				}
			}
		} else {
			mt.Input = hex.EncodeToString(tx.Data())
			UpdataTransferHistoryStatus(tx, reverse)
			txDataSlice := strings.Split(txData, ":")
			mda := new(mgoDcrmAccount)
			mda.Address = getTxFrom(tx).String()
			if mt.TxType == txType_NEWTRADE {
				//DO NOTHING
			} else if mt.TxType == txType_CONFIRMADDRESS {
				logPrintAll(" ==== mgoParseTransaction() ====", "mt.Hash", mt.Hash, "txType", "CONFIRM")
				//account := FindAccount(tbAccounts, mda.Address)
				//if account != nil {
				//	if account[0].IsConfirm == 0 {
				if reverse == false {
					UpdateComfirm(tbAccounts, mda.Address, 1)
				} else {
					UpdateComfirm(tbAccounts, mda.Address, 0)
				}
				//	}
				//}
				mt.CoinType = txDataSlice[2]
			} else {
				mt.CoinType = txDataSlice[3]
				mda.CoinType = txDataSlice[3]
				//fee
				mda.SortId = coinmap[txDataSlice[3]]
				mda.IsERC20, mda.CoinType = getERC20Info(txDataSlice[3])

				//from
				mda.Key = (crypto.Keccak256Hash([]byte(mda.Address), []byte(txDataSlice[3]))).String()
				mda.Timestamp = block.Time().Uint64()
				mt.IsERC20, mt.CoinType = getERC20Info(txDataSlice[3])
				if mt.TxType == txType_LOCKIN {
					logPrintAll(" ==== mgoParseTransaction() ====", "mt.Hash", mt.Hash, "txType", "LOCKIN")
					mt.ContractFrom, mt.ContractTo = getAddressByTxHash(txDataSlice[1], txDataSlice[3])
					//mda.Key = (crypto.Keccak256Hash([]byte(mda.Address), []byte(mda.CoinType))).String()
					logPrintAll("LOCKIN", "mda", mda)
					//UpdateDcrmBalance4State(mda, state)
				} else if mt.TxType == txType_LOCKOUT {
					logPrintAll(" ==== mgoParseTransaction() ====", "mt.Hash", mt.Hash, "txType", "LOCKOUT")
					//mda.Address = getTxFrom(tx).String()
					//mda.Key = (crypto.Keccak256Hash([]byte(mda.Address), []byte(mda.CoinType))).String()
					logPrintAll("LOCKOUT", "mda", mda)
					//UpdateDcrmBalance4State(mda, state)
					CoinType, err := getFeeCointype(mda.CoinType, mda.IsERC20)
					if err == nil {
						mdaFee := new(mgoDcrmAccount)
						mdaFee.Address = getTxFrom(tx).String()
						mdaFee.CoinType = CoinType
						mdaFee.IsERC20 = 0
						mdaFee.Key = (crypto.Keccak256Hash([]byte(mdaFee.Address), []byte(mdaFee.CoinType))).String()
						logPrintAll("LOCKOUT", "mda(fee)", mdaFee)
						//UpdateDcrmBalance4State(mdaFee, state)
					}
					mt.ContractFrom = mt.From
					mt.ContractTo = txDataSlice[1]
				} else if mt.TxType == txType_DCRMTX {
					logPrintAll(" ==== mgoParseTransaction() ====", "mt.Hash", mt.Hash, "txType", "DCRMTX")
					//mda.Address = getTxFrom(tx).String()
					//mda.Key = (crypto.Keccak256Hash([]byte(mda.Address), []byte(mda.CoinType))).String()
					logPrintAll("DCRMTX", "mda1", mda)
					//UpdateDcrmBalance4State(mda, state)
					mda2 := new(mgoDcrmAccount)
					mda2.Address = txDataSlice[1]
					mda2.IsERC20, mda2.CoinType = getERC20Info(txDataSlice[3])
					mda2.Key = (crypto.Keccak256Hash([]byte(mda2.Address), []byte(mda2.CoinType))).String()
					logPrintAll("DCRMTX", "mda2", mda2)
					//UpdateDcrmBalance4State(mda2, state)
					mt.ContractFrom = mt.From
					mt.ContractTo = txDataSlice[1]
				}
				valueTxf, _ := strconv.ParseFloat(txDataSlice[2], 64)
				mt.ContractValue = valueTxf / getCointypeDiv(mt.CoinType)
			}
		}
	} else {
		UpdataTransferHistoryStatus(tx, reverse)
	}
	ma := new(mgoAccount)
	//from
	ma.Address = getTxFrom(tx).String()
	logPrintAll("==== mgoParseTransaction() ====", "from ma.Address", ma.Address)
	//UpdateAccount(ma, tx.Value().String(), block.Time().Uint64(), reverse, feeFsn)
	//UpdateBalance4State(ma, state, block.Time().Uint64())
	//to
	maTo := new(mgoAccount)
	maTo.Address = tx.To().String()
	logPrintAll("==== mgoParseTransaction() ====", "to ma.Address", maTo.Address)
	//UpdateBalance4State(maTo, state, block.Time().Uint64())
	mgoUpdateTransaction(mt, reverse)
}

func processTx(i int, tx *types.Transaction, block *types.Block, receipts types.Receipts, reverse bool, state *state.StateDB, wg *sync.WaitGroup) {
	defer wg.Done()
	logPrintAll("==== processTx() ====", "index", i, "txHash", tx.Hash().String())
	mt := new(mgoTransaction)

	mt.Hash = tx.Hash().String()
	mt.Key = mt.Hash
	mt.Nonce = tx.Nonce()
	mt.BlockHash = block.Hash().String()
	mt.BlockNumber = block.NumberU64()
	mt.TransactionIndex = uint64(i)
	mt.From = getTxFrom(tx).String()
	mt.To = tx.To().String()
	value := tx.Value().String()
	valuef, _ := strconv.ParseFloat(value, 64)
	mt.Value = valuef / getCointypeDiv("FSN")
	mt.GasPrice = tx.GasPrice().String()
	mt.GasLimit = tx.Gas()
	for j, receipt := range receipts {
		if tx.Hash() == receipt.TxHash {
			logPrintAll("==== mgoParseTransaction() ====", "j", j, "GasUsed", receipt.GasUsed)
			mt.GasUsed = receipt.GasUsed
			break
		}
	}
	mt.Timestamp = block.Time().Uint64()
	mt.CoinType = "FSN"
	mt.TxType = getTxType(tx)
	txData := string(tx.Data())
	logPrintAll("==== mgoParseTransaction() ====", "mt", mt)
	if len(txData) > 0 {
		logPrintAll("==== mgoParseTransaction() ====", "mt.Hash", mt.Hash, "txData", txData)
		//mt.Input = fmt.Sprintf("%x", txData)
		if strings.HasPrefix(txData, "ODB") { // dex
			txDataSlice := strings.Split(txData, dexDataSplit)
			if txDataSlice[0] == "ODB" {
				logPrintAll("mongodb ==== mgoParseTransaction() ====", "mt.Hash", mt.Hash, "txType", "ODB")
				mr := GetMrFromMatch(txDataSlice[2])
				if mr != nil {
					logPrintAll("==== mgoParseTransaction() ====", "xprotocol.UnCompress, mr", mr)
					trade := txDataSlice[3]
					dtost := mr.Price
					mrPrice, _ := strconv.ParseFloat(dtost, 64)
					vol := mr.CurVolume

					oi := new(ODBInfo)
					oi.Trade = trade
					oi.Price = mr.Price
					oi.Volumes = mr.Volumes
					oi.Orders = mr.Orders
					mt.Input = fmt.Sprintf("%x", oi)

					for i, done := range mr.Done {
						logPrintAll("==== mgoParseTransaction() ====", "done", done)
						dtos := done.Quantity
						dtos = customerString(dtos)
						quantity, _ := strconv.ParseFloat(dtos, 64)
						//TODO have done from xprotocol/exchange.go
						//mgoUpdateOrder(done.Id, quantity)
						//mgoUpdateOrderCaches(done.Id, quantity)
						//TODO
						mdt := new(mgoDexTx)
						mdt.Hash = done.Id
						//errc := CheckOrderCacheTxForODB(tbOrders, done.Id)
						//if errc == false {
						//	log.Warn("==== mgoParseTransaction() ====", "CheckOrderTxForODB not exist hash", tx.Hash().String())
						//}
						mdt.ParentHash = tx.Hash().String()
						mdt.Height, _ = strconv.ParseUint(txDataSlice[1], 10, 64)
						mdt.Number = block.NumberU64()
						height := make([]byte, 20)
						binary.BigEndian.PutUint64(height, mdt.Height)
						mdt.Key = (crypto.Keccak256Hash([]byte(mdt.Hash), []byte(height))).String()
						mdt.From = done.From.String()
						mdt.OrderType = done.Ordertype
						dtos = done.Price
						mdt.Price, _ = strconv.ParseFloat(dtos, 64)
						mdt.MatchPrice = mrPrice
						mdt.Quantity = quantity
						//mdt.TotalQuantity = getTotalQuantity(mdt.Hash)
						//mdt.Completed = getCompleted(mdt.Hash)
						mdt.TotalQuantity, _ = strconv.ParseFloat(vol[i].Vol, 64)
						mdt.Completed = quantity
						mdt.Rule = done.Rule
						mdt.Side = strings.ToLower(done.Side)
						mdt.Trade = done.Trade
						mdt.Timestamp = uint64(done.Timestamp)
						logPrintAll("==== mgoParseTransaction() ====", "mdt", mdt)
						mgoInsertDexTx(mdt, reverse)

						//update Balance
						tradeSlice := strings.Split(done.Trade, "/")
						Trade0 := tradeSlice[0]
						Trade1 := tradeSlice[1]
						From := done.From.String()
						Timestamp := uint64(done.Timestamp)
						if strings.ToLower(mdt.Side) == "buy" {
							// "A/B"
							// A: quan*UA - quan*UA*3/1000
							// B: -price*quan*UB
							if Trade0 == "FSN" {
								ma := new(mgoAccount)
								ma.Address = From
								UpdateBalance4State(ma, state, block.Time().Uint64())
							} else {
								mda := new(mgoDcrmAccount)
								mda.Address = From
								mda.CoinType = Trade0
								mda.Key = (crypto.Keccak256Hash([]byte(mda.Address), []byte(mda.CoinType))).String()
								mda.Timestamp = Timestamp
								UpdateDcrmBalance4State(mda, state)
							}
							if Trade1 == "FSN" {
								ma := new(mgoAccount)
								//to
								ma.Address = From
								UpdateBalance4State(ma, state, block.Time().Uint64())
							} else {
								mda := new(mgoDcrmAccount)
								mda.Address = From
								mda.CoinType = Trade1
								mda.Key = (crypto.Keccak256Hash([]byte(mda.Address), []byte(mda.CoinType))).String()
								UpdateDcrmBalance4State(mda, state)
							}
						} else if strings.ToLower(mdt.Side) == "sell" {
							if Trade0 == "FSN" {
								ma := new(mgoAccount)
								ma.Address = From
								UpdateBalance4State(ma, state, block.Time().Uint64())
							} else {
								mda := new(mgoDcrmAccount)
								mda.Address = From
								mda.CoinType = Trade0
								mda.Key = (crypto.Keccak256Hash([]byte(mda.Address), []byte(mda.CoinType))).String()
								mda.Timestamp = Timestamp
								UpdateDcrmBalance4State(mda, state)
							}
							if Trade1 == "FSN" {
								ma := new(mgoAccount)
								ma.Address = From
								UpdateBalance4State(ma, state, block.Time().Uint64())
							} else {
								mda := new(mgoDcrmAccount)
								mda.Address = From
								mda.CoinType = Trade1
								mda.Key = (crypto.Keccak256Hash([]byte(mda.Address), []byte(mda.CoinType))).String()
								UpdateDcrmBalance4State(mda, state)
							}
						}
					}
					mt.HashLen = len(mr.Done)
					mt.CoinType = trade
					mgoUpdateTransaction(mt, reverse)

					mdb := new(mgoDexBlock)
					mdb.Hash = tx.Hash().String()
					Squence, _ := strconv.ParseInt(txDataSlice[1], 10, 64)
					mdb.Squence = uint64(Squence)
					mdb.Number = block.NumberU64()
					mdb.Orders = mr.Orders
					dtos := mr.Price
					mdb.Price, _ = strconv.ParseFloat(dtos, 64)
					mdb.Trade = trade
					mdb.Timestamp = block.Time().Uint64()
					dtos = mr.Volumes
					mdb.Volumes, _ = strconv.ParseFloat(dtos, 64)
					mgoInsertDexBlock(mdb, reverse)
					return //do not do mgoUpdateTransaction again
				}
			}
		} else {
			mt.Input = hex.EncodeToString(tx.Data())
			UpdataTransferHistoryStatus(tx, reverse)
			txDataSlice := strings.Split(txData, ":")
			mda := new(mgoDcrmAccount)
			mda.Address = getTxFrom(tx).String()
			if mt.TxType == txType_NEWTRADE {
				//DO NOTHING
			} else if mt.TxType == txType_CONFIRMADDRESS {
				logPrintAll(" ==== mgoParseTransaction() ====", "mt.Hash", mt.Hash, "txType", "CONFIRM")
				//account := FindAccount(tbAccounts, mda.Address)
				//if account != nil {
				//	if account[0].IsConfirm == 0 {
				if reverse == false {
					UpdateComfirm(tbAccounts, mda.Address, 1)
				} else {
					UpdateComfirm(tbAccounts, mda.Address, 0)
				}
				//	}
				//}
				mt.CoinType = txDataSlice[2]
			} else {
				mt.CoinType = txDataSlice[3]
				mda.CoinType = txDataSlice[3]
				//fee
				mda.SortId = coinmap[txDataSlice[3]]
				mda.IsERC20, mda.CoinType = getERC20Info(txDataSlice[3])

				//from
				mda.Key = (crypto.Keccak256Hash([]byte(mda.Address), []byte(txDataSlice[3]))).String()
				mda.Timestamp = block.Time().Uint64()
				mt.IsERC20, mt.CoinType = getERC20Info(txDataSlice[3])
				if mt.TxType == txType_LOCKIN {
					logPrintAll(" ==== mgoParseTransaction() ====", "mt.Hash", mt.Hash, "txType", "LOCKIN")
					mt.ContractFrom, mt.ContractTo = getAddressByTxHash(txDataSlice[1], txDataSlice[3])
					//mda.Key = (crypto.Keccak256Hash([]byte(mda.Address), []byte(mda.CoinType))).String()
					logPrintAll("LOCKIN", "mda", mda)
					UpdateDcrmBalance4State(mda, state)
				} else if mt.TxType == txType_LOCKOUT {
					logPrintAll(" ==== mgoParseTransaction() ====", "mt.Hash", mt.Hash, "txType", "LOCKOUT")
					//mda.Address = getTxFrom(tx).String()
					//mda.Key = (crypto.Keccak256Hash([]byte(mda.Address), []byte(mda.CoinType))).String()
					logPrintAll("LOCKOUT", "mda", mda)
					UpdateDcrmBalance4State(mda, state)
					CoinType, err := getFeeCointype(mda.CoinType, mda.IsERC20)
					if err == nil {
						mdaFee := new(mgoDcrmAccount)
						mdaFee.Address = getTxFrom(tx).String()
						mdaFee.CoinType = CoinType
						mdaFee.IsERC20 = 0
						mdaFee.Key = (crypto.Keccak256Hash([]byte(mdaFee.Address), []byte(mdaFee.CoinType))).String()
						logPrintAll("LOCKOUT", "mda(fee)", mdaFee)
						UpdateDcrmBalance4State(mdaFee, state)
					}
					mt.ContractFrom = mt.From
					mt.ContractTo = txDataSlice[1]
				} else if mt.TxType == txType_DCRMTX {
					logPrintAll(" ==== mgoParseTransaction() ====", "mt.Hash", mt.Hash, "txType", "DCRMTX")
					//mda.Address = getTxFrom(tx).String()
					//mda.Key = (crypto.Keccak256Hash([]byte(mda.Address), []byte(mda.CoinType))).String()
					logPrintAll("DCRMTX", "mda1", mda)
					UpdateDcrmBalance4State(mda, state)
					mda2 := new(mgoDcrmAccount)
					mda2.Address = txDataSlice[1]
					mda2.IsERC20, mda2.CoinType = getERC20Info(txDataSlice[3])
					mda2.Key = (crypto.Keccak256Hash([]byte(mda2.Address), []byte(mda2.CoinType))).String()
					logPrintAll("DCRMTX", "mda2", mda2)
					UpdateDcrmBalance4State(mda2, state)
					mt.ContractFrom = mt.From
					mt.ContractTo = txDataSlice[1]
				}
				valueTxf, _ := strconv.ParseFloat(txDataSlice[2], 64)
				mt.ContractValue = valueTxf / getCointypeDiv(mt.CoinType)
			}
		}
	} else {
		UpdataTransferHistoryStatus(tx, reverse)
	}
	ma := new(mgoAccount)
	//from
	ma.Address = getTxFrom(tx).String()
	logPrintAll("==== mgoParseTransaction() ====", "from ma.Address", ma.Address)
	//UpdateAccount(ma, tx.Value().String(), block.Time().Uint64(), reverse, feeFsn)
	UpdateBalance4State(ma, state, block.Time().Uint64())
	//to
	maTo := new(mgoAccount)
	maTo.Address = tx.To().String()
	logPrintAll("==== mgoParseTransaction() ====", "to ma.Address", maTo.Address)
	UpdateBalance4State(maTo, state, block.Time().Uint64())
	mgoUpdateTransaction(mt, reverse)
}

func getERC20Info(cointype string) (int, string) {
	IsERC20 := 0
	CoinType := cointype
	if strings.HasPrefix(cointype, "ERC20") {
		IsERC20 = 1
		CoinType = cointype[5:]
	}
	return IsERC20, CoinType
}

func CheckOrderCacheTxForODB(table, txHash string) bool {
	i := uint64(0)
	for ; i < ConfirmBlockTime; i++ { //forever
		if table == tbOrders {
			if isExistOrder(txHash) {
				logPrintAll("==== CheckOrderCacheTxForODB() ====", "table", table, "txHash", txHash, "spend(s)", i)
				return true
			}
		} else if table == tbOrderCaches {
			if isExistOrderCache(txHash) {
				logPrintAll("==== CheckOrderCacheTxForODB() ====", "table", table, "txHash", txHash, "spend(s)", i)
				return true
			}
		}
		time.Sleep(time.Duration(1) * time.Second)
	}
	return false
}
/*
func mgoUpdateOrderCache(hash string, quantity, price float64, v *orderbook.XvcOrder) {
	//errc := CheckOrderCacheTxForODB(tbOrderCaches, hash)
	//if errc == false {
	//	log.Warn("==== mgoUpdateOrderCaches() ====", "CheckOrderCacheTxForODB not exist hash", hash)
	//}
	go UpdateOrderCache(tbOrderCaches, hash, quantity, price, v)
}

func mgoUpdateOrder(hash string, remain float64) {
	//errc := CheckOrderCacheTxForODB(tbOrders, hash)
	//if errc == false {
	//	log.Warn("==== mgoUpdateOrder() ====", "CheckOrderCacheTxForODB not exist hash", hash)
	//}
	UpdateOrder(hash, remain.String())
}
*/

func getTxType(tx *types.Transaction) int {
	txdata := tx.Data()
	if types.IsDcrmTransaction(tx) {
		return txType_DCRMTX
	}
	if types.IsDcrmLockIn(tx) {
		return txType_LOCKIN
	}
	if IsDcrmLockOut(txdata) {
		return txType_LOCKOUT
	}
	if types.IsXvcTx(tx) {
		return txType_DEX
	}
	if types.IsDcrmConfirmAddr(tx) {
		return txType_CONFIRMADDRESS
	}
	if IsDexOrder(txdata) {
		return txType_ORDER
	}
	if types.IsAddNewTradeTx(tx) {
		return txType_NEWTRADE
	}
	return txType_TX
}

func IsDexOrder(data []byte) bool {
	str := string(data)
	if len(str) == 0 {
		return false
	}

	realtxdata, _ := types.GetRealTxData(str)
	m := strings.Split(realtxdata, ":")
	if m[0] == "ORDER" {
		return true
	}

	return false
}

func IsDcrmLockOut(data []byte) bool {
	str := string(data)
	if len(str) == 0 {
		return false
	}

	realtxdata, _ := types.GetRealTxData(str)
	m := strings.Split(realtxdata, ":")
	if m[0] == "LOCKOUT" {
		return true
	}

	return false
}

func getTxFrom(tx *types.Transaction) common.Address {
	signer := types.NewEIP155Signer(tx.ChainId())
	from, err := types.Sender(signer, tx)
	if err == nil {
		return from
	} else {
		return common.Address{}
	}
}

func mgoUpdateTransaction(mt *mgoTransaction, reverse bool) {
	logPrintAll("==== mgoUpdateTransaction() ====", "mt", mt, "reverse", reverse)
	if !reverse {
		AddTransaction(tbTransactions, mt)
	} else {
		DeleteTransaction(tbTransactions, mt)
	}
}

func mgoRemoveTransaction(mt *mgoTransaction) {
	go DeleteTransaction(tbTransactions, mt)
}

func mgoParseBlock(block *types.Block, reverse bool) {
	go parseBlock(block, reverse)
}

func parseBlock(block *types.Block, reverse bool) {
	logPrintAll("==== parseBlock() ====", "block", block.NumberU64())
	mb := new(mgoBlock)

	mb.Number = block.NumberU64()
	mb.Key = block.Hash().String()
	mb.Hash = block.Hash().String()
	mb.ParentHash = block.ParentHash().String()
	mb.Nonce = fmt.Sprintf("0x%x", block.Nonce())
	mb.Sha3Uncles = block.UncleHash().String()
	mb.LogsBloom = fmt.Sprintf("%v", block.Bloom())
	mb.TransactionsRoot = block.TxHash().String()
	mb.StateRoot = block.Root().String()
	mb.ReceiptHash = block.ReceiptHash().String()
	mb.Miner = block.Coinbase().String()
	mb.Difficulty = block.Difficulty().Uint64()
	mb.TotalDifficulty = getTotalDifficultyInfo(mb.Number-1) + mb.Difficulty
	mb.Size = float64(block.Size())
	mb.ExtraData = "0x" + hex.EncodeToString(block.Extra())
	mb.GasLimit = block.GasLimit()
	mb.GasUsed = block.GasUsed()
	mb.Timestamp = block.Time().Uint64()
	if mb.Number > 1 {
		mb.BlockTime = mb.Timestamp - getTimestampInfo(mb.Number-1)
	} else if mb.Number == 1 {
		mb.BlockTime = 5 //clique.FsnPeriod
	} else {
		mb.BlockTime = 0
	}
	mb.Txns = block.Transactions().Len()
	if mb.Miner != (common.Address{}).String() {
		reward := fmt.Sprintf("%v", clique.FsnBlockReward)
		mb.Reward = reward

	}
	avg := big.NewInt(0)
	txns := int64(0)
	txs := block.Transactions()
	for _, tx := range txs {
		gas := tx.GasPrice()
		avg = new(big.Int).Add(avg, gas)
		txns++
	}
	if txns > 0 {
		avg = new(big.Int).Div(avg, big.NewInt(txns))
	}
	mb.AvgGasprice = avg.String()
	mb.Uncles = block.Uncles()
	go mgoInsertBlock(mb, false)
}

func mgoInsertBlock(mb *mgoBlock, reverse bool) {
	logPrintAll("==== mgoInsertBlock() ====", "number", mb.Number, "reverse", reverse)
	if reverse {
		latestBlockNumber = mb.Number - 1
		DeleteBlock(tbBlocks, mb.Number)
		blockInfo.Number = mb.Number - 1
		blockInfo.TotalDifficulty = getTotalDifficulty(mb.Number - 1)
		blockInfo.Timestamp = getTimestamp(mb.Number - 1)
	} else {
		latestBlockNumber = mb.Number
		AddBlock(tbBlocks, mb)
		blockInfo.Number = mb.Number
		blockInfo.TotalDifficulty = mb.TotalDifficulty
		blockInfo.Timestamp = mb.Timestamp
	}
}

func mgoInsertDcrmAccount(mda *mgoDcrmAccount) {
	AddDcrmAccount(tbDcrmAccounts, mda)
}

func mgoInsertAccount(ma *mgoAccount) {
	AddAccount(tbAccounts, ma)
}

func getBlockReward(num uint64) string {
	fb := FindBlock(tbBlocks, num)
	if len(fb) > 0 {
		return fb[0].Reward
	}
	return "0"
}

func getTimestampInfo(num uint64) uint64 {
	if blockInfo.Number == num {
		return blockInfo.Timestamp
	} else {
		logPrintAll("getTimestampInfo", "err, num", num)
		return 0
	}
}

func getTimestamp(num uint64) uint64 {
	fb := FindBlock(tbBlocks, num)
	if len(fb) > 0 {
		return fb[0].Timestamp
	}
	return 0
}

func getTotalDifficultyInfo(num uint64) uint64 {
	if blockInfo.Number == num {
		return blockInfo.TotalDifficulty
	} else {
		logPrintAll("getTotalDifficultyInfo", "err, num", num)
		return 0
	}
}

func getTotalDifficulty(num uint64) uint64 {
	if num >= 0 {
		fb := FindBlock(tbBlocks, num)
		if len(fb) > 0 {
			return fb[0].TotalDifficulty
		}
	}
	return 0
}

func AddHistory(srcTx *types.Transaction) {
	if !Mongo {
		return
	}
	tx := &(*srcTx)
	go func() {
		logPrintAll("==== AddHistory() ====", "", "")
		mth := new(mgoHistory)

		mth.Hash = tx.Hash().String()
		mth.Nonce = tx.Nonce()
		mth.From = getTxFrom(tx).String()
		mth.To = tx.To().String()
		value := tx.Value().String()
		valuef, _ := strconv.ParseFloat(value, 64)
		mth.Value = valuef / getCointypeDiv("FSN")
		mth.GasPrice = tx.GasPrice().String()
		mth.GasLimit = tx.Gas()
		mth.Timestamp = uint64(time.Now().Unix())
		mth.CoinType = "FSN"
		TxType := getTxType(tx)
		mth.Status = 0
		stat := fmt.Sprintf("%v", mth.Status)
		mth.Key = (crypto.Keccak256Hash([]byte(mth.Hash), []byte(stat))).String()
		txData := string(tx.Data())
		if len(txData) > 0 {
			logPrintAll("==== AddHistory() ====", "mth.Hash", mth.Hash, "txData", txData)
			mth.Input = fmt.Sprintf("%x", txData)
			txDataSlice := strings.Split(txData, ":")
			if TxType == txType_CONFIRMADDRESS {
				return
			}

			_, mth.CoinType = getERC20Info(txDataSlice[3])
			//TODO
			mth.ContractTo = txDataSlice[1]
			valueTxf, _ := strconv.ParseFloat(txDataSlice[2], 64)
			mth.ContractValue = valueTxf / getCointypeDiv(mth.CoinType)

			if TxType == txType_LOCKIN {
				mth.ContractTo = ""
				mth.OutHash = txDataSlice[1]
				mgoAddHistory(tbLockinHistory, mth)
			} else if TxType == txType_LOCKOUT {
				mgoAddHistory(tbLockoutHistory, mth)
			} else if TxType == txType_DCRMTX {
				mgoAddHistory(tbTransferHistory, mth)
			} else if TxType == txType_ORDER {
				//mgoAddOrder(tx)
				//mgoAddOrderCache(tx)
			}
		} else { // FSN
			logPrintAll("AddHistory()", "fsn mgoAddTransferHistory", mth)
			mgoAddHistory(tbTransferHistory, mth)
		}
	}()
}

func UpdataTransferHistoryStatus(tx *types.Transaction, reverse bool) {
	if !Mongo {
		return
	}
	logPrintAll("==== UpdataTransferHistoryStatus() ====", "reverse", reverse)
	txType := getTxType(tx)
	table := tbTransferHistory
	if txType == txType_LOCKIN {
		table = tbLockinHistory
	} else if txType == txType_LOCKOUT {
		table = tbLockoutHistory
	} else if txType == txType_DCRMTX {
		table = tbTransferHistory
	}
	if !reverse {
		UpdataTransferHistoryStatus2(table, tx.Hash().String(), true)
	} else {
		//UpdataTransferHistoryStatus2(table, tx.Hash().String(), false)
	}
}

func getCointypeDiv(cointype string) float64 {
	if cointype == "FSN" || cointype == "ETH" ||
		cointype == "ERC20BNB" || cointype == "BNB" ||
		cointype == "ERC20GUSD" || cointype == "GUSD" ||
		cointype == "ERC20MKR" || cointype == "MKR" ||
		cointype == "ERC20HT" || cointype == "HT" {
		return DIVE18
	}
	if cointype == "BTC" || cointype == "BCH" || cointype == "USDT" {
		return DIVE8
	}
	if cointype == "TRX" || cointype == "XRP" {
		return DIVE6
	}
	if cointype == "EVT1" {
		return DIVE5
	}
	if cointype == "EOS" {
		return DIVE4
	}
	if cointype == "ATOM" {
		return DIVE3
	}
	return float64(1)
}

func mgoInsertDexBlock(mdb *mgoDexBlock, reverse bool) {
	if !reverse {
		AddDexBlock(tbDexBlocks, mdb)
	} else {
		RemoveDexBlock(tbDexBlocks, mdb)
	}
}

func mgoInsertDexTx(mdt *mgoDexTx, reverse bool) {
	if !reverse {
		AddDexTx(tbDexTxns, mdt)
	} else {
		RemoveDexTx(tbDexTxns, mdt)
	}
}

func mgoEmptyOrder() {
	emptyTable(tbOrders)
}

func mgoEmptyOrderCache() {
	emptyTable(tbOrderCaches)
}

func mgoAddOrderCache(tx *types.Transaction) {
	moc := new(mgoOrderCache)
	moc.Key = tx.Hash().String()
	moc.Hash = tx.Hash().String()
	txData := string(tx.Data())
	txDataSlice := strings.Split(txData, ":")
	moc.Trade = txDataSlice[2]
	moc.Side = strings.ToLower(txDataSlice[4])
	moc.Price, _ = strconv.ParseFloat(txDataSlice[5], 64)
	moc.Volume, _ = strconv.ParseFloat(txDataSlice[6], 64)
	moc.Total = moc.Price * moc.Volume
	go AddOrderCache(tbOrderCaches, moc, INSERT)
}

//txhash:tx:fusionaddr:trade:ordertype:side:price:quanity:rule:time
func mgoInsertOrderCache(msg string) {
	logPrintAll("==== mgoInsertOrderCache() ====", "msg", msg)
	m := strings.Split(msg, sep9)
	moc := new(mgoOrderCache)
	moc.Key = m[0]
	moc.Hash = m[0]
	//txData := string(tx.Data())
	//txDataSlice := strings.Split(txData, ":")
	moc.Trade = m[3]
	moc.Side = strings.ToLower(m[5])
	moc.Price, _ = strconv.ParseFloat(m[6], 64)
	moc.Price /= MUL1E10
	moc.Volume, _ = strconv.ParseFloat(m[7], 64)
	moc.Volume /= MUL1E10
	moc.Total = moc.Price * moc.Volume
	go AddOrderCache(tbOrderCaches, moc, INSERT)
}

func InsertOrderCache(hash, price, quantity, trade, side string) {
	if !Mongo {
		return
	}
	go func () {
		OrderCacheLock.Lock()
		defer OrderCacheLock.Unlock()
		oc, _ := orderCache.Load(hash)
		if oc == EXIST  {
			return
		}
		orderCache.Store(hash, EXIST)
		moc := new(mgoOrderCache)
		moc.Key = (crypto.Keccak256Hash([]byte(price), []byte(trade), []byte(side))).String()
		logPrintAll("==== mgoInsertOrderCache() ====", "price", price, "quantity", quantity, "trade", trade, "side", side, "Key", moc.Key)
		moc.Trade = trade
		moc.Side = strings.ToLower(side)
		moc.Price, _ = strconv.ParseFloat(price, 64)
		moc.Price /= MUL1E10
		moc.Volume, _ = strconv.ParseFloat(quantity, 64)
		moc.Volume /= MUL1E10
		moc.Total = moc.Price * moc.Volume
		oci, _ := orderCacheInfo.Load(moc.Key)
		if oci == EXIST  {
			AddOrderCache(tbOrderCaches, moc, UPDATE)
		} else {
			AddOrderCache(tbOrderCaches, moc, INSERT)
		}
		orderCacheInfo.Store(moc.Key, EXIST)
	}()
}

func RemoveOrderCache(price, trade, side string) {
	if !Mongo {
		return
	}
	go func () {
		OrderCacheLock.Lock()
		defer OrderCacheLock.Unlock()
		Key := (crypto.Keccak256Hash([]byte(price), []byte(trade), []byte(side))).String()
		removeOrderCache(tbOrderCaches, Key)
		logPrintAll("==== RemoveOrderCache() ====", "price", price, "trade", trade, "side", side, "Key", Key)
		orderCacheInfo.Delete(Key)
	}()
}

/*
func mgoInsertOrderCacheFromDone(v *orderbook.XvcOrder) {
	logPrintAll("==== mgoInsertOrderCacheFromDone() ====", "v", v)
	moc := new(mgoOrderCache)
	moc.Key = v.ID()
	moc.Hash = v.ID()
	//txData := string(tx.Data())
	//txDataSlice := strings.Split(txData, ":")
	moc.Trade = v.TRADE()
	moc.Side = strings.ToLower(v.SIDE())
	moc.Price, _ = strconv.ParseFloat(fmt.Sprintf("%v", v.PRICE()), 64)
	moc.Volume, _ = strconv.ParseFloat(fmt.Sprintf("%v", v.QUANTITY()), 64)
	moc.Total = (moc.Price * MUL1E9) * (moc.Volume * MUL1E9) / MUL1E9 / MUL1E9
	go AddOrderCache(tbOrderCaches, moc)
}

func mgoInsertOrderFromDone(v *orderbook.XvcOrder) {
	//OrderAndCacheLock.Lock()
	//defer OrderAndCacheLock.Unlock()
	logPrintAll("==== mgoInsertOrderFromDone() ====", "v", v)
	mo := new(mgoOrder)
	mo.Key = v.ID()
	mo.Hash = v.ID()
	//txData := string(tx.Data())
	//txDataSlice := strings.Split(txData, ":")
	mo.Pair = v.TRADE()
	mo.Side = strings.ToLower(v.SIDE())
	mo.Price, _ = strconv.ParseFloat(fmt.Sprintf("%v", v.PRICE()), 64)
	mo.Quantity, _ = strconv.ParseFloat(fmt.Sprintf("%v", v.QUANTITY()), 64)
	mo.Value = (mo.Price * MUL1E9) * (mo.Quantity * MUL1E9) / MUL1E9 / MUL1E9
	mo.Timestamp = uint64(v.TIME())
	go AddOrder(tbOrders, mo)
}
*/
//msg
//txhash:tx:fusionaddr:trade:ordertype:side:price:quanity:rule:time
func mgoInsertOrder(msg string) bool {
	logPrintAll("mongodb ==== mgoInsertOrder() ====", "msg", msg)
	m := strings.Split(msg, sep9)
	mo := new(mgoOrder)
	mo.Key = m[0]
	mo.Hash = m[0]
	//if isExistOrder(mo.Hash) {
	//	return
	//}
	//mo.Nonce = tx.Nonce()
	mo.From = m[2]
	//mo.To = tx.To().String()
	//mo.GasPrice = tx.GasPrice().String()
	//mo.GasLimit = tx.Gas()
	//mo.Data = m[1]

	mo.Pair = m[3]
	mo.Side = strings.ToLower(m[5])
	mo.Price, _ = strconv.ParseFloat(m[6], 64)
	mo.Price /= MUL1E10
	mo.Quantity, _ = strconv.ParseFloat(m[7], 64)
	mo.Quantity /= MUL1E10
	tm, _ := strconv.Atoi(m[9])
	mo.Timestamp = uint64(tm)

	mo.Value = (mo.Price * MUL1E9) * (mo.Quantity * MUL1E9) / MUL1E9 / MUL1E9
	return AddOrder(tbOrders, mo)
}

//txhash:tx:fusionaddr:trade:ordertype:side:price:quanity:rule:time
func mgoUpdateOrderOrg(msg string) {
	logPrintAll("==== mgoInsertOrder() ====", "msg", msg)
	m := strings.Split(msg, sep9)
	mo := new(mgoOrder)
	mo.Key = m[0]
	mo.Hash = m[0]
	mo.From = m[2]
	mo.Pair = m[3]
	mo.Side = strings.ToLower(m[5])
	mo.Price, _ = strconv.ParseFloat(m[6], 64)
	mo.Quantity, _ = strconv.ParseFloat(m[7], 64)
	tm, _ := strconv.Atoi(m[9])
	mo.Timestamp = uint64(tm)

	mo.Value = (mo.Price * MUL1E9) * (mo.Quantity * MUL1E9) / MUL1E9 / MUL1E9
	UpdateOrderOrg(tbOrders, mo)
}

func mgoAddOrder(tx *types.Transaction) {
	logPrintAll("==== mgoAddOrder() ====", "tx", tx)
	mo := new(mgoOrder)
	mo.Key = tx.Hash().String()
	mo.Hash = tx.Hash().String()
	//if isExistOrder(mo.Hash) {
	//	return
	//}
	mo.Nonce = tx.Nonce()
	mo.From = getTxFrom(tx).String()
	mo.To = tx.To().String()
	mo.GasPrice = tx.GasPrice().String()
	mo.GasLimit = tx.Gas()
	txData := string(tx.Data())
	mo.Data = txData

	txDataSlice := strings.Split(txData, ":")
	mo.Pair = txDataSlice[2]
	mo.Side = strings.ToLower(txDataSlice[4])
	mo.Price, _ = strconv.ParseFloat(txDataSlice[5], 64)
	mo.Quantity, _ = strconv.ParseFloat(txDataSlice[6], 64)
	mo.Timestamp = uint64(time.Now().Unix())

	mo.Value = (mo.Price * MUL1E9) * (mo.Quantity * MUL1E9) / MUL1E9 / MUL1E9
	go AddOrder(tbOrders, mo)
}

func UpdateOrderCacheFromShare(msg interface{}) {
	log.Info("mongodb ==== UpdateOrderCacheFromShare() ====", "msg", msg)
	if !Mongo {
		return
	}
	res := msg.(string)
	res, err := UnCompress(res)
	if err != nil {
		return
	}
	r, err := Decode(res)
	if err != nil {
		return
	}

	//switch r.(type) {
	//case *SendMsg:
	rr := r.(*SendMsg)
	if rr.MsgType == "rpc_order_create" {
		//UpdateOrderCache(rr.Msg)
		m := strings.Split(rr.Msg, "dcrmsep9")
		addOrderCache(m[0], m[1])
	}
	//}
}

//call from InsertToOrderBook() xprotocol/exchange.go
func InsertOrderAndCache(msg string) {
	if !Mongo {
		return
	}
	go func () {
		OrderAndCacheLock.Lock()
		defer OrderAndCacheLock.Unlock()
		logPrintAll("==== InsertOrderAndCache() ====", "msg", msg)
		if database == nil {
			mongoInit()
		}
		m := strings.Split(msg, sep9)
		oci, _ := orderInfo.Load(m[0])
		if oci != nil {
			return
		}
		mgoInsertOrder(msg)
		//mgoInsertOrderCache(msg)
		//orderInfo[m[0]] = EXIST
		Quantity, _ := strconv.ParseFloat(m[7], 64)
		Quantity /= MUL1E10
		orderInfo.Store(m[0], Quantity)
		logPrintAll("==== InsertOrderAndCache() ====", "mgoInsertOrder, hash", m[0])
	}()
}

func UpdateOrderAddCache(tx *types.Transaction) {
	logPrintAll("==== UpdateOrderAddCache() ====", "tx", tx)
	if !Mongo {
		return
	}
	if database == nil {
		mongoInit()
	}
	mgoAddOrder(tx)
	addOrderCache(tx.Hash().String(), string(tx.Data()))
}

func addOrderCache(txHash string, msg string) {
	logPrintAll("==== addOrderCache() ====", "msg", msg)
	//m := strings.Split(msg, "dcrmsep9")
	m := strings.Split(msg, ":")
	if len(m) < 7 {
		return
	}
	//if isExistOrder(m[0]) {
	//if isExistOrderCache(txHash) {
	//	logPrintAll("==== addOrderCache() ====", "isExistOrderCache", txHash)
	//	return
	//}
	moc := new(mgoOrderCache)
	moc.Key = txHash
	moc.Hash = txHash
	moc.Trade = m[2]
	moc.Side = m[4]
	moc.Price, _ = strconv.ParseFloat(m[5], 64)
	moc.Volume, _ = strconv.ParseFloat(m[6], 64)
	moc.Total = moc.Price * moc.Volume
	go AddOrderCache(tbOrderCaches, moc, INSERT)
}
/*
func MgoUpdateOrderAndCaches(done []*orderbook.XvcOrder, curvolEx []string) {
	if !Mongo {
		return
	}
	go func() {
		logPrintAll("==== MgoUpdateOrderAndCaches() ====")
		for i, v := range done {
			logPrintAll(" ==== MgoUpdateOrderAndCaches() ====", "done", v)
			orderHash := v.ID()
			volumeS := curvolEx[i]
			volumeS = customerString(volumeS)
			volume, _ := strconv.ParseFloat(volumeS, 64)
			completedS := v.QUANTITY()
			completedS = customerString(completedS)
			completed, _ := strconv.ParseFloat(completedS, 64)
			if volume >= completed {
				//mgoUpdateOrder(orderHash, volume-completed)
				priceS := v.PRICE()
				//priceS = customerString(priceS)
				price, _ := strconv.ParseFloat(priceS, 64)
				mgoUpdateOrderCache(orderHash, volume-completed, price, v)
			}
		}
	}()
}
*/
func customerString(number string) string {
	dtos := number
	n := strings.Index(dtos, ".")
	if n > 0 {
		if n+QuantityAccuracy+1 < len(dtos) {
			dtos = dtos[:n+QuantityAccuracy+1]
		}
	}
	return dtos
}

func UpdateOrderFromShare(tx *types.Transaction) {
	if !Mongo {
		return
	}
	//OrderAndCacheLock.Lock()
	//defer OrderAndCacheLock.Unlock()
	data := string(tx.Data())
	logPrintAll("==== UpdateOrderFromShare() ====", "data", data, "MatchTrade", MatchTrade)
	if data == "" {
		return
	}
	m := strings.Split(data, dexDataSplit)
	if m[0] != "ODB" || strings.EqualFold(MatchTrade, m[3]) {
		return
	}

	mr := GetMrFromMatch(m[2])
	if mr == nil {
		return
	}

	logPrintAll("==== UpdateOrderFromShare() ====", "match nonce", m[1], "trade", m[3])
	curvol := mr.CurVolume
	curvolString := []string{}
	for i := 0; i < len(curvol); i++ {
		curvolString = append(curvolString, fmt.Sprintf("%v", curvol[i].Vol))
	}
	//MgoUpdateOrderAndCaches(mr.Done, curvolString) //TODO
}

func GetMrFromMatch(data string) *MatchRes {
	v_uncompress, erru := UnCompress(data)
	if erru != nil {
		return nil
	}
	mr, errd := DecodeMatchRes(v_uncompress)
	if errd != nil {
		return nil
	}

	if mr != nil {
		return mr
	}
	return nil
}

