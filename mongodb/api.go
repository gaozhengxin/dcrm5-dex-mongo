package mongodb

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	//"github.com/fusion/go-fusion/consensus/clique"
	"github.com/fusion/go-fusion/common"
	"github.com/fusion/go-fusion/crypto"
	"github.com/fusion/go-fusion/log"
	//"github.com/fusion/go-fusion/xprotocol/orderbook"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

var (
	Session *mgo.Session
	InitOnce bool
)

func mongoInit() {
	mongoServerInit()
	mgoEmptyOrder()
	mgoEmptyOrderCache()

}

func mongoServerInit() {
	if InitOnce {
		return
	}
	if Session != nil {
		Session.Refresh()
	}
	InitOnce = true
	url := fmt.Sprintf("mongodb://%v", MongoIP) //url := "localhost"
	fmt.Printf("mongodb url %v\n", url)
	for {
		session, err := mgo.Dial(url)
		if err != nil {
			log.Warn("mgo.Dial", "url", url, "fail", err)
			time.Sleep(time.Duration(1) * time.Second)
			continue
		}
		Session = session
		break
	}
	Session.SetMode(mgo.Monotonic, true)
	database = Session.DB(dbname)
	fmt.Printf("mongodb mongoServerInit finished.\n")
	ConfirmBlockTime = ConfirmBlockNumber * 5 //clique.FsnPeriod
	InitOnce = false
}

func getCollection(table string) *mgo.Collection {
	if InitOnce {
		return nil
	}
	if err := Session.Ping(); err != nil {
		log.Warn("Session", "ping error", err)
		go mongoServerInit()
		return nil
	}
	if table == tbBlocks {
		if collectionBlock == nil {
			collectionBlock = database.C(table)
		}
		return collectionBlock
	} else if table == tbAccounts {
		if collectionAccount == nil {
			collectionAccount = database.C(table)
		}
		return collectionAccount
	} else if table == tbTransferHistory {
		if collectionTransferHistory == nil {
			collectionTransferHistory = database.C(table)
		}
		return collectionTransferHistory
	} else if table == tbLockinHistory {
		if collectionLockinHistory == nil {
			collectionLockinHistory = database.C(table)
		}
		return collectionLockinHistory
	} else if table == tbLockoutHistory {
		if collectionLockoutHistory == nil {
			collectionLockoutHistory = database.C(table)
		}
		return collectionLockoutHistory
	} else if table == tbDexBlocks {
		if collectionDexBlock == nil {
			collectionDexBlock = database.C(table)
		}
		return collectionDexBlock
	} else if table == tbDexTxns {
		if collectionDexTx == nil {
			collectionDexTx = database.C(table)
		}
		return collectionDexTx
	} else if table == tbOrderCaches {
		if collectionOrderCache == nil {
			collectionOrderCache = database.C(table)
		}
		return collectionOrderCache
	} else if table == tbOrders {
		if collectionOrder == nil {
			collectionOrder = database.C(table)
		}
		return collectionOrder
	} else if table == tbDcrmAccounts {
		if collectionDcrmAccount == nil {
			collectionDcrmAccount = database.C(table)
		}
		return collectionDcrmAccount
	} else if table == tbTransactions {
		if collectionTransaction == nil {
			collectionTransaction = database.C(table)
		}
		return collectionTransaction
	}
	return nil
}

func mgoAddHistory(table string, mth *mgoHistory) {
	logPrintAll("==== mgoAddHistory() ====", "mth", mth)
	collectionTable := getCollection(table)
	if collectionTable == nil {
		log.Warn("getCollection()", "table", table, "collectionTable", "nil")
		return
	}
	err := collectionTable.Insert(mth)
	if err == nil {
		logPrintAll("Insert success", "table", table, "from", mth.From, "to", mth.To)
	} else {
		logPrintAll("mongodb Insert failed", "table", table, "from", mth.From, "to", mth.To, "err", err)
	}
}

