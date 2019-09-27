// Copyright 2014 The go-ethereum Authors
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

// Package state provides a caching layer atop the Ethereum state trie.
package state

import (
	"fmt"
	"math/big"
	"sort"
	"sync"

	//"errors"
	"github.com/fusion/go-fusion/common"
	"github.com/fusion/go-fusion/core/types"
	"github.com/fusion/go-fusion/crypto"
	"github.com/fusion/go-fusion/log"
	"github.com/fusion/go-fusion/rlp"
	"github.com/fusion/go-fusion/trie"
)

type revision struct {
	id           int
	journalIndex int
}

var (
	// emptyState is the known hash of an empty state trie entry.
	emptyState = crypto.Keccak256Hash(nil)

	// emptyCode is the known hash of the empty EVM bytecode.
	emptyCode = crypto.Keccak256Hash(nil)
)

//////////////////////////
type SafeMapStateObjects struct {
    sync.RWMutex
    Map map[common.Address]*stateObject
}

func NewSafeMapStateObjects(size int) *SafeMapStateObjects {
    sm := new(SafeMapStateObjects)
    if size <= 0 {
	sm.Map = make(map[common.Address]*stateObject)
    } else {
	sm.Map = make(map[common.Address]*stateObject,size)
    }
    return sm
}

func (sm *SafeMapStateObjects) ReadMap(key common.Address) (*stateObject,bool) {
    sm.RLock()
    value,ok := sm.Map[key]
    sm.RUnlock()
    return value,ok
}

func (sm *SafeMapStateObjects) WriteMap(key common.Address, value *stateObject) {
    sm.Lock()
    sm.Map[key] = value
    sm.Unlock()
}

func (sm *SafeMapStateObjects) DeleteMap(key common.Address) {
    sm.Lock()
    delete(sm.Map,key)
    sm.Unlock()
}

func (sm *SafeMapStateObjects) ListMap() ([]common.Address,[]*stateObject) {
    sm.RLock()
    key := make([]common.Address,len(sm.Map))
    value := make([]*stateObject,len(sm.Map))
    i := 0
    for k,v := range sm.Map {
	key[i] = k
	value[i] = v
	i++
    }
    sm.RUnlock()
    return key,value
}

func (sm *SafeMapStateObjects) MapLength() int {
    sm.RLock()
    l := len(sm.Map)
    sm.RUnlock()
    return l
}

///
type SafeMapStateObjectsDirty struct {
    sync.RWMutex
    Map map[common.Address]struct{}
}

func NewSafeMapStateObjectsDirty(size int) *SafeMapStateObjectsDirty {
    sm := new(SafeMapStateObjectsDirty)
    if size <= 0 {
	sm.Map = make(map[common.Address]struct{})
    } else {
	sm.Map = make(map[common.Address]struct{},size)
    }
    return sm
}

func (sm *SafeMapStateObjectsDirty) ReadMap(key common.Address) (struct{},bool) {
    sm.RLock()
    value,ok := sm.Map[key]
    sm.RUnlock()
    return value,ok
}

func (sm *SafeMapStateObjectsDirty) WriteMap(key common.Address, value struct{}) {
    sm.Lock()
    sm.Map[key] = value
    sm.Unlock()
}

func (sm *SafeMapStateObjectsDirty) DeleteMap(key common.Address) {
    sm.Lock()
    delete(sm.Map,key)
    sm.Unlock()
}

func (sm *SafeMapStateObjectsDirty) ListMap() ([]common.Address,[]struct{}) {
    sm.RLock()
    key := make([]common.Address,len(sm.Map))
    value := make([]struct{},len(sm.Map))
    i := 0
    for k,v := range sm.Map {
	key[i] = k
	value[i] = v
	i++
    }
    sm.RUnlock()
    return key,value
}

func (sm *SafeMapStateObjectsDirty) MapLength() int {
    sm.RLock()
    l := len(sm.Map)
    sm.RUnlock()
    return l
}
///////////////////////////

// StateDBs within the ethereum protocol are used to store anything
// within the merkle trie. StateDBs take care of caching and storing
// nested states. It's the general query interface to retrieve:
// * Contracts
// * Accounts
type StateDB struct {
	db   Database
	trie Trie

	// This map holds 'live' objects, which will get modified while processing a state transition.
	//stateObjects      map[common.Address]*stateObject
	stateObjects      *SafeMapStateObjects
	//stateObjectsDirty map[common.Address]struct{}
	stateObjectsDirty *SafeMapStateObjectsDirty

	// DB error.
	// State objects are used by the consensus core and VM which are
	// unable to deal with database-level errors. Any error that occurs
	// during a database read is memoized here and will eventually be returned
	// by StateDB.Commit.
	dbErr error

	// The refund counter, also used by state transitioning.
	refund uint64

	thash, bhash common.Hash
	txIndex      int
	logs         map[common.Hash][]*types.Log
	logSize      uint

	preimages map[common.Hash][]byte

	// Journal of state modifications. This is the backbone of
	// Snapshot and RevertToSnapshot.
	journal        *journal
	validRevisions []revision
	nextRevisionId int

	lock sync.Mutex

}

