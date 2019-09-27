// Copyright 2016 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package core

import (
	"container/heap"
	"math"
	"math/big"
	"sort"
	"strings"
	"errors" 
	"github.com/fusion/go-fusion/common"
	"github.com/fusion/go-fusion/core/types"
	"github.com/fusion/go-fusion/log"
	"github.com/fusion/go-fusion/crypto/dcrm" 
	"github.com/fusion/go-fusion/crypto/dcrm/cryptocoins"
	"github.com/fusion/go-fusion/crypto"
	"strconv"
)

var (
    dcrmlockoutcallback   func(interface{}) (string,error)
    removeobtxcb   func(uint64,uint64,string) (bool,error)
    gettrade func(string) string
)

func callDcrmLockOut(do types.DcrmLockOutData) (string,error) {
     if dcrmlockoutcallback == nil {
	 return "",nil
     }

    return dcrmlockoutcallback(do)
}

func RegisterDcrmLockOutCallback(recvDcrmFunc func(interface{}) (string,error)) {
	dcrmlockoutcallback = recvDcrmFunc
}

func RegisterRemoveOBTxcb(recvObFunc func(uint64,uint64,string) (bool,error)) {
	removeobtxcb = recvObFunc
}

func RegisterGetTradecb(recvObFunc func(string) string) {
	gettrade = recvObFunc
}

// nonceHeap is a heap.Interface implementation over 64bit unsigned integers for
// retrieving sorted transactions from the possibly gapped future queue.
type nonceHeap []uint64

func (h nonceHeap) Len() int           { return len(h) }
func (h nonceHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h nonceHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *nonceHeap) Push(x interface{}) {
	*h = append(*h, x.(uint64))
}