func UpdataTransferHistoryStatus2(table, hash string, success bool) {
	logPrintAll("==== UpdataTransferHistoryStatus2() ====", "table", table, "key", hash, "success", success)

	Key := (crypto.Keccak256Hash([]byte(hash), []byte("0"))).String()
	Key2 := ""
	logPrintAll("==== UpdataTransferHistoryStatus() ====", "key", Key, "success", success)
	status := 2
	Key2 = (crypto.Keccak256Hash([]byte(hash), []byte("2"))).String()
	if success {
		status = 1
		Key2 = (crypto.Keccak256Hash([]byte(hash), []byte("1"))).String()
	}
	logPrintAll("==== UpdataTransferHistoryStatus() ====", "key", Key, "key2", Key2, "success", success)
	collectionTable := getCollection(table)
	if collectionTable == nil {
		log.Warn("getCollection()", "table", table, "collectionTable", "nil")
		return
	}
	collectionTable.Update(bson.M{"key": Key}, bson.M{"$set": bson.M{
		"key":    Key2,
		"status": status,
	}})
}

func FindTransferHistory(table, key string) []mgoHistory {
	collectionTable := getCollection(table)
	if collectionTable == nil {
		log.Warn("getCollection()", "table", table, "collectionTable", "nil")
		return nil
	}
	t := make([]mgoHistory, 1)
	err := collectionTable.Find(bson.M{"key": key}).One(&t[0])
	if err == nil {
		logPrintAll("==== FindTransferHistory() ===", "Found key", key)
		return t
	}
	return nil
}

func emptyTable(table string) {
	log.Info("mongodb ==== emptyTable() ===", "table", table)
	//OrderCacheLock.Lock()
	//defer OrderCacheLock.Unlock()
	collectionTable := getCollection(table)
	if collectionTable == nil {
		log.Warn("getCollection()", "table", table, "collectionTable", "nil")
		return
	}
	collectionTable.RemoveAll(nil) //v < 3.2
	//collectionTable.DeleteMany()//v > 3.2
}

func AddOrderCache(table string, moc *mgoOrderCache, iu int) {
	logPrintAll("==== AddOrderCache() ====", "moc", moc)
	//OrderCacheLock.Lock()
	//defer OrderCacheLock.Unlock()
	err := errors.New("")
	collectionTable := getCollection(table)
	if collectionTable == nil {
		log.Warn("getCollection()", "table", table, "collectionTable", "nil")
		return
	}
	if iu == UPDATE {
		logPrintAll("==== AddOrderCache() ====", "update volume", moc.Volume, "total", moc.Total, "Key", moc.Key)
		err = collectionTable.Update(bson.M{"_id": moc.Key}, bson.M{"$set": bson.M{
			"volume": moc.Volume,
			"total": moc.Total,
		}})
	} else {
		logPrintAll("==== AddOrderCache() ====", "insert moc", moc)
		err = collectionTable.Insert(moc)
	}
	if err == nil {
		logPrintAll("==== AddOrderCache() ====", "Insert success: AddOrderCache, ", moc)
	} else { //exist
		logPrintAll(" ==== AddOrderCache() ====", "Insert failed: AddOrderCache, ", moc, "error", err)
	}
}

func AddOrder(table string, mo *mgoOrder) bool {
	logPrintAll("==== AddOrder() ====", "mo", mo)
	OrderLock.Lock()
	defer OrderLock.Unlock()
	//if isExistOrder(mo.Hash) {
	//	return
	//}
	collectionTable := getCollection(table)
	if collectionTable == nil {
		log.Warn("getCollection()", "table", table, "collectionTable", "nil")
		return false
	}
	err := collectionTable.Insert(mo)
	if err == nil {
		logPrintAll("==== AddOrder() ====", "Insert success, AddOrder", mo)
		return true
	} else {
		logPrintAll(" ==== AddOrder() ====", "Insert failed, AddOrder", mo, "error", err)
		return false
	}
}

func UpdateOrderOrg(table string, mo *mgoOrder) {
	logPrintAll("==== AddOrderOrg() ====", "mo", mo)
	OrderLock.Lock()
	defer OrderLock.Unlock()
	//if isExistOrder(mo.Hash) {
	//	return
	//}
	collectionTable := getCollection(table)
	if collectionTable == nil {
		log.Warn("getCollection()", "table", table, "collectionTable", "nil")
		return
	}
	err := collectionTable.Update(bson.M{"_id": mo.Hash}, bson.M{"$set": bson.M{
		"quantity":  mo.Quantity,
		"value":     mo.Value,
		"timestamp": mo.Timestamp,
	}})
	if err == nil {
		logPrintAll("==== UpdateOrderOrg() ====", "success, Order", mo)
	} else {
		logPrintAll("==== UpdateOrderOrg() ====", "failed, Order", mo)
	}
}