// Create a new state from a given trie.
func New(root common.Hash, db Database) (*StateDB, error) {
	tr, err := db.OpenTrie(root)
	if err != nil {
		return nil, err
	}
	return &StateDB{
		db:                db,
		trie:              tr,
		//stateObjects:      make(map[common.Address]*stateObject),
		stateObjects:      NewSafeMapStateObjects(0),
		//stateObjectsDirty: make(map[common.Address]struct{}),
		stateObjectsDirty: NewSafeMapStateObjectsDirty(0),
		logs:              make(map[common.Hash][]*types.Log),
		preimages:         make(map[common.Hash][]byte),
		journal:           newJournal(),
	}, nil
}

// setError remembers the first non-nil error it is called with.
func (self *StateDB) setError(err error) {
	if self.dbErr == nil {
		self.dbErr = err
	}
}

func (self *StateDB) Error() error {
	return self.dbErr
}

// Reset clears out all ephemeral state objects from the state db, but keeps
// the underlying state trie to avoid reloading data for the next operations.
func (self *StateDB) Reset(root common.Hash) error {
	tr, err := self.db.OpenTrie(root)
	if err != nil {
		return err
	}
	self.trie = tr
	//self.stateObjects = make(map[common.Address]*stateObject)
	self.stateObjects = NewSafeMapStateObjects(0)
	//self.stateObjectsDirty = make(map[common.Address]struct{})
	self.stateObjectsDirty = NewSafeMapStateObjectsDirty(0)
	self.thash = common.Hash{}
	self.bhash = common.Hash{}
	self.txIndex = 0
	self.logs = make(map[common.Hash][]*types.Log)
	self.logSize = 0
	self.preimages = make(map[common.Hash][]byte)
	self.clearJournalAndRefund()
	return nil
}

func (self *StateDB) AddLog(log *types.Log) {
	self.journal.append(addLogChange{txhash: self.thash})

	log.TxHash = self.thash
	log.BlockHash = self.bhash
	log.TxIndex = uint(self.txIndex)
	log.Index = self.logSize
	self.logs[self.thash] = append(self.logs[self.thash], log)
	self.logSize++
}

func (self *StateDB) GetLogs(hash common.Hash) []*types.Log {
	return self.logs[hash]
}

func (self *StateDB) Logs() []*types.Log {
	var logs []*types.Log
	for _, lgs := range self.logs {
		logs = append(logs, lgs...)
	}
	return logs
}

// AddPreimage records a SHA3 preimage seen by the VM.
func (self *StateDB) AddPreimage(hash common.Hash, preimage []byte) {
	if _, ok := self.preimages[hash]; !ok {
		self.journal.append(addPreimageChange{hash: hash})
		pi := make([]byte, len(preimage))
		copy(pi, preimage)
		self.preimages[hash] = pi
	}
}

// Preimages returns a list of SHA3 preimages that have been submitted.
func (self *StateDB) Preimages() map[common.Hash][]byte {
	return self.preimages
}

// AddRefund adds gas to the refund counter
func (self *StateDB) AddRefund(gas uint64) {
	self.journal.append(refundChange{prev: self.refund})
	self.refund += gas
}

// SubRefund removes gas from the refund counter.
// This method will panic if the refund counter goes below zero
func (self *StateDB) SubRefund(gas uint64) {
	self.journal.append(refundChange{prev: self.refund})
	if gas > self.refund {
		panic("Refund counter below zero")
	}
	self.refund -= gas
}

// Exist reports whether the given account address exists in the state.
// Notably this also returns true for suicided accounts.
func (self *StateDB) Exist(addr common.Address) bool {
	return self.getStateObject(addr) != nil
}

// Empty returns whether the state object is either non-existent
// or empty according to the EIP161 specification (balance = nonce = code = 0)
func (self *StateDB) Empty(addr common.Address) bool {
	so := self.getStateObject(addr)
	return so == nil || so.empty()
}

// Retrieve the balance from the given address or 0 if object not found
func (self *StateDB) GetBalance(addr common.Address,cointype string) *big.Int {
	stateObject := self.getStateObject(addr)
	if stateObject != nil {
	    return stateObject.Balance(cointype)
	}
	return common.Big0
}