func (h *nonceHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// txSortedMap is a nonce->transaction hash map with a heap based index to allow
// iterating over the contents in a nonce-incrementing way.
type txSortedMap struct {
	items map[uint64]*types.Transaction // Hash map storing the transaction data
	index *nonceHeap                    // Heap of nonces of all the stored transactions (non-strict mode)
	cache types.Transactions            // Cache of the transactions already sorted
}

// newTxSortedMap creates a new nonce-sorted transaction map.
func newTxSortedMap() *txSortedMap {
	return &txSortedMap{
		items: make(map[uint64]*types.Transaction),
		index: new(nonceHeap),
	}
}

// Get retrieves the current transactions associated with the given nonce.
func (m *txSortedMap) Get(nonce uint64) *types.Transaction {
	return m.items[nonce]
}

// Put inserts a new transaction into the map, also updating the map's nonce
// index. If a transaction already exists with the same nonce, it's overwritten.
func (m *txSortedMap) Put(tx *types.Transaction) {
	nonce := tx.Nonce()
	if m.items[nonce] == nil {
		heap.Push(m.index, nonce)
	}
	m.items[nonce], m.cache = tx, nil
}

// Forward removes all transactions from the map with a nonce lower than the
// provided threshold. Every removed transaction is returned for any post-removal
// maintenance.
func (m *txSortedMap) Forward(threshold uint64) types.Transactions {
	var removed types.Transactions

	//log.Debug("============txSortedMap.Forward,","threshold",threshold,"m.index len",m.index.Len(),"m.index[0]",(*m.index)[0],"","===========")
	// Pop off heap items until the threshold is reached
	for m.index.Len() > 0 && (*m.index)[0] < threshold {
		nonce := heap.Pop(m.index).(uint64)
		//log.Debug("============txSortedMap.Forward,","removed nonce",nonce,"","===========")
		removed = append(removed, m.items[nonce])
		delete(m.items, nonce)
	}
	// If we had a cached order, shift the front
	if m.cache != nil {
		m.cache = m.cache[len(removed):]
	}
	return removed
}

// Filter iterates over the list of transactions and removes all of them for which
// the specified function evaluates to true.
func (m *txSortedMap) Filter(pool *TxPool,filter func(*TxPool,*types.Transaction) bool) types.Transactions {
	var removed types.Transactions

	//log.Debug("==============txSortedMap.Filter==================")
	// Collect all the transactions to filter out
	for nonce, tx := range m.items {
	    //log.Debug("==========txSortedMap.Filter,","nonce",nonce,"tx hash",tx.Hash(),"tx data",string(tx.Data()),"","===========")
		if types.IsXvcTx(tx) && FilterXvcTx(pool,tx,0) {
			removed = append(removed, tx)
			delete(m.items, nonce)
		} else if types.IsDcrmLockIn(tx) && FilterDcrmTx(pool,tx) {
			removed = append(removed, tx)
			delete(m.items, nonce)
		} else if !types.IsDcrmLockIn(tx) && !types.IsDcrmConfirmAddr(tx) && !types.IsXvcTx(tx) && filter(pool,tx) {
			removed = append(removed, tx)
			delete(m.items, nonce)
		}
	}
	// If transactions were removed, the heap and cache are ruined
	if len(removed) > 0 {
		*m.index = make([]uint64, 0, len(m.items))
		for nonce := range m.items {
			*m.index = append(*m.index, nonce)
		}
		heap.Init(m.index)

		m.cache = nil
	}
	return removed
}

// Cap places a hard limit on the number of items, returning all transactions
// exceeding that limit.
func (m *txSortedMap) Cap(threshold int) types.Transactions {
	// Short circuit if the number of items is under the limit
	if len(m.items) <= threshold {
		return nil
	}
	// Otherwise gather and drop the highest nonce'd transactions
	var drops types.Transactions

	sort.Sort(*m.index)
	for size := len(m.items); size > threshold; size-- {
		drops = append(drops, m.items[(*m.index)[size-1]])
		delete(m.items, (*m.index)[size-1])
	}
	*m.index = (*m.index)[:threshold]
	heap.Init(m.index)

	// If we had a cache, shift the back
	if m.cache != nil {
		m.cache = m.cache[:len(m.cache)-len(drops)]
	}
	return drops
}

// Remove deletes a transaction from the maintained map, returning whether the
// transaction was found.
func (m *txSortedMap) Remove(nonce uint64) bool {
	// Short circuit if no transaction is present
	_, ok := m.items[nonce]
	if !ok {
		return false
	}
	// Otherwise delete the transaction and fix the heap index
	for i := 0; i < m.index.Len(); i++ {
		if (*m.index)[i] == nonce {
			heap.Remove(m.index, i)
			break
		}
	}
	delete(m.items, nonce)
	m.cache = nil

	return true
}

// Ready retrieves a sequentially increasing list of transactions starting at the
// provided nonce that is ready for processing. The returned transactions will be
// removed from the list.
//
// Note, all transactions with nonces lower than start will also be returned to
// prevent getting into and invalid state. This is not something that should ever
// happen but better to be self correcting than failing!
func (m *txSortedMap) Ready(pool *TxPool,start uint64) types.Transactions { 
	// Short circuit if no transactions are available
	if m.index.Len() == 0 || (*m.index)[0] > start {
		return nil
	}
	// Otherwise start accumulating incremental transactions
	var ready types.Transactions

	for next := (*m.index)[0]; m.index.Len() > 0 && (*m.index)[0] == next; next++ {

	    tx := m.items[next]
	    //input := tx.Data()
	    //data := string(input)
	    //realtxdata,_ := types.GetRealTxData(data)
	    //mm := strings.Split(realtxdata,":")
	    val,ok := types.GetDcrmValidateDataKReady(tx.Hash().Hex())
	    _, err := types.Sender(pool.signer,tx)
	    if err != nil {
		if types.IsDcrmLockOut(tx) {
			ready = append(ready, m.items[next])
		    	m.Remove(next)
			//bug
			if ok == true && val != "" {
			    types.DeleteDcrmValidateData(tx.Hash().Hex())
			}
		}
		    continue
	    }

	    /*if types.IsDcrmLockIn(tx) {
		_,err := pool.ValidateLockin(tx)
		if err == nil {
		    log.Debug("=============txPool.Ready,lockin success.==============","lockin tx hash",tx.Hash(),"lockin tx data",string(tx.Data()))
		    ready = append(ready, m.items[next])
		    m.Remove(next)
		} else if strings.EqualFold(err.Error(),"confirming") {
		    log.Debug("=============txPool.Ready,lockin is pending.==============","lockin tx hash",tx.Hash(),"lockin tx data",string(tx.Data()))
		    //.....
		} else {
		    log.Debug("=============txPool.Ready,lockin fail and remove the tx.==============","err",err,"lockin tx hash",tx.Hash(),"lockin tx data",string(tx.Data()))
		    m.Remove(next)
		    pool.removeTx(tx.Hash(),true)
		}

	    } else if types.IsDcrmLockOut(tx) {
		if ok == true && val != "" {
		    _,err := pool.ValidateLockin2(tx,val)
		    if err == nil {
			log.Debug("=============txPool.Ready,lockout success.==============","lockout tx hash",tx.Hash(),"lockout tx data",string(tx.Data()))
			ready = append(ready, m.items[next])
			m.Remove(next)
			//bug
			types.DeleteDcrmValidateData(tx.Hash().Hex())
		    } else if strings.EqualFold(err.Error(),"confirming") {
			log.Debug("=============txPool.Ready,lockout is pending.==============","lockout tx hash",tx.Hash(),"lockout tx data",string(tx.Data()))
			//.....
		    } else {
			//log.Debug("=============txPool.Ready,lockout error,but success.==============","lockout tx hash",tx.Hash(),"lockout tx data",string(tx.Data()))
			//ready = append(ready, m.items[next])   ////TODO  ?????????????????????
		    	//m.Remove(next)
			//bug
			//types.DeleteDcrmValidateData(tx.Hash().Hex())
			
			log.Debug("=============txPool.Ready,lockout error.==============","err",err,"lockout tx hash",tx.Hash(),"lockout tx data",string(tx.Data()))
		    	m.Remove(next)
			pool.removeTx(tx.Hash(),true)
			//bug
			types.DeleteDcrmValidateData(tx.Hash().Hex())
		    }
		} else { //bug:if no val and tx is invalide
			ready = append(ready, m.items[next])
		    	m.Remove(next)
		}

	    } else {
		ready = append(ready, m.items[next])
		m.Remove(next)
	    }*/

	    if types.IsDcrmLockIn(tx) {
		_,err := pool.ValidateLockin(tx)
		if err == nil {
		    log.Debug("=============txPool.Ready,lockin success.==============","lockin tx hash",tx.Hash(),"lockin tx data",string(tx.Data()))
		    ready = append(ready, m.items[next])
		    m.Remove(next)
		} else if strings.EqualFold(err.Error(),"confirming") {
		    //log.Debug("=============txPool.Ready,lockin is pending.==============","lockin tx hash",tx.Hash(),"lockin tx data",string(tx.Data()))
		    //.....
		} else {
		    log.Debug("=============txPool.Ready,lockin fail and remove the tx.==============","err",err,"lockin tx hash",tx.Hash(),"lockin tx data",string(tx.Data()))
		    m.Remove(next)
		    pool.removeTx(tx.Hash(),true)
		}

	    } else { //bug:if no val and tx is invalide
		ready = append(ready, m.items[next])
		m.Remove(next)
	    }
	}
	m.cache = nil

	return ready
}

// Len returns the length of the transaction map.
func (m *txSortedMap) Len() int {
	return len(m.items)
}

// Flatten creates a nonce-sorted slice of transactions based on the loosely
// sorted internal representation. The result of the sorting is cached in case
// it's requested again before any modifications are made to the contents.
func (m *txSortedMap) Flatten() types.Transactions {
	// If the sorting was not cached yet, create and cache it
	if m.cache == nil {
		m.cache = make(types.Transactions, 0, len(m.items))
		for _, tx := range m.items {
			m.cache = append(m.cache, tx)
		}
		sort.Sort(types.TxByNonce(m.cache))
	}
	// Copy the cache to prevent accidental modifications
	txs := make(types.Transactions, len(m.cache))
	copy(txs, m.cache)
	return txs
}

// txList is a "list" of transactions belonging to an account, sorted by account
// nonce. The same type can be used both for storing contiguous transactions for
// the executable/pending queue; and for storing gapped transactions for the non-
// executable/future queue, with minor behavioral changes.
type txList struct {
	strict bool         // Whether nonces are strictly continuous or not
	txs    *txSortedMap // Heap indexed sorted hash map of the transactions

	costcap *big.Int // Price of the highest costing transaction (reset only if exceeds balance)
	gascap  uint64   // Gas limit of the highest spending transaction (reset only if exceeds block limit)
}

// newTxList create a new transaction list for maintaining nonce-indexable fast,
// gapped, sortable transaction lists.
func newTxList(strict bool) *txList {
	return &txList{
		strict:  strict,
		txs:     newTxSortedMap(),
		costcap: new(big.Int),
	}
}

// Overlaps returns whether the transaction specified has the same nonce as one
// already contained within the list.
func (l *txList) Overlaps(tx *types.Transaction) bool {
	return l.txs.Get(tx.Nonce()) != nil
}

func IsDcrmTx(tx *types.Transaction) bool {
    return types.IsDcrmLockOut(tx) || types.IsDcrmLockIn(tx) || types.IsDcrmTransaction(tx) || types.IsDcrmConfirmAddr(tx)
}

func GetDcrmTxCointype(tx *types.Transaction) (string,error) {
    if tx == nil {
	return "",errors.New("tx is not the dcrm tx.")
    }
    
    realtxdata,_ := types.GetRealTxData(string(tx.Data()))
    m := strings.Split(realtxdata,":")
    if len(m) > 1 {
	return m[0],nil
    }

    return "",errors.New("tx is not the dcrm tx.")
}

// Add tries to insert a new transaction into the list, returning whether the
// transaction was accepted, and if yes, any previous transaction it replaced.
//
// If the new transaction is accepted into the list, the lists' cost and gas
// thresholds are also potentially updated.
func (l *txList) Add(tx *types.Transaction, priceBump uint64) (bool, *types.Transaction) {
        //log.Debug("==========txList.Add==============","tx hash",tx.Hash(),"tx data",string(tx.Data()))
	// If there's an older better transaction, abort
	old := l.txs.Get(tx.Nonce())
	if old != nil { 
	    //log.Debug("==============txList.Add,","old nonce",old.Nonce(),"","=========","tx hash",tx.Hash(),"tx data",string(tx.Data()))
	}
	//log.Debug("==============txList.Add,","tx nonce",tx.Nonce(),"","=========","tx hash",tx.Hash(),"tx data",string(tx.Data()))
	sametype := false
	txtype,e1 := GetDcrmTxCointype(tx)
	oldtype,e2 := GetDcrmTxCointype(tx)
	if old != nil && IsDcrmTx(tx) && IsDcrmTx(old) && (e1 == nil && e2 == nil && txtype == oldtype ) {
	    //log.Debug("==============txList.Add,old and new is dcrmtx=========","tx hash",tx.Hash(),"tx data",string(tx.Data()))
	    sametype = true
	} else if old != nil && !IsDcrmTx(old) && !IsDcrmTx(tx) {
	    //log.Debug("==============txList.Add,old and new is not dcrmtx=========","tx hash",tx.Hash(),"tx data",string(tx.Data()))
	    sametype = true
	}

	if sametype == true {
		//log.Debug("==========txList.Add,old != nil==============","tx hash",tx.Hash(),"tx data",string(tx.Data()))
		threshold := new(big.Int).Div(new(big.Int).Mul(old.GasPrice(), big.NewInt(100+int64(priceBump))), big.NewInt(100))
		// Have to ensure that the new gas price is higher than the old gas
		// price as well as checking the percentage threshold to ensure that
		// this is accurate for low (Wei-level) gas price replacements
		if old.GasPrice().Cmp(tx.GasPrice()) >= 0 || threshold.Cmp(tx.GasPrice()) > 0 {
			//log.Debug("==========txList.Add,old.GasPrice().Cmp(tx.GasPrice()) >= 0==============","tx hash",tx.Hash(),"tx data",string(tx.Data()))
			return false, nil
		}
	}
	// Otherwise overwrite the old transaction with the current one
	l.txs.Put(tx)
	if cost := tx.Cost(); l.costcap.Cmp(cost) < 0 {
		//log.Debug("==========txList.Add,l.costcap.Cmp(cost) < 0==============","tx hash",tx.Hash(),"tx data",string(tx.Data()))
		l.costcap = cost
	}
	if gas := tx.Gas(); l.gascap < gas {
		//log.Debug("==========txList.Add,l.gascap < gas==============","tx hash",tx.Hash(),"tx data",string(tx.Data()))
		l.gascap = gas
	}
	return true, old
}

// Forward removes all transactions from the list with a nonce lower than the
// provided threshold. Every removed transaction is returned for any post-removal
// maintenance.
func (l *txList) Forward(threshold uint64) types.Transactions {
	return l.txs.Forward(threshold)
}

func FilterXvcTx(pool *TxPool,tx *types.Transaction,bn uint64) bool {
    //log.Debug("===========FilterXvcTx===============")
    if tx == nil || pool == nil {
	return false
    }

    from, err := types.Sender(pool.signer,tx)
    if err != nil {
	log.Debug("===========FilterXvcTx============","from",from,"err",err,"tx hash",tx.Hash(),"tx data",string(tx.Data()))
	return false
    }

    log.Debug("===========FilterXvcTx,","tx hash",tx.Hash(),"tx data",string(tx.Data()),"","================")

    if types.IsXvcTx(tx) {
	data,_ := types.GetRealTxData(string(tx.Data()))
	m := strings.Split(data,common.SepOB)
	trade := gettrade(data)
	hash := crypto.Keccak256Hash([]byte(m[1]+":"+strings.ToLower(trade))).Hex()
	 ret := pool.currentState.GetMatchNonce(from)
	log.Debug("===========FilterXvcTx,","hash",hash,"ret",ret,"tx hash",tx.Hash(),"tx data",string(tx.Data()),"","================")
	 if ret != "" && strings.EqualFold(ret,hash) {
	    log.Debug("===========FilterXvcTx,return true.","hash",hash,"ret",ret,"tx hash",tx.Hash(),"tx data",string(tx.Data()),"","================")
	     return true
	 }
    }

    if types.IsXvcTx(tx) && removeobtxcb != nil {
	data,_ := types.GetRealTxData(string(tx.Data()))
	m := strings.Split(data,common.SepOB)
	h,_ := strconv.Atoi(m[1])

	trade := gettrade(data)
	b,err := removeobtxcb(uint64(h),uint64(bn),trade)
	if err!= nil {
	    return false
	}

	log.Debug("===========FilterXvcTx=========","call removeobtxcb return value",b,"tx hash",tx.Hash(),"tx data",string(tx.Data()))
	return b
    }

    return false
}

func FilterDcrmTx(pool *TxPool,tx *types.Transaction) bool {
    if tx == nil || pool == nil {
	return false
    }

    from, err := types.Sender(pool.signer,tx)
    if err != nil {
	return false
    }

    //log.Debug("===========FilterDcrmTx,","tx hash",tx.Hash(),"tx data",string(tx.Data()),"from",from,"","================")
    realtxdata,_ := types.GetRealTxData(string(tx.Data()))
    m := strings.Split(realtxdata,":")
    
    if types.IsDcrmTransaction(tx) {
	value := m[2]
	cointype := m[3]

	if strings.EqualFold(cointype,"ETH") == true || strings.EqualFold(cointype,"ERC20GUSD") == true || strings.EqualFold(cointype,"ERC20BNB") == true || strings.EqualFold(cointype,"ERC20MKR") == true || strings.EqualFold(cointype,"ERC20HT") == true || strings.EqualFold(cointype,"ERC20BNT") == true {
	    amount,_ := new(big.Int).SetString(value,10)
	     ret := pool.currentState.GetBalance(from,cointype)
	    //log.Debug("===========FilterDcrmTx,","amount",amount,"ret",ret,"","================","tx hash",tx.Hash(),"tx data",string(tx.Data()))
	     if ret  != nil {
		if ret.Cmp(amount) < 0 {
		    //log.Debug("=============FilterDcrmTx,TRANSACTION,ErrInsufficientBalance===============","tx hash",tx.Hash(),"tx data",string(tx.Data()))
		    return true
		}
	     }
	} else if strings.EqualFold(cointype,"BTC") == true {
	    amount,_ := new(big.Int).SetString(value,10)
	    one,_ := new(big.Int).SetString("1",10)
	    if amount.Cmp(one) >= 0 {
		 ret := pool.currentState.GetBalance(from,cointype)
		//log.Debug("===========FilterDcrmTx,","amount",amount,"ret",ret,"","================","tx hash",tx.Hash(),"tx data",string(tx.Data()))
		if ret != nil {
		    if ret.Cmp(amount) < 0 {
			//log.Debug("=============FilterDcrmTx,TRANSACTION,ErrInsufficientBalance===============","tx hash",tx.Hash(),"tx data",string(tx.Data()))
			return true
		    }
		}
	    }
	} else {
	    amount,_ := new(big.Int).SetString(value,10)
	    ret := pool.currentState.GetBalance(from,cointype)
	    if ret != nil {
		if ret.Cmp(amount) < 0 {
		    //log.Debug("=============FilterDcrmTx,TRANSACTION,ErrInsufficientBalance===============","tx hash",tx.Hash(),"tx data",string(tx.Data()))
		    return true 
		}
	    }
	}
	///
    }

    if types.IsDcrmLockOut(tx) {
	value := m[2]
	cointype := m[3]
	
	amount,_ := new(big.Int).SetString(value,10)

	if chandler := cryptocoins.NewCryptocoinHandler(cointype); chandler.IsToken() == false {
	    //============case1: coin=========
	    ret := pool.currentState.GetBalance(from,cointype)
	    if ret != nil {
		 fee := chandler.GetDefaultFee()
		 total := new(big.Int).Add(amount,fee.Val)
		 //log.Debug("==========FilterDcrmTx=================","from",from,"cointype",cointype,"get dcrm addr balance",ret,"lockout value",amount,"fee",fee,"amount+fee",total,"tx hash",tx.Hash(),"tx data",string(tx.Data()))
		if ret.Cmp(total) < 0 {
		    log.Debug("==========FilterDcrmTx=================","err",dcrm.GetRetErr(dcrm.ErrInsufficientDcrmFunds),"tx hash",tx.Hash(),"tx data",string(tx.Data()))
		    //pool.removeTx(tx.Hash(),true)
		    return true
		}
	    } 
     } else {
	 //============case2: token========
	 fee:= chandler.GetDefaultFee()
	 ret1 := pool.currentState.GetBalance(from,cointype)
	 ret2 := pool.currentState.GetBalance(from,fee.Cointype)
	 if ret1 != nil && ret2 != nil {
	     //log.Debug("========FilterDcrmTx, token balance and coin balance========", "token balance", ret1, "coin balance", ret2,"tx hash",tx.Hash(),"tx data",string(tx.Data()))
	     if ret1.Cmp(amount) < 0 {
		     // token余额不足
		     log.Debug("==========FilterDcrmTx,token balance insufficient=================","err",dcrm.GetRetErr(dcrm.ErrInsufficientDcrmFunds),"tx hash",tx.Hash(),"tx data",string(tx.Data()))
		    //pool.removeTx(tx.Hash(),true)
		     return true
	     }
	     if ret2.Cmp(fee.Val) < 0 {
		     // fee不足
		     log.Debug("==========FilterDcrmTx,insufficient fund to pay fee=================","err",dcrm.GetRetErr(dcrm.ErrInsufficientDcrmFunds),"tx hash",tx.Hash(),"tx data",string(tx.Data()))
		    //pool.removeTx(tx.Hash(),true)
		     return true
	     }
	 }
     }

	/*if strings.EqualFold(cointype,"ETH") == true || strings.EqualFold(cointype,"ERC20GUSD") == true || strings.EqualFold(cointype,"ERC20BNB") == true || strings.EqualFold(cointype,"ERC20MKR") == true || strings.EqualFold(cointype,"ERC20HT") == true || strings.EqualFold(cointype,"ERC20BNT") == true {
	    amount,_ := new(big.Int).SetString(value,10)
	     ret := pool.GetCurrentState().GetBalance(from,cointype)
	    log.Debug("===========FilterDcrmTx,","amount",amount,"ret",ret,"","================","tx hash",tx.Hash(),"tx data",string(tx.Data()))
	     if ret != nil {
		if ret.Cmp(amount) < 0 {
		    log.Debug("=============FilterDcrmTx,LOCKOUT,ErrInsufficientBalance===============","tx hash",tx.Hash(),"tx data",string(tx.Data()))
		    return true
		}
	     }
	}
	///
	if strings.EqualFold(cointype,"BTC") == true {
	    amount,_ := new(big.Int).SetString(value,10)
	    one,_ := new(big.Int).SetString("1",10)
	    if amount.Cmp(one) >= 0 {
		 ret := pool.GetCurrentState().GetBalance(from,cointype)
		log.Debug("===========FilterDcrmTx,","amount",amount,"ret",ret,"","================","tx hash",tx.Hash(),"tx data",string(tx.Data()))
		if ret != nil {
		    if ret.Cmp(amount) < 0 {
			log.Debug("=============FilterDcrmTx,LOCKOUT,ErrInsufficientBalance===============","tx hash",tx.Hash(),"tx data",string(tx.Data()))
			return true
		    }
		}
	    }
	}*/
	///
    }
    
    if types.IsDcrmLockIn(tx) {
	 ret := pool.currentState.GetAccountHashkey(from,m[3])
	 if ret != "" && strings.EqualFold(ret,m[1]) {
	     return true
	 }
    }

    return false
}

// Filter removes all transactions from the list with a cost or gas limit higher
// than the provided thresholds. Every removed transaction is returned for any
// post-removal maintenance. Strict-mode invalidated transactions are also
// returned.
//
// This method uses the cached costcap and gascap to quickly decide if there's even
// a point in calculating all the costs or if the balance covers all. If the threshold
// is lower than the costgas cap, the caps will be reset to a new high after removing
// the newly invalidated transactions.
func (l *txList) Filter(pool *TxPool,costLimit *big.Int, gasLimit uint64) (types.Transactions, types.Transactions) {

	//cc := fmt.Sprintf("%v",l.costcap)
	//cl := fmt.Sprintf("%v",costLimit)
	//log.Debug("==========txList.Filter,","costLimit",cl,"gasLimit",gasLimit,"l.costcap.",cc,"","==============")

	// If all transactions are below the threshold, short circuit
	/*if l.costcap.Cmp(costLimit) <= 0 && l.gascap <= gasLimit {
		log.Debug("==========txList.Filter,l.costcap.Cmp(costLimit) <= 0==============")
		return nil, nil
	    }*/////----------  bug:if run dcrmSendTransaction fast and continue,tx pool will not filter the dcrm tx

	l.costcap = new(big.Int).Set(costLimit) // Lower the caps to the thresholds
	l.gascap = gasLimit

	// Filter out all the transactions above the account's funds
	removed := l.txs.Filter(pool,func(pool * TxPool,tx *types.Transaction) bool {
	    //log.Debug("==========txList.Filter,l.txs.Filter==============")
	    return tx.Cost().Cmp(costLimit) > 0 || tx.Gas() > gasLimit || FilterDcrmTx(pool,tx) || FilterXvcTx(pool,tx,0)
	})

	// If the list was strict, filter anything above the lowest nonce
	var invalids types.Transactions

	if l.strict && len(removed) > 0 {
		lowest := uint64(math.MaxUint64)
		for _, tx := range removed {
			if nonce := tx.Nonce(); lowest > nonce {
				lowest = nonce
			}
		}
		
		invalids = l.txs.Filter(pool,func(pool *TxPool,tx *types.Transaction) bool {
		    //log.Debug("==========txList.Filter,invalids = l.txs.Filter==============")
		    return tx.Nonce() > lowest })
	}
	return removed, invalids
}

// Cap places a hard limit on the number of items, returning all transactions
// exceeding that limit.
func (l *txList) Cap(threshold int) types.Transactions {
	return l.txs.Cap(threshold)
}

// Remove deletes a transaction from the maintained list, returning whether the
// transaction was found, and also returning any transaction invalidated due to
// the deletion (strict mode only).
func (l *txList) Remove(pool *TxPool,tx *types.Transaction) (bool, types.Transactions) { 
	//log.Debug("==========txList.Remove tx==============","tx hash",tx.Hash(),"tx data",string(tx.Data()))
	// Remove the transaction from the set
	nonce := tx.Nonce()
	if removed := l.txs.Remove(nonce); !removed {
		//log.Debug("==========txList.Remove,!removed==============","tx hash",tx.Hash(),"tx data",string(tx.Data()))
		return false, nil
	}
	// In strict mode, filter out non-executable transactions
	if l.strict {
		return true, l.txs.Filter(pool,func(pool *TxPool,tx *types.Transaction) bool {
		    //log.Debug("==========txList.Remove,l.txs.Filter==============","tx hash",tx.Hash(),"tx data",string(tx.Data()))
		    return tx.Nonce() > nonce })
	}
	return true, nil
}

// Ready retrieves a sequentially increasing list of transactions starting at the
// provided nonce that is ready for processing. The returned transactions will be
// removed from the list.
//
// Note, all transactions with nonces lower than start will also be returned to
// prevent getting into and invalid state. This is not something that should ever
// happen but better to be self correcting than failing!
func (l *txList) Ready(pool *TxPool,start uint64) types.Transactions {
	return l.txs.Ready(pool,start)
}

// Len returns the length of the transaction list.
func (l *txList) Len() int {
	return l.txs.Len()
}

// Empty returns whether the list of transactions is empty or not.
func (l *txList) Empty() bool {
	return l.Len() == 0
}

// Flatten creates a nonce-sorted slice of transactions based on the loosely
// sorted internal representation. The result of the sorting is cached in case
// it's requested again before any modifications are made to the contents.
func (l *txList) Flatten() types.Transactions {
	return l.txs.Flatten()
}

// priceHeap is a heap.Interface implementation over transactions for retrieving
// price-sorted transactions to discard when the pool fills up.
type priceHeap []*types.Transaction

func (h priceHeap) Len() int      { return len(h) }
func (h priceHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h priceHeap) Less(i, j int) bool {
	// Sort primarily by price, returning the cheaper one
	switch h[i].GasPrice().Cmp(h[j].GasPrice()) {
	case -1:
		return true
	case 1:
		return false
	}
	// If the prices match, stabilize via nonces (high nonce is worse)
	return h[i].Nonce() > h[j].Nonce()
}

func (h *priceHeap) Push(x interface{}) {
	*h = append(*h, x.(*types.Transaction))
}

func (h *priceHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// txPricedList is a price-sorted heap to allow operating on transactions pool
// contents in a price-incrementing way.
type txPricedList struct {
	all    *txLookup  // Pointer to the map of all transactions
	items  *priceHeap // Heap of prices of all the stored transactions
	stales int        // Number of stale price points to (re-heap trigger)
}

// newTxPricedList creates a new price-sorted transaction heap.
func newTxPricedList(all *txLookup) *txPricedList {
	return &txPricedList{
		all:   all,
		items: new(priceHeap),
	}
}

// Put inserts a new transaction into the heap.
func (l *txPricedList) Put(tx *types.Transaction) {
	heap.Push(l.items, tx)
}

// Removed notifies the prices transaction list that an old transaction dropped
// from the pool. The list will just keep a counter of stale objects and update
// the heap if a large enough ratio of transactions go stale.
func (l *txPricedList) Removed() {
	// Bump the stale counter, but exit if still too low (< 25%)
	l.stales++
	if l.stales <= len(*l.items)/4 {
		return
	}
	// Seems we've reached a critical number of stale transactions, reheap
	reheap := make(priceHeap, 0, l.all.Count())

	l.stales, l.items = 0, &reheap
	l.all.Range(func(hash common.Hash, tx *types.Transaction) bool {
		*l.items = append(*l.items, tx)
		return true
	})
	heap.Init(l.items)
}

// Cap finds all the transactions below the given price threshold, drops them
// from the priced list and returns them for further removal from the entire pool.
func (l *txPricedList) Cap(threshold *big.Int, local *accountSet) types.Transactions {
	drop := make(types.Transactions, 0, 128) // Remote underpriced transactions to drop
	save := make(types.Transactions, 0, 64)  // Local underpriced transactions to keep

	for len(*l.items) > 0 {
		// Discard stale transactions if found during cleanup
		tx := heap.Pop(l.items).(*types.Transaction)
		if l.all.Get(tx.Hash()) == nil {
			l.stales--
			continue
		}
		// Stop the discards if we've reached the threshold
		if tx.GasPrice().Cmp(threshold) >= 0 {
			save = append(save, tx)
			break
		}
		// Non stale transaction found, discard unless local
		if local.containsTx(tx) {
			save = append(save, tx)
		} else {
			drop = append(drop, tx)
		}
	}
	for _, tx := range save {
		heap.Push(l.items, tx)
	}
	return drop
}

// Underpriced checks whether a transaction is cheaper than (or as cheap as) the
// lowest priced transaction currently being tracked.
func (l *txPricedList) Underpriced(tx *types.Transaction, local *accountSet) bool {
	// Local transactions cannot be underpriced
	if local.containsTx(tx) {
		return false
	}
	// Discard stale price points if found at the heap start
	for len(*l.items) > 0 {
		head := []*types.Transaction(*l.items)[0]
		if l.all.Get(head.Hash()) == nil {
			l.stales--
			heap.Pop(l.items)
			continue
		}
		break
	}
	// Check if the transaction is underpriced or not
	if len(*l.items) == 0 {
		log.Error("Pricing query for empty pool") // This cannot happen, print to catch programming errors
		return false
	}
	cheapest := []*types.Transaction(*l.items)[0]
	return cheapest.GasPrice().Cmp(tx.GasPrice()) >= 0
}

// Discard finds a number of most underpriced transactions, removes them from the
// priced list and returns them for further removal from the entire pool.
func (l *txPricedList) Discard(count int, local *accountSet) types.Transactions {
	drop := make(types.Transactions, 0, count) // Remote underpriced transactions to drop
	save := make(types.Transactions, 0, 64)    // Local underpriced transactions to keep

	for len(*l.items) > 0 && count > 0 {
		// Discard stale transactions if found during cleanup
		tx := heap.Pop(l.items).(*types.Transaction)
		if l.all.Get(tx.Hash()) == nil {
			l.stales--
			continue
		}
		// Non stale transaction found, discard unless local
		if local.containsTx(tx) {
			save = append(save, tx)
		} else {
			drop = append(drop, tx)
			count--
		}
	}
	for _, tx := range save {
		heap.Push(l.items, tx)
	}
	return drop
}

func ExsitOrderTxInPending(pool *TxPool,trade string) bool {
    if pool == nil || trade == "" {
	return false
    }

    pending, err := pool.Pending()
    if err != nil {
	    log.Error("Failed to fetch pending transactions", "err", err)
	    return false
    }

    if len(pending) == 0 {
	log.Debug("============ExsitOrderTxInPending,pending len is 0.===============")
	    return false
    }

    as := pool.Locals()
    for _, account := range as {
	    if txs := pending[account]; len(txs) > 0 {
		if ExsitOrderTx(txs,trade) {
		    return true
		}
	    }
    }

    return false
}

func ExsitOrderTx(txs types.Transactions,trade string) bool {
    if trade == "" {
	return false
    }

    for _, tx := range txs {
	s := string(tx.Data())
	if s == "" {
	    continue
	}

	trade2 := gettrade(s)
	if types.IsXvcTx(tx) && strings.EqualFold(trade,trade2) {
	    log.Debug("========core.ExsitOrderTx,exsit order tx in txpool.=========","tx hash",tx.Hash().Hex(),"tx",tx,"tx.data",string(tx.Data()))
	    return true 
	}
    }

    return false
}