func AddDexTx(table string, mdt *mgoDexTx) {
	//DexTxnsLock.Lock()
	//defer DexTxnsLock.Unlock()
	logPrintAll("==== AddDexTx() ====", "mdt", mdt)
	collectionTable := getCollection(table)
	if collectionTable == nil {
		log.Warn("getCollection()", "table", table, "collectionTable", "nil")
		return
	}
	err := collectionTable.Insert(mdt)
	if err == nil {
		logPrintAll("==== AddDexTx() ====", "Insert success, AddDexTx height", mdt.Height, "Number", mdt.Number, "mdt.Hash", mdt.Hash)
	} else {
		logPrintAll(" ==== AddDexTx() ====", "Insert failed, AddDexTx height", mdt.Height, "Number", mdt.Number, "mdt.Hash", mdt.Hash, "err", err)
	}
}

func RemoveDexTx(table string, mdt *mgoDexTx) {
	//DexTxnsLock.Lock()
	//defer DexTxnsLock.Unlock()
	logPrintAll("==== RemoveDexTx() ====", "mdt", mdt)
	collectionTable := getCollection(table)
	if collectionTable == nil {
		log.Warn("getCollection()", "table", table, "collectionTable", "nil")
		return
	}
	err := collectionTable.Remove(bson.M{"_id": mdt.Key})
	if err == nil {
		logPrintAll("==== RemoveDexTx() ====", "Remove success, RemoveDexTx height", mdt.Height, "Number", mdt.Number, "mdt.Hash", mdt.Hash)
	} else {
		logPrintAll("==== RemoveDexTx() ====", "Remove failed, RemoveDexTx height", mdt.Height, "Number", mdt.Number, "mdt.Hash", mdt.Hash)
	}
}

func AddDexBlock(table string, mdb *mgoDexBlock) {
	DexBlockLock.Lock()
	defer DexBlockLock.Unlock()
	collectionTable := getCollection(table)
	if collectionTable == nil {
		log.Warn("getCollection()", "table", table, "collectionTable", "nil")
		return
	}
	err := collectionTable.Insert(mdb)
	if err == nil {
		logPrintAll("==== AddDexBlock() ====", "Insert success: AddDexBlock", mdb)
	} else {
		logPrintAll("==== AddDexBlock() ====", "Insert failed: AddDexBlock", mdb, "error", err)
	}
}

func RemoveDexBlock(table string, mdb *mgoDexBlock) {
	DexBlockLock.Lock()
	defer DexBlockLock.Unlock()
	collectionTable := getCollection(table)
	if collectionTable == nil {
		log.Warn("getCollection()", "table", table, "collectionTable", "nil")
		return
	}
	err := collectionTable.Remove(bson.M{"_id": mdb.Hash})
	if err == nil {
		logPrintAll("==== RemoveDexBlock() ====", "Remove success: RemoveDexBlock", mdb)
	} else {
		logPrintAll("==== RemoveDexBlock() ====", "Remove failed: RemoveDexBlock", mdb)
	}
}

func AddTransaction(table string, mt *mgoTransaction) {
	collectionTable := getCollection(table)
	if collectionTable == nil {
		log.Warn("getCollection()", "table", table, "collectionTable", "nil")
		return
	}
	err := collectionTable.Insert(mt)
	if err == nil {
		logPrintAll("==== AddTransaction() ====", "Insert success: mgoTransaction blockNumber", mt.BlockNumber)
	} else {
		logPrintAll("==== AddTransaction() ====", "Insert failed: mgoTransaction blockNumber", mt.BlockNumber, "error", err)
	}
}

func DeleteTransaction(table string, mt *mgoTransaction) {
	if collectionTransaction == nil {
		collectionTransaction = database.C(table)
	}
	err := collectionTransaction.Remove(bson.M{"hash": mt.Hash})
	if err == nil {
		logPrintAll("==== DeleteTransaction() ====", "delete success", "", "hash", mt.Hash)
	} else {
		logPrintAll("==== DeleteTransaction() ====", "delete fail err", err, "hash", mt.Hash)
	}
}