func (self *StateDB) SetBalance(addr common.Address, amount *big.Int,cointype string) {
	stateObject := self.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SetBalance(amount,cointype)
	}
}

func (self *StateDB) GetDcrmAddr(addr common.Address,cointype string) string {
	stateObject := self.getStateObject(addr)
	if stateObject != nil {
		return stateObject.DcrmAddr(cointype)
	}

	return "" 
}

func (self *StateDB) SetAccountDcrmAddr(a common.Address,dcrmaddr string,cointype string) {
	stateObject := self.getStateObject(a)
	if stateObject != nil {
		stateObject.SetDcrmAddr(dcrmaddr,cointype)
	}
}

func (self *StateDB) GetAccountHashkey(addr common.Address,cointype string) string {
	stateObject := self.getStateObject(addr)
	if stateObject != nil {
		return stateObject.Hashkey(cointype)
	}

	return "" 
}

func (self *StateDB) SetAccountHashkey(a common.Address,hashkey string,cointype string) {
	stateObject := self.getStateObject(a)
	if stateObject != nil {
		stateObject.SetHashkey(hashkey,cointype)
	}
}

func (self *StateDB) GetOutSideBalance(addr common.Address,cointype string) *big.Int {
	stateObject := self.getStateObject(addr)
	if stateObject != nil {
		return stateObject.OutSideBalance(cointype)
	}
	return common.Big0
}

func (self *StateDB) SetOutSideBalance(addr common.Address, amount *big.Int,cointype string) {
	stateObject := self.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SetOutSideBalance(amount,cointype)
	}
}

func (self *StateDB) AddOutSideBalance(addr common.Address, amount *big.Int,cointype string) {
	stateObject := self.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.AddOutSideBalance(amount,cointype)
	}
}

// SubOutSideBalance subtracts amount from the account associated with addr.
func (self *StateDB) SubOutSideBalance(addr common.Address, amount *big.Int,cointype string) {
	stateObject := self.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SubOutSideBalance(amount,cointype)
	}
}

func (self *StateDB) GetNonce(addr common.Address) uint64 {
	stateObject := self.getStateObject(addr)
	if stateObject != nil {
		log.Debug("==========statedb.GetNonce==========","nonce",stateObject.Nonce(),"addr",addr.Hex())
		return stateObject.Nonce()
	}

	return 0
}

func (self *StateDB) GetMatchNonce(addr common.Address) string {
	stateObject := self.getStateObject(addr)
	if stateObject != nil {
		log.Debug("==========statedb.GetMatchNonce==========","match nonce",stateObject.MatchNonce(),"addr",addr.Hex())
		return stateObject.MatchNonce()
	}

	return ""
}

func (self *StateDB) AddAccountDcrmAddr(a common.Address,dcrmaddr string,cointype string) {
	stateObject := self.getStateObject(a)
	if stateObject != nil {
		stateObject.AddDcrmAddr(dcrmaddr,cointype)
	}
}

func (self *StateDB) GetCode(addr common.Address) []byte {
	stateObject := self.getStateObject(addr)
	if stateObject != nil {
		return stateObject.Code(self.db)
	}
	return nil
}

func (self *StateDB) GetCodeSize(addr common.Address) int {
	stateObject := self.getStateObject(addr)
	if stateObject == nil {
		return 0
	}
	if stateObject.code != nil {
		return len(stateObject.code)
	}
	size, err := self.db.ContractCodeSize(stateObject.addrHash, common.BytesToHash(stateObject.CodeHash()))
	if err != nil {
		self.setError(err)
	}
	return size
}

func (self *StateDB) GetCodeHash(addr common.Address) common.Hash {
	stateObject := self.getStateObject(addr)
	if stateObject == nil {
		return common.Hash{}
	}
	return common.BytesToHash(stateObject.CodeHash())
}

// GetState retrieves a value from the given account's storage trie.
func (self *StateDB) GetState(addr common.Address, hash common.Hash) common.Hash {
	stateObject := self.getStateObject(addr)
	if stateObject != nil {
		return stateObject.GetState(self.db, hash)
	}
	return common.Hash{}
}

// GetCommittedState retrieves a value from the given account's committed storage trie.
func (self *StateDB) GetCommittedState(addr common.Address, hash common.Hash) common.Hash {
	stateObject := self.getStateObject(addr)
	if stateObject != nil {
		return stateObject.GetCommittedState(self.db, hash)
	}
	return common.Hash{}
}

// Database retrieves the low level database supporting the lower level trie ops.
func (self *StateDB) Database() Database {
	return self.db
}

func (self *StateDB) GetStateDcrmAccountData(a common.Address, b common.Hash) []byte {
	stateObject := self.getStateObject(a)
	if stateObject != nil {
		return stateObject.GetStateDcrmAccountData(self.db, b)
	}
	return nil
}

func (self *StateDB) SetStateDcrmAccountData(addr common.Address, key common.Hash, value []byte) {
	//log.Debug("===========SetStateDcrmAccountData","key",key,"","============")
	stateObject := self.GetOrNewStateObject(addr)
	if stateObject != nil {
		//log.Debug("==================SetStateDcrmAccountData,get stateObject is not nil.============")
		stateObject.SetStateDcrmAccountData(self.db, key, value)
	}
}

func (self *StateDB) GetCommittedStateDcrmAccountData(addr common.Address, hash common.Hash) []byte {
	stateObject := self.getStateObject(addr)
	if stateObject != nil {
		return stateObject.GetCommittedStateDcrmAccountData(self.db, hash)
	}
	return nil 
}

func (self *StateDB) IsExsitDcrmAddress(a common.Address, cointype string,dcrmaddr string) (bool,error) {
	stateObject := self.getStateObject(a)
	if stateObject != nil {
	    return stateObject.IsExsitDcrmAddress(self.db, cointype,dcrmaddr)
	}
	return false,nil
}

// StorageTrie returns the storage trie of an account.
// The return value is a copy and is nil for non-existent accounts.
func (self *StateDB) StorageTrie(addr common.Address) Trie {
	stateObject := self.getStateObject(addr)
	if stateObject == nil {
		return nil
	}
	cpy := stateObject.deepCopy(self)
	return cpy.updateTrie(self.db)
}

func (self *StateDB) HasSuicided(addr common.Address) bool {
	stateObject := self.getStateObject(addr)
	if stateObject != nil {
		return stateObject.suicided
	}
	return false
}

/*
 * SETTERS
 */

// AddBalance adds amount to the account associated with addr.
func (self *StateDB) AddBalance(addr common.Address, amount *big.Int,cointype string) {
	stateObject := self.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.AddBalance(amount,cointype)
	}
}

// SubBalance subtracts amount from the account associated with addr.
func (self *StateDB) SubBalance(addr common.Address, amount *big.Int,cointype string) {
	stateObject := self.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SubBalance(amount,cointype)
	}
}

func (self *StateDB) SetNonce(addr common.Address, nonce uint64) {
        //log.Debug("============statedb.SetNonce 11111111==========","nonce",nonce,"addr",addr.Hex())//++++++++++caihaijun+++++++++++
	stateObject := self.GetOrNewStateObject(addr)
	if stateObject != nil {
	//	log.Debug("============statedb.SetNonce 2222222==========","nonce",nonce,"addr",addr.Hex())//++++++++++caihaijun+++++++++++
		stateObject.SetNonce(nonce)
	}
}

func (self *StateDB) SetMatchNonce(addr common.Address, nonce string) {
	stateObject := self.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SetMatchNonce(nonce)
	}
}

func (self *StateDB) SetCode(addr common.Address, code []byte) {
	stateObject := self.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SetCode(crypto.Keccak256Hash(code), code)
	}
}

func (self *StateDB) SetState(addr common.Address, key, value common.Hash) {
	stateObject := self.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SetState(self.db, key, value)
	}
}

// Suicide marks the given account as suicided.
// This clears the account balance.
//
// The account's state object is still available until the state is committed,
// getStateObject will return a non-nil account after Suicide.
func (self *StateDB) Suicide(addr common.Address) bool {
	stateObject := self.getStateObject(addr)
	if stateObject == nil {
		return false
	}
	if len(stateObject.data.Balance) != 0 {
	    dest := make([]*DcrmBalance,len(stateObject.data.Balance))
	    for k,v := range stateObject.data.Balance {
		ba := new(big.Int).SetBytes(v.Balance.Bytes())
		outba := new(big.Int).SetBytes(v.OutSideBalance.Bytes())
		if IsNegative(ba) {
		    ba,_ = new(big.Int).SetString("0",10)
		    v.Balance,_ = new(big.Int).SetString("0",10)
		}
		if IsNegative(outba) {
		    outba,_ = new(big.Int).SetString("0",10)
		    v.OutSideBalance,_ = new(big.Int).SetString("0",10)
		}
		tmp := &DcrmBalance{Cointype:v.Cointype,DcrmAddr:v.DcrmAddr,Hashkey:v.Hashkey,Balance:ba,OutSideBalance:outba}
		dest[k] = tmp
	    }

	    self.journal.append(suicideChange{
		account:     &addr,
		prev:        stateObject.suicided,
		prevbalance: dest, 
	    })
	} else {
	    self.journal.append(suicideChange{
		account:     &addr,
		prev:        stateObject.suicided,
		prevbalance: make([]*DcrmBalance,len(stateObject.data.Balance)),
	    })
	}
	stateObject.markSuicided()
	for _,v := range stateObject.data.Balance {
	    v.Balance = new(big.Int)
	    v.OutSideBalance = new(big.Int)
	}

	return true
}