func AddBlock(table string, mb *mgoBlock) {
	logPrintAll("==== AddBlock() ====", "number", mb.Number)
	collectionTable := getCollection(table)
	if collectionTable == nil {
		log.Warn("getCollection()", "table", table, "collectionTable", "nil")
		return
	}
	//BlockLock.Lock()
	//defer BlockLock.Unlock()
	err := collectionTable.Insert(mb)
	if err == nil {
		logPrintAll("==== AddBlock() ====", "Insert success: mgoBlock number", mb.Number)
	} else {
		logPrintAll("==== AddBlock() ====", "Insert failed: mgoBlock number", mb.Number, "error", err)
	}
}

func AddAddress(address, publicKey string) {
	if !Mongo {
		return
	}
	logPrintAll("==== AddAddress() ====", "address", address, "publicKey", publicKey)
	ma := new(mgoAccount)
	ma.Address = address
	ma.Key = ma.Address
	ma.PublicKey = publicKey
	if FindAccount(tbAccounts, ma.Address) != nil {
		go UpdateAccountPublicKey(tbAccounts, ma.Address, ma.PublicKey)
	} else {
		logPrintAll("AddAddress call AddAccount", "address", address, "publicKey", publicKey)
		go AddAccount(tbAccounts, ma)
	}
}

func AddAccount(table string, ma *mgoAccount) {
	logPrintAll("==== addAccount() ====", "", "")
	if !Mongo {
		return
	}
	collectionTable := getCollection(table)
	if collectionTable == nil {
		log.Warn("getCollection()", "table", table, "collectionTable", "nil")
		return
	}
	err := collectionTable.Insert(ma)
	if err == nil {
		logPrintAll("==== AddAccount() ====", "Insert success: mgoAccount address", ma.Address)
	} else {
		logPrintAll("==== AddAccount() ====", "Insert failed: mgoAccount address", ma.Address)
	}
}

func AddDcrmAddress(address, cointype, dcrmAddress string, lockin, lockout int) {
	if !Mongo {
		return
	}
	logPrintAll("==== AddDcrmAddress() ====", "address", address, "cointype", cointype, "dcrmAddress", dcrmAddress)
	mda := new(mgoDcrmAccount)
	mda.Address = address
	mda.SortId = coinmap[cointype]
	if strings.HasPrefix(cointype, "ERC20") {
		mda.IsERC20 = 1
		mda.CoinType = cointype[5:]
	} else {
		mda.CoinType = cointype
	}
	mda.IsLockin = lockin
	mda.IsLockout = lockout
	mda.DcrmAddress = dcrmAddress
	go AddDcrmAccount(tbDcrmAccounts, mda)
}

func AddDcrmAccount(table string, mda *mgoDcrmAccount) {
	collectionTable := getCollection(table)
	if collectionTable == nil {
		log.Warn("getCollection()", "table", table, "collectionTable", "nil")
		return
	}
	cointype := recoverERC20Type(mda.CoinType, mda.IsERC20)
	mda.Key = (crypto.Keccak256Hash([]byte(mda.Address), []byte(cointype))).String()
	logPrintAll("==== AddDcrmAccount() ====", "mda", mda)
	err := collectionTable.Insert(mda)
	if err == nil {
		logPrintAll("==== AddDcrmAccount() ====", "Insert success: mgoDcrmAccount Address", mda.Address)
	} else {
		logPrintAll("==== AddDcrmAccount() ====", "Insert failed: mgoDcrmAccount Address", mda.Address, "key", mda.Key, "err", err)
	}
}

func DeleteDcrmAccount(table, address, cointype string) {
	logPrintAll("==== DeleteDcrmAccount() ====", "Delete Address", address)
	collectionTable := getCollection(table)
	if collectionTable == nil {
		log.Warn("getCollection()", "table", table, "collectionTable", "nil")
		return
	}
	Key := (crypto.Keccak256Hash([]byte(address), []byte(cointype))).String()
	err := collectionTable.Remove(bson.M{"_id": Key})
	if err == nil {
		logPrintAll("success", "Delete address", address, "key", Key)
	}
}

//func DeleteDexBlock(table string, Number uint64, Trade string) {
func DeleteDexBlock(table string, Number uint64) {
	DexBlockLock.Lock()
	defer DexBlockLock.Unlock()
	collectionTable := getCollection(table)
	if collectionTable == nil {
		log.Warn("getCollection()", "table", table, "collectionTable", "nil")
		return
	}
	//err := collectionTable.Remove(bson.M{"number": Number, "trade": Trade})
	err := collectionTable.Remove(bson.M{"number": Number})
	if err == nil {
		logPrintAll("success", "Delete DexBlock Number", Number)
	}
}

func FindDcrmAccount(table, initAddress, cointype string) []mgoDcrmAccount {
	if collectionDcrmAccount == nil {
		collectionDcrmAccount = database.C(table)
	}
	if len(initAddress) == 0 {
		account := make([]mgoDcrmAccount, 20)
		collectionDcrmAccount.Find(nil).All(&account)
		logPrintAll("==== FindDcrmAccount() ====", "find", account)
		return account
	} else {
		logPrintAll("==== FindDcrmAccount() ====", "Find initAddress", initAddress)
		account := make([]mgoDcrmAccount, 1)
		Key := (crypto.Keccak256Hash([]byte(initAddress), []byte(cointype))).String()
		err := collectionDcrmAccount.Find(bson.M{"_id": Key}).One(&account[0])
		logPrintAll("", "account", account[0])
		if err == nil {
			logPrintAll("", "Found initAddress", initAddress)
			return account
		}
		logPrintAll("", "UnFound initAddress", initAddress)
	}
	return nil
}

func FindAccount(table, address string) []mgoAccount {
	if collectionAccount == nil {
		collectionAccount = database.C(table)
	}
	if len(address) == 0 {
		account := make([]mgoAccount, 20)
		collectionAccount.Find(nil).All(&account)
		return account
	} else {
		account := make([]mgoAccount, 1)
		err := collectionAccount.Find(bson.M{"address": address}).One(&account[0])
		if err == nil {
			logPrintAll("==== FindAccount() ====", "Found address", address)
			return account
		}
	}
	return nil
}

func IsSyncedBlock(blockHash string) bool {
	bsi, _ := blockSyncInfo.Load(blockHash)
	if bsi == EXIST {
		return true
	}
	return false
}

func FindTransaction(table, Hash string) []mgoTransaction {
	collectionTable := getCollection(table)
	if collectionTable == nil {
		log.Warn("getCollection()", "table", table, "collectionTable", "nil")
		return nil
	}
	d := make([]mgoTransaction, 1)
	err := collectionTable.Find(bson.M{"hash": Hash}).One(&d[0])
	if err == nil {
		logPrintAll("==== FindTransaction() ===", "Found hash", Hash)
		return d
	}
	return nil
}

func FindDexTx(table, Hash string, Height uint64) []mgoDexTx {
	collectionTable := getCollection(table)
	if collectionTable == nil {
		log.Warn("getCollection()", "table", table, "collectionTable", "nil")
		return nil
	}
	d := make([]mgoDexTx, 1)
	height := make([]byte, 20)
	binary.BigEndian.PutUint64(height, Height)
	Key := (crypto.Keccak256Hash([]byte(Hash), []byte(height))).String()
	err := collectionTable.Find(bson.M{"_id": Key}).One(&d[0])
	if err == nil {
		logPrintAll("==== FindDexTx() ===", "Found hash", Hash)
		return d
	}
	return nil
}

func FindDexBlock(table string, Number uint64, Trade string) []mgoDexBlock {
	collectionTable := getCollection(table)
	if collectionTable == nil {
		log.Warn("getCollection()", "table", table, "collectionTable", "nil")
		return nil
	}
	b := make([]mgoDexBlock, 1)
	err := collectionTable.Find(bson.M{"number": Number, "trade": Trade}).One(&b[0])
	if err == nil {
		logPrintAll("==== FindDexBlock() ===", "Found number", Number, "Trade", Trade)
		return b
	}
	return nil
}