//
// Setting, updating & deleting state object methods.
//

// updateStateObject writes the given object to the trie.
func (self *StateDB) updateStateObject(stateObject *stateObject) {
	addr := stateObject.Address()
        //log.Debug("===============updateStateObject","stateobject addr",stateObject.Address().Hex(),"","================")
	data, err := rlp.EncodeToBytes(stateObject)
	if err != nil {
		panic(fmt.Errorf("can't encode object at %x: %v", addr[:], err))
	}
	self.setError(self.trie.TryUpdate(addr[:], data))
}

// deleteStateObject removes the given object from the state trie.
func (self *StateDB) deleteStateObject(stateObject *stateObject) {
        //log.Debug("===============deleteStateObject","stateobject addr",stateObject.Address().Hex(),"","================")
	stateObject.deleted = true
	addr := stateObject.Address()
	self.setError(self.trie.TryDelete(addr[:]))
}

// Retrieve a state object given by the address. Returns nil if not found.
func (self *StateDB) getStateObject(addr common.Address) (stateObject *stateObject) {
	// Prefer 'live' objects.
	//if obj := self.stateObjects[addr]; obj != nil {
	if self.stateObjects == nil {
	    return nil
	}
	objtmp,b := self.stateObjects.ReadMap(addr)
	if b == true && objtmp != nil {
        //log.Debug("===============statedb.getStateObject, get obj != nil,","addr",addr.Hex(),"","================")
		if objtmp.deleted {
		//log.Debug("===============statedb.getStateObject, get obj != nil, fail: obj's deleted.","addr",addr.Hex(),"","================")
			return nil
		}
		return objtmp
	}

        //log.Debug("===============statedb.getStateObject,do TryGet================")
	// Load the object from the database.
	enc, err := self.trie.TryGet(addr[:])
	if len(enc) == 0 {
		//log.Debug("===============statedb.getStateObject, len(enc) == 0","addr",addr.Hex(),"","================")
		self.setError(err)
		return nil
	}
	var data Account
	if err := rlp.DecodeBytes(enc, &data); err != nil {
		//log.Debug("===============statedb.getStateObject, Failed to decode state object","addr",addr.Hex(),"","================")
		log.Error("Failed to decode state object", "addr", addr, "err", err)
		return nil
	}
	// Insert into the live set.
	obj := newObject(self, addr, data)
	//log.Debug("===============statedb.getStateObject, get obj nil and new a stateobject","addr",addr.Hex(),"","================")
	self.setStateObject(obj)
	return obj
}

func (self *StateDB) setStateObject(object *stateObject) {
	//self.stateObjects[object.Address()] = object
	if self.stateObjects == nil {
	    return
	}
	self.stateObjects.WriteMap(object.Address(),object)
}

// Retrieve a state object or create a new state object if nil.
func (self *StateDB) GetOrNewStateObject(addr common.Address) *stateObject {
	stateObject := self.getStateObject(addr)
	if stateObject == nil || stateObject.deleted {
		//log.Debug("============StateDB.GetOrNewStateObject,stateObject == nil || stateObject.deleted================")
		stateObject, _ = self.createObject(addr)
	}
	return stateObject
}

// createObject creates a new state object. If there is an existing account with
// the given address, it is overwritten and returned as the second return value.
func (self *StateDB) createObject(addr common.Address) (newobj, prev *stateObject) {
	prev = self.getStateObject(addr)
	newobj = newObject(self, addr, Account{})
	//log.Debug("===============statedb.createObject","addr",addr.Hex(),"","================")
	newobj.setNonce(0) // sets the object to dirty
	if prev == nil {
		self.journal.append(createObjectChange{account: &addr})
	} else {
		self.journal.append(resetObjectChange{prev: prev})
	}
	self.setStateObject(newobj)
	return newobj, prev
}