func FindBlock(table string, num uint64) []mgoBlock {
	if collectionBlock == nil {
		collectionBlock = database.C(table)
	}
	if num < 0 {
		blocks := make([]mgoBlock, 20)
		collectionBlock.Find(nil).All(&blocks)
		return blocks
	} else {
		b := make([]mgoBlock, 1)
		err := collectionBlock.Find(bson.M{"number": num}).One(&b[0])
		if err == nil {
			logPrintAll("==== FindBlock() ===", "Found number", num)
			return b
		}
		logPrintAll("==== FindBlock() ====", "UnFound number", num)
	}
	return nil
}

func isExistOrderCache(hash string) bool {
	cache := FindOrderCache(tbOrderCaches, hash)
	if cache != nil {
		return true
	}
	return false
}

func isExistOrder(hash string) bool {
	order := FindOrder(tbOrders, hash)
	if order != nil {
		return true
	}
	return false
}

func FindOrder(table string, hash string) []mgoOrder {
	collectionTable := getCollection(table)
	if collectionTable == nil {
		log.Warn("getCollection()", "table", table, "collectionTable", "nil")
		return nil
	}
	if hash == "" {
		return nil
	}
	c := make([]mgoOrder, 1)
	err := collectionTable.Find(bson.M{"hash": hash}).One(&c[0])
	if err == nil {
		return c
	}
	return nil
}

func FindOrderCache(table string, hash string) []mgoOrderCache {
	collectionTable := getCollection(table)
	if collectionTable == nil {
		log.Warn("getCollection()", "table", table, "collectionTable", "nil")
		return nil
	}
	if hash == "" {
		return nil
	}
	c := make([]mgoOrderCache, 1)
	err := collectionTable.Find(bson.M{"hash": hash}).One(&c[0])
	if err == nil {
		return c
	}
	return nil
}

func getTotalQuantity(Hash string) float64 {
	o := FindOrder(tbOrders, Hash)
	if o != nil {
		return o[0].Quantity
	}
	return 0.0
}

func getCompleted(Hash string) float64 {
	o := FindOrder(tbOrders, Hash)
	if o != nil {
		return o[0].Completed
	}
	return 0.0
}

func removeOrderCache(table, Key string) {
	//OrderCacheLock.Lock()
	//defer OrderCacheLock.Unlock()
	collectionTable := getCollection(table)
	if collectionTable == nil {
		log.Warn("getCollection()", "table", table, "collectionTable", "nil")
		return
	}
	collectionTable.Remove(bson.M{"_id": Key})
}

/*
func UpdateOrderCache(table string, hash string, quantity, price float64, v *orderbook.XvcOrder) {
	logPrintAll("==== UpdateOrderCache() ====", "hash", hash, "remain", quantity)

	orderCacheTimer := time.NewTimer(time.Duration(1) * time.Second)
	defer orderCacheTimer.Stop()
	oci, _ := orderCacheInfo.Load(hash)
	logPrintAll("==== UpdateOrderCache() ====", "hash", hash, "exist(1)/finish(2)", oci)
	count := 1
	maxwait := 5 * 100
	for {
		oci, _ := orderCacheInfo.Load(hash)
		if oci == EXIST {
			break
		} else if oci == FINISHED {
			collectionTable := getCollection(table)
			OrderCacheLock.Lock()
			collectionTable.Remove(bson.M{"_id": hash})
			OrderCacheLock.Unlock()
			logPrintAll("==== UpdateOrderCache() ====", "remove: hash", hash, "remain", quantity)
			return
		}
		count++
		if count > maxwait {
			logPrintAll("=== UpdateOrderCache() ====", "error not found orderCache hash", hash)
			break
		}
		<-orderCacheTimer.C
		orderCacheTimer.Reset(time.Duration(1) * time.Second)
	}
	collectionTable := getCollection(table)
	if quantity < (1 / DIVE8) {
		UpdateOrderStatus(hash, SUCCESS)
		OrderCacheLock.Lock()
		collectionTable.Remove(bson.M{"_id": hash})
		OrderCacheLock.Unlock()
		logPrintAll("==== UpdateOrderCache() ====", "remove: hash", hash, "remain", quantity)
		orderCacheInfo.Store(hash, FINISHED)
	} else {
		total := price * quantity
		OrderCacheLock.Lock()
		collectionTable.Update(bson.M{"_id": hash}, bson.M{"$set": bson.M{
			"volume": quantity,
			"total":  total,
		}})
		OrderCacheLock.Unlock()
		logPrintAll("==== UpdateOrderCache() ====", "update: hash", hash, "remain", quantity)
	}
}
*/
func UpdateOrderStatus(hash string, status int) {
	if !Mongo {
		return
	}
	logPrintAll("==== UpdateOrderStatus() ====", "hash", hash, "status", status)
	table := tbOrders
	OrderLock.Lock()
	defer OrderLock.Unlock()
	collectionTable := getCollection(table)
	if collectionTable == nil {
		log.Warn("getCollection()", "table", table, "collectionTable", "nil")
		return
	}
	collectionTable.Update(bson.M{"_id": hash}, bson.M{"$set": bson.M{
		"status": status,
	}})
}