// CreateAccount explicitly creates a state object. If a state object with the address
// already exists the balance is carried over to the new account.
//
// CreateAccount is called during the EVM CREATE operation. The situation might arise that
// a contract does the following:
//
//   1. sends funds to sha(account ++ (nonce + 1))
//   2. tx_create(sha(account ++ nonce)) (note that this gets the address of 1)
//
// Carrying over the balance ensures that Ether doesn't disappear.
func (self *StateDB) CreateAccount(addr common.Address) {
    	//log.Debug("=====================CreateAccount,addr is %v==============\n",addr.Hex())
	new2, prev := self.createObject(addr)
	if prev != nil {
		//new.setBalance(prev.data.Balance)
		if len(prev.data.Balance) != 0 {
		    new2.data.Balance = make([]*DcrmBalance,len(prev.data.Balance))
		    for k,v := range prev.data.Balance {
			ba := new(big.Int).SetBytes(v.Balance.Bytes())
			outba := new(big.Int).SetBytes(v.OutSideBalance.Bytes())
			if IsNegative(ba) {
			    ba,_ = new(big.Int).SetString("0",10)
			    v.Balance,_ = new(big.Int).SetString("0",10)
			}
			if IsNegative(outba) {
			    outba,_ = new(big.Int).SetString("0",10)
			    v.OutSideBalance,_ = new(big.Int).SetString("0",10)
			}
			tmp := &DcrmBalance{Cointype:v.Cointype,DcrmAddr:v.DcrmAddr,Hashkey:v.Hashkey,Balance:ba,OutSideBalance:outba}
			new2.data.Balance[k] = tmp
		    }
		} else {
		    new2.data.Balance = make([]*DcrmBalance,len(prev.data.Balance))
		}
	}
}

func (db *StateDB) ForEachStorage(addr common.Address, cb func(key, value common.Hash) bool) {
	so := db.getStateObject(addr)
	if so == nil {
		return
	}
	it := trie.NewIterator(so.getTrie(db.db).NodeIterator(nil))
	for it.Next() {
		key := common.BytesToHash(db.trie.GetKey(it.Key))
		if value, dirty := so.dirtyStorage[key]; dirty {
			cb(key, value)
			continue
		}
		cb(key, common.BytesToHash(it.Value))
	}
}

func IsNegative(ba *big.Int) bool {
    if ba == nil {
	return false
    }

    zero,_ := new(big.Int).SetString("0",10)
    if ba.Cmp(zero) < 0 {
	return true
    }

    return false
}

// Copy creates a deep, independent copy of the state.
// Snapshots of the copied state cannot be applied to the copy.
func (self *StateDB) Copy() *StateDB {
	self.lock.Lock()
	defer self.lock.Unlock()

	// Copy all the basic fields, initialize the memory ones
	state := &StateDB{
		db:                self.db,
		trie:              self.db.CopyTrie(self.trie),
		//stateObjects:      make(map[common.Address]*stateObject, len(self.journal.dirties)),
		stateObjects:      NewSafeMapStateObjects(len(self.journal.dirties)),
		//stateObjectsDirty: make(map[common.Address]struct{}, len(self.journal.dirties)),
		stateObjectsDirty:      NewSafeMapStateObjectsDirty(len(self.journal.dirties)),
		refund:            self.refund,
		logs:              make(map[common.Hash][]*types.Log, len(self.logs)),
		logSize:           self.logSize,
		preimages:         make(map[common.Hash][]byte),
		journal:           newJournal(),
	}
	// Copy the dirty states, logs, and preimages
	for addr := range self.journal.dirties {
		// As documented [here](https://github.com/ethereum/go-ethereum/pull/16485#issuecomment-380438527),
		// and in the Finalise-method, there is a case where an object is in the journal but not
		// in the stateObjects: OOG after touch on ripeMD prior to Byzantium. Thus, we need to check for
		// nil
		//if object, exist := self.stateObjects[addr]; exist {
		if self.stateObjects == nil {
		    return nil
		}
		object,exist := self.stateObjects.ReadMap(addr)
		if exist == true && object != nil {
			//state.stateObjects[addr] = object.deepCopy(state)
			state.stateObjects.WriteMap(addr,object.deepCopy(state))
			state.stateObjectsDirty.WriteMap(addr,struct{}{})
			//state.stateObjectsDirty[addr] = struct{}{}
		}
	}
	// Above, we don't copy the actual journal. This means that if the copy is copied, the
	// loop above will be a no-op, since the copy's journal is empty.
	// Thus, here we iterate over stateObjects, to enable copies of copies
	if self.stateObjectsDirty == nil {
	    return nil
	}
	keys,_ := self.stateObjectsDirty.ListMap()
	l := len(keys)
	ii := 0
	for ii = 0;ii < l;ii++ {
	    addr := keys[ii]
	    //if _, exist := state.stateObjects[addr]; !exist {
	    _,exist := state.stateObjects.ReadMap(addr)
	    if !exist {
		    //state.stateObjects[addr] = self.stateObjects[addr].deepCopy(state)
		    if self.stateObjects == nil {
			return nil
		    }
		    objtmp,_ := self.stateObjects.ReadMap(addr)
		    if objtmp == nil {
			return nil
		    }
		    state.stateObjects.WriteMap(addr,objtmp.deepCopy(state))
		    state.stateObjectsDirty.WriteMap(addr,struct{}{})
		    //state.stateObjectsDirty[addr] = struct{}{}
	    }
	}
	/*for addr := range self.stateObjectsDirty {
		//if _, exist := state.stateObjects[addr]; !exist {
		_,exist := state.stateObjects.ReadMap(addr)
		if !exist {
			//state.stateObjects[addr] = self.stateObjects[addr].deepCopy(state)
			if self.stateObjects == nil {
			    return nil
			}
			objtmp,_ := self.stateObjects.ReadMap(addr)
			state.stateObjects.WriteMap(addr,objtmp.deepCopy(state))
			state.stateObjectsDirty.WriteMap(addr,struct{}{})
			//state.stateObjectsDirty[addr] = struct{}{}
		}
	}*/
	for hash, logs := range self.logs {
		cpy := make([]*types.Log, len(logs))
		for i, l := range logs {
			cpy[i] = new(types.Log)
			*cpy[i] = *l
		}
		state.logs[hash] = cpy
	}
	for hash, preimage := range self.preimages {
		state.preimages[hash] = preimage
	}
	return state
}

// Snapshot returns an identifier for the current revision of the state.
func (self *StateDB) Snapshot() int {
	id := self.nextRevisionId
	self.nextRevisionId++
	self.validRevisions = append(self.validRevisions, revision{id, self.journal.length()})
	return id
}

// RevertToSnapshot reverts all state changes made since the given revision.
func (self *StateDB) RevertToSnapshot(revid int) {
	// Find the snapshot in the stack of valid snapshots.
	idx := sort.Search(len(self.validRevisions), func(i int) bool {
		return self.validRevisions[i].id >= revid
	})
	if idx == len(self.validRevisions) || self.validRevisions[idx].id != revid {
		panic(fmt.Errorf("revision id %v cannot be reverted", revid))
	}
	snapshot := self.validRevisions[idx].journalIndex

	// Replay the journal to undo changes and remove invalidated snapshots
	self.journal.revert(self, snapshot)
	self.validRevisions = self.validRevisions[:idx]
}

// GetRefund returns the current value of the refund counter.
func (self *StateDB) GetRefund() uint64 {
	return self.refund
}

// Finalise finalises the state by removing the self destructed objects
// and clears the journal as well as the refunds.
func (s *StateDB) Finalise(deleteEmptyObjects bool) {
	//log.Debug("============StateDB.Finalise=========\n")
	for addr := range s.journal.dirties {
	        if s.stateObjects == nil {
		    return
		}
		//stateObject, exist := s.stateObjects[addr]
		stateObject, exist := s.stateObjects.ReadMap(addr)
		//log.Debug("===============statedb.Finalise, get stateobject","exist",exist,"addr",addr.Hex(),"","================")
		if !exist {
			// ripeMD is 'touched' at block 1714175, in tx 0x1237f737031e40bcde4a8b7e717b2d15e3ecadfe49bb1bbc71ee9deb09c6fcf2
			// That tx goes out of gas, and although the notion of 'touched' does not exist there, the
			// touch-event will still be recorded in the journal. Since ripeMD is a special snowflake,
			// it will persist in the journal even though the journal is reverted. In this special circumstance,
			// it may exist in `s.journal.dirties` but not in `s.stateObjects`.
			// Thus, we can safely ignore it here
			continue
		}

		if stateObject.suicided || (deleteEmptyObjects && stateObject.empty()) {
		        if stateObject.suicided {
			    //log.Debug("=================Finalise,stateObject.suicided=========")
			}
			if deleteEmptyObjects && stateObject.empty() {
			    //log.Debug("=================Finalise,(deleteEmptyObjects && stateObject.empty())========")
			}
			//log.Debug("===============statedb.Finalise, deleteStateObject==============")
			s.deleteStateObject(stateObject)
		} else {
			//log.Debug("===============statedb.Finalise, updateRoot==============")
			stateObject.updateRoot(s.db)
			s.updateStateObject(stateObject)
		}
		//s.stateObjectsDirty[addr] = struct{}{}
		if s.stateObjectsDirty != nil {
		    s.stateObjectsDirty.WriteMap(addr,struct{}{})
		}
	}
	// Invalidate journal because reverting across transactions is not allowed.
	s.clearJournalAndRefund()
}