func UpdateOrder(hash string, remain string) {
	if !Mongo {
		return
	}
	logPrintAll("==== UpdateOrder() ====", "hash", hash, "remain", remain)
	//OrderLock.Lock()
	//defer OrderLock.Unlock()
	table := tbOrders
	collectionTable := getCollection(table)
	if collectionTable == nil {
		log.Warn("getCollection()", "table", table, "collectionTable", "nil")
		return
	}
	oci, _ := orderInfo.Load(hash)
	if oci == nil {
		return
	}
	r, _ := strconv.ParseFloat(remain, 64)
	r /= MUL1E10
	complete := oci.(float64) - r
	logPrintAll("==== UpdateOrder() ====", "hash", hash, "remain", remain, "org", oci, "complete", complete)
	collectionTable.Update(bson.M{"_id": hash}, bson.M{"$set": bson.M{
		"completed": complete,
	}})

	if r < (1 / DIVE8) {
		UpdateOrderStatus(hash, SUCCESS)
	}
}

func UpdateDcrmBalance(table, address, cointype, valueString string, value float64) {
	logPrintAll("==== UpdateDcrmBalance() ====", "Address", address, "value", value)
	collectionTable := getCollection(table)
	if collectionTable == nil {
		log.Warn("getCollection()", "table", table, "collectionTable", "nil")
		return
	}
	Key := (crypto.Keccak256Hash([]byte(address), []byte(cointype))).String()
	collectionTable.Update(bson.M{"_id": Key}, bson.M{"$set": bson.M{
		"balanceString": valueString,
		"balance":       value,
	}})
	return
}

func UpdateAccountPublicKey(table, address, pubkey string) {
	logPrintAll("==== UpdateAccountPublicKey() ====", "Address", address, "pubkey", pubkey)
	collectionTable := getCollection(table)
	if collectionTable == nil {
		log.Warn("getCollection()", "table", table, "collectionTable", "nil")
		return
	}
	collectionTable.Update(bson.M{"address": address}, bson.M{"$set": bson.M{
		"publicKey": pubkey,
	}})
	return
}

func UpdateBalance(table, address, valueString string, value float64) {
	logPrintAll("==== UpdateBalance() ====", "Address", address, "valueString", valueString, "value", value)
	collectionTable := getCollection(table)
	if collectionTable == nil {
		log.Warn("getCollection()", "table", table, "collectionTable", "nil")
		return
	}
	collectionTable.Update(bson.M{"address": address}, bson.M{"$set": bson.M{
		"balanceString": valueString,
		"balance":       value,
	}})
	return
}

func UpdateComfirm(table, address string, value float64) {
	logPrintAll("==== UpdateComfirm() ====", "address", address, "value", value)
	if collectionAccount == nil {
		collectionAccount = database.C(table)
	}
	collectionAccount.Update(bson.M{"address": address}, bson.M{"$set": bson.M{
		"isConfirm": value,
	}})
	return
}

func DeleteBlock(table string, num uint64) {
	if collectionBlock == nil {
		collectionBlock = database.C(table)
	}

	if num < 0 {
		fs := FindBlock(table, 0)
		for _, b := range fs {
			logPrintAll("==== DeleteBlock() ====", "Remove: num", b.Number)
			collectionBlock.Remove(bson.M{"number": b.Number})
		}
	} else {
		err := collectionBlock.Remove(bson.M{"number": num})
		if err != nil {
			logPrintAll("==== DeleteBlock() ====", "delete failed", err.Error())
		} else {
			logPrintAll("==== DeleteBlock() ====", "delete success", "")
		}
	}
}