// IntermediateRoot computes the current root hash of the state trie.
// It is called in between transactions to get the root hash that
// goes into transaction receipts.
func (s *StateDB) IntermediateRoot(deleteEmptyObjects bool) common.Hash {
	//log.Debug("===============statedb.IntermediateRoot","deleteEmptyObjects",deleteEmptyObjects,"","================")
	s.Finalise(deleteEmptyObjects)
	hash := s.trie.Hash()
	log.Debug("===============statedb.IntermediateRoot","get trie hash",hash.Hex(),"","================")
	//return s.trie.Hash()
	return hash
}

// Prepare sets the current transaction hash and index and block hash which is
// used when the EVM emits new state logs.
func (self *StateDB) Prepare(thash, bhash common.Hash, ti int) {
	self.thash = thash
	self.bhash = bhash
	self.txIndex = ti
}

func (s *StateDB) clearJournalAndRefund() {
	s.journal = newJournal()
	s.validRevisions = s.validRevisions[:0]
	s.refund = 0
}

// Commit writes the state to the underlying in-memory trie database.
func (s *StateDB) Commit(deleteEmptyObjects bool) (root common.Hash, err error) {

	//log.Debug("===============StateDB.Commit=============")
	defer s.clearJournalAndRefund()

	for addr := range s.journal.dirties {
		//log.Debug("===============Commit,range s.journal.dirties,addr is %v=============\n",addr)
		//s.stateObjectsDirty[addr] = struct{}{}
		if s.stateObjectsDirty != nil {
		    s.stateObjectsDirty.WriteMap(addr,struct{}{})
		}
	}
	// Commit objects to the trie.
	if s.stateObjects == nil || s.stateObjectsDirty == nil {
	    return common.Hash{},nil
	}
	keys,values := s.stateObjects.ListMap()
	l := len(keys)
	i := 0
	for i = 0;i<l;i++ {
	    addr := keys[i]
	    stateObject := values[i]

	    //log.Debug("===============StateDB.Commit,get obj,","addr",addr.Hex(),"","================")
	    //_, isDirty := s.stateObjectsDirty[addr]
	    _, isDirty := s.stateObjectsDirty.ReadMap(addr)
	    //log.Debug("===============StateDB.Commit,get obj","isDirty",isDirty,"addr",addr.Hex(),"","=============")
	    switch {
	    case stateObject.suicided || (isDirty && deleteEmptyObjects && stateObject.empty()):
		    // If the object has been removed, don't bother syncing it
		    // and just mark it for deletion in the trie.
		    if stateObject.suicided {
			//log.Debug("=================Commit,stateObject.suicided=========\n")
		    }
		    if deleteEmptyObjects && stateObject.empty() {
			//log.Debug("=================Commit,(deleteEmptyObjects && stateObject.empty())========\n")
		    }
		    
		    //log.Debug("===============StateDB.Commit,deleteStateObject=================")
		    s.deleteStateObject(stateObject)
	    case isDirty:
		    // Write any contract code associated with the state object
		    //log.Debug("======Commit,isDirty is true========\n")
		    if stateObject.code != nil && stateObject.dirtyCode {
			    //log.Debug("======Commit,stateObject.code != nil && stateObject.dirtyCode========\n")
			    s.db.TrieDB().InsertBlob(common.BytesToHash(stateObject.CodeHash()), stateObject.code)
			    stateObject.dirtyCode = false
		    }

		    //log.Debug("===============StateDB.Commit,call CommitTrie=================")
		    // Write any storage changes in the state object to its storage trie.
		    if err := stateObject.CommitTrie(s.db); err != nil {
			    //log.Debug("===============StateDB.Commit,call CommitTrie fail.=============\n")
			    return common.Hash{}, err
		    }
		    // Update the object in the main account trie.
		    s.updateStateObject(stateObject)
	    }
	    //delete(s.stateObjectsDirty, addr)
	    if s.stateObjectsDirty != nil {
		s.stateObjectsDirty.DeleteMap(addr)
	    }
	}

	//log.Debug("===============StateDB.Commit,call trie.Commit=================")
	// Write trie changes.
	root, err = s.trie.Commit(func(leaf []byte, parent common.Hash) error {
		log.Debug("===============Commit,s.trie.Commit=============","err",err)
		var account Account
		if err := rlp.DecodeBytes(leaf, &account); err != nil {
			//log.Debug("===============Commit,s.trie.Commit fail. decodebytes fail.=============","err",err)
			return nil
		}
		if account.Root != emptyState {
			s.db.TrieDB().Reference(account.Root, parent)
		}
		code := common.BytesToHash(account.CodeHash)
		if code != emptyCode {
			s.db.TrieDB().Reference(code, parent)
		}
		return nil
	})
	log.Debug("Trie cache stats after commit", "misses", trie.CacheMisses(), "unloads", trie.CacheUnloads())
	return root, err
}