func ClearDb(table string) {
	logPrintAll("==== ClearDb() ====", "Clean table", table)
	DeleteBlock(table, 0)
}

func UnCompress(s string) (string, error) {

	if s == "" {
		return "", errors.New("args is nil")
	}

	var data bytes.Buffer
	data.Write([]byte(s))

	r, err := zlib.NewReader(&data)
	if err != nil {
		return "", err
	}

	var out bytes.Buffer
	io.Copy(&out, r)
	return out.String(), nil
}

func DecodeMatchRes(s string) (*MatchRes, error) {
	var data bytes.Buffer
	data.Write([]byte(s))

	dec := gob.NewDecoder(&data)

	var res MatchRes
	err := dec.Decode(&res)
	if err != nil {
		panic(err)
	}

	return &res, nil
}

type CurrentVolume struct {
	Id  string
	Vol string
}

type MatchRes struct {
	Price     string
	Done      []*XvcOrder
	Volumes   string
	Orders    int
	CurVolume []*CurrentVolume
}

type ODBInfo struct {
	Trade     string //ETH/BTC
	Price     string
	Volumes   string
	Orders    int
}

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

func Decode(s string) (interface{}, error) {
	var m SendMsg
	err := json.Unmarshal([]byte(s), &m)
	if err != nil {
		return nil, err
	}

	return &m, nil
}

type SendMsg struct {
	MsgType string
	Nonce   string
	WorkId  int
	Msg     string
}

type txOutput struct {
	ToAddress string      `json:"ToAddress"`
	Amount    interface{} `json:"Amount"`
}
type txResult struct {
	From    string     `json:"FromAddress"`
	Error   string     `json:Error`
	Outputs []txOutput `json:"TxOutputs"`
}
type txJson struct {
	Code   string   `json:"code"`
	Result txResult `json:"result"`
}

func getAddressByTxHash(hash, cointype string) (string, string) {
	retFrom := ""
	retTo := ""
	//http://5.189.139.168:23333/gettransaction?txhash=0xe34d08e9c6c929e8642a30a9404fea9035f78e1cc574cb216ad1674e1e4fbfc8&cointype=ERC20BNB
	url := fmt.Sprintf("%s?txhash=%s&cointype=%s", "http://5.189.139.168:23333/gettransaction", hash, cointype)
	logPrintAll("==== getFromByTxHash ====", "url", url)
	//fmt.Printf("==== getFromByTxHash ====, url = %+v\n", url)
	resp, _ := http.Get(url)
	if resp != nil {
		body, _ := ioutil.ReadAll(resp.Body)
		if body != nil {
			retString := string(body)
			logPrintAll("==== getFromByTxHash ====", "retString", retString)
			//fmt.Printf("==== getFromByTxHash ====, retString = %+v\n", retString)
			var tj txJson
			errj := json.Unmarshal([]byte(retString), &tj)
			if errj == nil {
                               if tj.Result.Error != "" {
					logPrintAll("==== getFromByTxHash ====", "code", tj.Code, "Error", tj.Result.Error)
					//fmt.Printf("==== getFromByTxHash ====, code: %v, error:  %v\n", tj.Code, tj.Result.Error)
                                       return retFrom, retTo
                               }
                               if tj.Result.From != "" && tj.Result.Outputs[0].ToAddress != "" {
                                       retFrom = tj.Result.From
                                       retTo = tj.Result.Outputs[0].ToAddress
					logPrintAll("==== getFromByTxHash ====", "code", tj.Code, "from", retFrom, "to", retTo)
					//fmt.Printf("==== getFromByTxHash ====, code: %+v, from: %v, to: %v\n", tj.Code, retFrom, retTo)
                               }
			} else {
				fmt.Printf("mongodb ==== getFromByTxHash ====, url: %v, err: %+v\n", url, errj)
			}
		}
		resp.Body.Close()
	}
	return retFrom, retTo
}

func logPrintAll(msg string, ctx ...interface{}) {
	logHead := fmt.Sprintf("mongodb %v", msg)
	if LOGPRINTALL {
		log.Info(logHead, ctx...)
	} else {
		log.Debug(logHead, ctx...)
	}
}

