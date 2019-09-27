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

package state

import (
	"bytes"
	"fmt"
	"io"
	"math/big"
	"strings"
	"errors"
	"encoding/json"
	"github.com/fusion/go-fusion/core/types"
	"github.com/fusion/go-fusion/common"
	"github.com/fusion/go-fusion/crypto"
	"github.com/fusion/go-fusion/rlp"
	"github.com/fusion/go-fusion/log"
)

var emptyCodeHash = crypto.Keccak256(nil)

type Code []byte

func (self Code) String() string {
	return string(self) //strings.Join(Disassemble(self), " ")
}

type StorageDcrmAccountData map[common.Hash][]byte

func (self StorageDcrmAccountData) Copy() StorageDcrmAccountData {
	cpy := make(StorageDcrmAccountData)
	for key, value := range self {
		cpy[key] = value
	}

	return cpy
}

type Storage map[common.Hash]common.Hash

func (self Storage) String() (str string) {
	for key, value := range self {
		str += fmt.Sprintf("%X : %X\n", key, value)
	}

	return
}

func (self Storage) Copy() Storage {
	cpy := make(Storage)
	for key, value := range self {
		cpy[key] = value
	}

	return cpy
}

// stateObject represents an Ethereum account which is being modified.
//
// The usage pattern is as follows:
// First you need to obtain a state object.
// Account values can be accessed and modified through the object.
// Finally, call CommitTrie to write the modified storage trie into a database.
type stateObject struct {
	address  common.Address
	addrHash common.Hash // hash of ethereum address of the account
	data     Account
	db       *StateDB

	// DB error.
	// State objects are used by the consensus core and VM which are
	// unable to deal with database-level errors. Any error that occurs
	// during a database read is memoized here and will eventually be returned
	// by StateDB.Commit.
	dbErr error

	// Write caches.
	trie Trie // storage trie, which becomes non-nil on first access
	code Code // contract bytecode, which gets set when code is loaded

	originStorage Storage // Storage cache of original entries to dedup rewrites
	dirtyStorage  Storage // Storage entries that need to be flushed to disk

	cachedStorageDcrmAccountData StorageDcrmAccountData
	dirtyStorageDcrmAccountData  StorageDcrmAccountData

	// Cache flags.
	// When an object is marked suicided it will be delete from the trie
	// during the "update" phase of the state transition.
	dirtyCode bool // true if the code was updated
	suicided  bool
	deleted   bool
}

// empty returns whether the account is considered empty.
func (s *stateObject) empty() bool {
    balance_empty := false
    if len(s.data.Balance) == 0 {
	balance_empty = true
    } else {
	balance_empty = true
	for _,v := range s.data.Balance {
	    if v.Balance.Sign() != 0 || v.OutSideBalance.Sign() != 0 {
		balance_empty = false
		break
	    }
	}
    }

    return s.data.Nonce == 0 && balance_empty == true && bytes.Equal(s.data.CodeHash, emptyCodeHash)
}

type DcrmBalance struct {
    Cointype string
    DcrmAddr string
    Hashkey string
    Balance *big.Int
    OutSideBalance *big.Int
}

// Account is the Ethereum consensus representation of accounts.
// These objects are stored in the main account trie.
type Account struct {
	Nonce    uint64
	Balance  []*DcrmBalance
	Root     common.Hash // merkle root of the storage trie
	CodeHash []byte
	MatchNonce string
}

func DeepCopyAccount(data Account) Account {
    var dest Account

    if len(data.Balance) != 0 {
	dest.Balance = make([]*DcrmBalance,len(data.Balance))
	for k,v := range data.Balance {
	    ba := new(big.Int).SetBytes(v.Balance.Bytes())
	    outba := new(big.Int).SetBytes(v.OutSideBalance.Bytes())
	    tmp := &DcrmBalance{Cointype:v.Cointype,DcrmAddr:v.DcrmAddr,Hashkey:v.Hashkey,Balance:ba,OutSideBalance:outba}
	    dest.Balance[k] = tmp
	}
    }

    dest.Nonce  = data.Nonce
    dest.MatchNonce  = data.MatchNonce
    dest.Root = data.Root
    dest.CodeHash = data.CodeHash
    return dest
}

// newObject creates a state object.
func newObject(db *StateDB, address common.Address, data Account) *stateObject {

	if len(data.Balance) == 0 {
	    for _, v := range types.AllSupportedCointypes {
		zero,_ := new(big.Int).SetString("0",10)
		dba := &DcrmBalance{Cointype:v,DcrmAddr:"",Hashkey:"",Balance:zero,OutSideBalance:zero}
		data.Balance = append(data.Balance,dba)
	    }
	}
	
	if data.CodeHash == nil {
		data.CodeHash = emptyCodeHash
	}

	dest := DeepCopyAccount(data)

	return &stateObject{
		db:            db,
		address:       address,
		addrHash:      crypto.Keccak256Hash(address[:]),
		data:          dest,
		originStorage: make(Storage),
		dirtyStorage:  make(Storage),
		cachedStorageDcrmAccountData: make(StorageDcrmAccountData),
		dirtyStorageDcrmAccountData:  make(StorageDcrmAccountData),

	}
}

func (self *stateObject) GetDcrmAccountOutSideBalance(cointype string) (*big.Int,error) {
    for _,v := range self.data.Balance {
	if strings.EqualFold(v.Cointype,cointype) {
	    return v.OutSideBalance,nil
	}
    }

    return new(big.Int),nil
}

func (c *stateObject) AddOutSideBalance(amount *big.Int,cointype string) {
	// EIP158: We must check emptiness for the objects such that the account
	// clearing (0,0,0 objects) can take effect.
	if amount.Sign() == 0 || cointype == "" {
		if c.empty() {
			c.touch()
		}

		return
	}

	for _,v := range c.data.Balance {
	    if strings.EqualFold(v.Cointype,cointype) {
		c.SetOutSideBalance(new(big.Int).Add(v.OutSideBalance, amount),cointype)
		break
	    }
	}
}

// SubOutSideBalance removes amount from c's balance.
// It is used to remove funds from the origin account of a transfer.
func (c *stateObject) SubOutSideBalance(amount *big.Int,cointype string) {
	if amount.Sign() == 0 || cointype == "" {
		return
	}

	for _,v := range c.data.Balance {
	    if strings.EqualFold(v.Cointype,cointype) {
		c.SetOutSideBalance(new(big.Int).Sub(v.OutSideBalance, amount),cointype)
		break
	    }
	}
}

func (self *stateObject) SetOutSideBalance(amount *big.Int,cointype string) {
    for _,v := range self.data.Balance {
	if strings.EqualFold(v.Cointype,cointype) {
	    self.db.journal.append(outsidebalanceChange{
		    account: &self.address,
		    prev:    new(big.Int).Set(v.OutSideBalance),
		    cointype:cointype,
	    })
	    self.setOutSideBalance(amount,cointype)
	    break
	}
    }
}

func (self *stateObject) setOutSideBalance(amount *big.Int,cointype string) {
	for _,v := range self.data.Balance {
	    if strings.EqualFold(v.Cointype,cointype) {
		v.OutSideBalance = amount
		break
	    }
	}
}

func (self *stateObject) OutSideBalance(cointype string) *big.Int {
    if cointype == "" {
	return new(big.Int)
    }

    for _,v := range self.data.Balance {
	if strings.EqualFold(v.Cointype,cointype) {
	    return v.OutSideBalance
	}
    }

    return new(big.Int)
}

// EncodeRLP implements rlp.Encoder.
func (c *stateObject) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, c.data)
}

// setError remembers the first non-nil error it is called with.
func (self *stateObject) setError(err error) {
	if self.dbErr == nil {
		self.dbErr = err
	}
}

func (self *stateObject) markSuicided() {
	self.suicided = true
}

func (c *stateObject) touch() {
	c.db.journal.append(touchChange{
		account: &c.address,
	})
	if c.address == ripemd {
		// Explicitly put it in the dirty-cache, which is otherwise generated from
		// flattened journals.
		c.db.journal.dirty(c.address)
	}
}

func (c *stateObject) getTrie(db Database) Trie {
	if c.trie == nil {
		var err error
		c.trie, err = db.OpenStorageTrie(c.addrHash, c.data.Root)
		if err != nil {
			c.trie, _ = db.OpenStorageTrie(c.addrHash, common.Hash{})
			c.setError(fmt.Errorf("can't create storage trie: %v", err))
		}
	}
	return c.trie
}

// GetState retrieves a value from the account storage trie.
func (self *stateObject) GetState(db Database, key common.Hash) common.Hash {
	// If we have a dirty value for this state entry, return it
	value, dirty := self.dirtyStorage[key]
	if dirty {
		return value
	}
	// Otherwise return the entry's original value
	return self.GetCommittedState(db, key)
}

// GetCommittedState retrieves a value from the committed account storage trie.
func (self *stateObject) GetCommittedState(db Database, key common.Hash) common.Hash {
	// If we have the original value cached, return that
	value, cached := self.originStorage[key]
	if cached {
		return value
	}
	// Otherwise load the value from the database
	enc, err := self.getTrie(db).TryGet(key[:])
	if err != nil {
		self.setError(err)
		return common.Hash{}
	}
	if len(enc) > 0 {
		_, content, _, err := rlp.Split(enc)
		if err != nil {
			self.setError(err)
		}
		value.SetBytes(content)
	}
	self.originStorage[key] = value
	return value
}

type ErrorRet struct {
    Code int
    Error string
}

func GetRetErrJsonStr(code int,err string) string {
    m := &ErrorRet{Code:code,Error:err}
    ret,_ := json.Marshal(m)
    return string(ret)
}

func (self *stateObject) GetDcrmAccountLockinHashkey(cointype string) (string,error) {
    for _,v := range self.data.Balance {
	if strings.EqualFold(v.Cointype,cointype) {
	    return v.Hashkey,nil
	}
    }

    return "",errors.New("get hashkey fail.")
}

func (self *stateObject) GetDcrmAccountBalance(cointype string) (*big.Int,error) {
    for _,v := range self.data.Balance {
	if strings.EqualFold(v.Cointype,cointype) {
	    return v.Balance,nil
	}
    }

    return new(big.Int),nil
}

func getDataByIndex(value string,index int) (string,string,string,error) {
	if value == "" || index < 0 {
		s := GetRetErrJsonStr(117,"read data from block fail.")
		return "","","",fmt.Errorf(s)
	}

	v := strings.Split(value,"|")
	if len(v) < (index + 1) {
		s := GetRetErrJsonStr(117,"read data from block fail.")
		return "","","",fmt.Errorf(s)
	}

	vv := v[index]
	ss := strings.Split(vv,":")
	if len(ss) == 3 {
	    return ss[0],ss[1],ss[2],nil
	}
	if len(ss) == 2 {//for prev version
	    return ss[0],ss[1],"",nil
	}

	s := GetRetErrJsonStr(117,"read data from block fail.")
	return "","","",fmt.Errorf(s)
}

func IsExsitDcrmAddrInData(value string,dcrmaddr string) (bool,error) {
	if value == "" || dcrmaddr == "" {
		s := GetRetErrJsonStr(118,"read data from block,param error.")
		return false,fmt.Errorf(s)
	}

	v := strings.Split(value,"|")
	if len(v) < 1 {
		s := GetRetErrJsonStr(117,"read data from block fail.")
		return false,fmt.Errorf(s)
	}

	for _,vv := range v {
	    ss := strings.Split(vv,":")
	    if strings.EqualFold(ss[0],dcrmaddr) {
		return true,nil
	    }
	}

	return false,nil
}

func (self *stateObject) IsExsitDcrmAddress(db Database,cointype string,dcrmaddr string) (bool,error) {

        for _,v := range self.data.Balance {
	if strings.EqualFold(v.Cointype,cointype) {
	    if v.DcrmAddr != "" && strings.EqualFold(v.DcrmAddr,dcrmaddr) {
		return true,nil
	    }
	}
    }
    
    return false,errors.New("no get dcrm addr.")
}

func (self *stateObject) GetStateDcrmAccountData(db Database, key common.Hash) []byte {
	//log.Debug("========stateObject.GetStateDcrmAccountData================")
	// If we have a dirty value for this state entry, return it
	value, dirty := self.dirtyStorageDcrmAccountData[key]
	if dirty {
		return value
	}
	//log.Debug("========stateObject.GetStateDcrmAccountData,call GetCommittedStateDcrmAccountData================")
	// Otherwise return the entry's original value
	return self.GetCommittedStateDcrmAccountData(db, key)
}

func (self *stateObject) GetCommittedStateDcrmAccountData(db Database, key common.Hash) []byte {
	//log.Debug("========stateObject.GetCommittedStateDcrmAccountData================")
	value, exists := self.cachedStorageDcrmAccountData[key]
	if exists {
		return value
	}
	//log.Debug("========stateObject.GetCommittedStateDcrmAccountData,call TryGet","key",string(key[:]),"","================")
	// Load from DB in case it is missing.
	value, err := self.getTrie(db).TryGet(key[:])
	if err == nil && len(value) != 0 {
		self.cachedStorageDcrmAccountData[key] = value
	}
	//log.Debug("========stateObject.GetCommittedStateDcrmAccountData,call TryGet","value",string(value),"","================")
 	return value
 }

func (self *stateObject) SetStateDcrmAccountData(db Database, key common.Hash, value []byte) {
	//log.Debug("========stateObject.SetStateDcrmAccountData================")
	self.db.journal.append(storageDcrmAccountDataChange{
		account:  &self.address,
		key:      key,
		prevalue: self.GetStateDcrmAccountData(db, key),
	})
	self.setStateDcrmAccountData(key, value)
}

func (self *stateObject) setStateDcrmAccountData(key common.Hash, value []byte) {
	//log.Debug("===============SetStateDcrmAccountData,value is %s===========\n",string(value))
	//self.cachedStorageDcrmAccountData[key] = value
	self.dirtyStorageDcrmAccountData[key] = value

	//if self.onDirty != nil {
	//	self.onDirty(self.Address())
	//	self.onDirty = nil
	//}
}

// SetState updates a value in account storage.
func (self *stateObject) SetState(db Database, key, value common.Hash) {
	// If the new value is the same as old, don't set
	prev := self.GetState(db, key)
	if prev == value {
		return
	}
	// New value is different, update and journal the change
	self.db.journal.append(storageChange{
		account:  &self.address,
		key:      key,
		prevalue: prev,
	})
	self.setState(key, value)
}

func (self *stateObject) setState(key, value common.Hash) {
	self.dirtyStorage[key] = value
}

// updateTrie writes cached storage modifications into the object's storage trie.
func (self *stateObject) updateTrie(db Database) Trie {
	//log.Debug("","===============stateObject.updateTrie===========")
	tr := self.getTrie(db)
	for key, value := range self.dirtyStorage {
	    //log.Debug("===============stateObject.updateTrie, dirtyStorage:","get key",key,"get value",value.Hex(),"","=====================")
		delete(self.dirtyStorage, key)

		// Skip noop changes, persist actual changes
		if value == self.originStorage[key] {
		    //log.Debug("============stateObject.updateTrie, dirtyStorage:","key",key.Hex(),"","no change,and skip.============")
			continue
		}
		self.originStorage[key] = value

		if (value == common.Hash{}) {
		    //log.Debug("===============stateObject.updateTrie, dirtyStorage:","key",key.Hex(),"","value is nil and delete it.===========")
			self.setError(tr.TryDelete(key[:]))
			continue
		}
		// Encoding []byte cannot fail, ok to ignore the error.
		v, _ := rlp.EncodeToBytes(bytes.TrimLeft(value[:], "\x00"))
		self.setError(tr.TryUpdate(key[:], v))
	}

	for key, value := range self.dirtyStorageDcrmAccountData {
	    //log.Debug("===============stateObject.updateTrie, dirtyStorageDcrmAccountData:","get key",key.Hex(),"get value",string(value),"","=====================")
		delete(self.dirtyStorageDcrmAccountData, key)

		// Skip noop changes, persist actual changes
		if string(value) == string(self.cachedStorageDcrmAccountData[key]) {
		    //log.Debug("============stateObject.updateTrie, dirtyStorageDcrmAccountData:","key",key.Hex(),"","no change,and skip.============")
			continue
		}
		self.cachedStorageDcrmAccountData[key] = value

		if (value == nil) {
		    //log.Debug("===============stateObject.updateTrie, dirtyStorageDcrmAccountData:","key",key.Hex(),"","value is nil and delete it.===========")
			self.setError(tr.TryDelete(key[:]))
			continue
		}
		// Encoding []byte cannot fail, ok to ignore the error.
		//v, _ := rlp.EncodeToBytes(bytes.TrimLeft(value[:], "\x00"))
		v := value//v, _ := rlp.EncodeToBytes(bytes.TrimLeft(value[:], ""))
		//log.Debug("===============stateObject.updateTrie, dirtyStorageDcrmAccountData:","key",key.Hex(),"value",string(v),"","is update into trie.===========")
		self.setError(tr.TryUpdate(key[:], v))
	}
	return tr
}

// UpdateRoot sets the trie root to the current root hash of
func (self *stateObject) updateRoot(db Database) {
	self.updateTrie(db)
	self.data.Root = self.trie.Hash()
}

// CommitTrie the storage trie of the object to db.
// This updates the trie root.
func (self *stateObject) CommitTrie(db Database) error {
	//log.Debug("=========stateObject.CommitTrie, call updateTrie to update trie root and write to db ======")
	self.updateTrie(db)
	if self.dbErr != nil {
	    //log.Debug("=========stateObject.CommitTrie,db error.======")
		return self.dbErr
	}
	root, err := self.trie.Commit(nil)
	if err == nil {
		//log.Debug("=========stateObject.CommitTrie,update root ======")
		self.data.Root = root
	}
	return err
}

// AddBalance removes amount from c's balance.
// It is used to add funds to the destination account of a transfer.
func (c *stateObject) AddBalance(amount *big.Int,cointype string) {
	// EIP158: We must check emptiness for the objects such that the account
	// clearing (0,0,0 objects) can take effect.
	if amount.Sign() == 0 || cointype == "" {
		if c.empty() {
			c.touch()
		}

		return
	}
	
	for _,v := range c.data.Balance {
	    if strings.EqualFold(v.Cointype,cointype) {
		c.SetBalance(new(big.Int).Add(v.Balance, amount),cointype)
		break
	    }
	}
}

// SubBalance removes amount from c's balance.
// It is used to remove funds from the origin account of a transfer.
func (c *stateObject) SubBalance(amount *big.Int,cointype string) {
	if amount.Sign() == 0 || cointype == "" {
		return
	}
	
	for _,v := range c.data.Balance {
	    if strings.EqualFold(v.Cointype,cointype) {
		c.SetBalance(new(big.Int).Sub(v.Balance, amount),cointype)
		break
	    }
	}
}

func (self *stateObject) SetBalance(amount *big.Int,cointype string) {
    for _,v := range self.data.Balance {
	if strings.EqualFold(v.Cointype,cointype) {
	    self.db.journal.append(balanceChange{
		    account: &self.address,
		    prev:    new(big.Int).Set(v.Balance),
		    cointype:cointype,
	    })
	    self.setBalance(amount,cointype)
	    break
	}
    }
}

func (self *stateObject) setBalance(amount *big.Int,cointype string) {
	for _,v := range self.data.Balance {
	    if strings.EqualFold(v.Cointype,cointype) {
		v.Balance = amount
		break
	    }
	}
}

func (self *stateObject) SetDcrmAddr(dcrmaddr string,cointype string) {
        log.Debug("===========stateobject.SetDcrmAddr=========","dcrmaddr",dcrmaddr,"cointype",cointype,"state object",self)

	for _,v := range self.data.Balance {
	    if strings.EqualFold(v.Cointype,cointype) {
		self.db.journal.append(dcrmAddrChange{
			account: &self.address,
			prev:    v.DcrmAddr,
			cointype:cointype,
		})
		self.setDcrmAddr(dcrmaddr,cointype)
		break
	    }
	}
}

func (self *stateObject) setDcrmAddr(dcrmaddr string,cointype string) {
	for _,v := range self.data.Balance {
	    if strings.EqualFold(v.Cointype,cointype) {
		v.DcrmAddr = dcrmaddr
		break
	    }
	}
}

func (self *stateObject) SetHashkey(hashkey string,cointype string) {
        log.Debug("===========stateobject.SetHashkey=========","hashkey",hashkey,"cointype",cointype,"state object",self)

	for _,v := range self.data.Balance {
	    if strings.EqualFold(v.Cointype,cointype) {
		self.db.journal.append(hashkeyChange{
			account: &self.address,
			prev:    v.Hashkey,
			cointype:cointype,
		})
		self.setHashkey(hashkey,cointype)
		break
	    }
	}
}

func (self *stateObject) setHashkey(hashkey string,cointype string) {
	for _,v := range self.data.Balance {
	    if strings.EqualFold(v.Cointype,cointype) {
		v.Hashkey = hashkey
		break
	    }
	}
}

// Return the gas back to the origin. Used by the Virtual machine or Closures
func (c *stateObject) ReturnGas(gas *big.Int) {}

func (self *stateObject) deepCopy(db *StateDB) *stateObject {
	stateObject := newObject(db, self.address, self.data)
	if self.trie != nil {
		stateObject.trie = db.db.CopyTrie(self.trie)
	}
	stateObject.code = self.code
	stateObject.dirtyStorage = self.dirtyStorage.Copy()
	stateObject.originStorage = self.originStorage.Copy()
	stateObject.dirtyStorageDcrmAccountData = self.dirtyStorageDcrmAccountData.Copy()
	stateObject.cachedStorageDcrmAccountData = self.cachedStorageDcrmAccountData.Copy()
	stateObject.suicided = self.suicided
	stateObject.dirtyCode = self.dirtyCode
	stateObject.deleted = self.deleted
	return stateObject
}

//
// Attribute accessors
//

// Returns the address of the contract/account
func (c *stateObject) Address() common.Address {
	return c.address
}

// Code returns the contract code associated with this object, if any.
func (self *stateObject) Code(db Database) []byte {
	if self.code != nil {
		return self.code
	}
	if bytes.Equal(self.CodeHash(), emptyCodeHash) {
		return nil
	}
	code, err := db.ContractCode(self.addrHash, common.BytesToHash(self.CodeHash()))
	if err != nil {
		self.setError(fmt.Errorf("can't load code hash %x: %v", self.CodeHash(), err))
	}
	self.code = code
	return code
}

func (self *stateObject) SetCode(codeHash common.Hash, code []byte) {
	prevcode := self.Code(self.db.db)
	self.db.journal.append(codeChange{
		account:  &self.address,
		prevhash: self.CodeHash(),
		prevcode: prevcode,
	})
	self.setCode(codeHash, code)
}

func (self *stateObject) setCode(codeHash common.Hash, code []byte) {
	self.code = code
	self.data.CodeHash = codeHash[:]
	self.dirtyCode = true
}

func (self *stateObject) SetNonce(nonce uint64) {
        //log.Debug("===========stateobject.SetNonce=========","nonce",nonce)
	self.db.journal.append(nonceChange{
		account: &self.address,
		prev:    self.data.Nonce,
	})
	self.setNonce(nonce)
}

func (self *stateObject) setNonce(nonce uint64) {
	self.data.Nonce = nonce
}

func (self *stateObject) SetMatchNonce(nonce string) {
	self.db.journal.append(matchnonceChange{
		account: &self.address,
		prev:    self.data.MatchNonce,
	})
	self.setMatchNonce(nonce)
}

func (self *stateObject) AddDcrmAddr(dcrmaddr string,cointype string) {
        log.Debug("===========stateobject.AddDcrmAddr=========","dcrmaddr",dcrmaddr,"cointype",cointype,"state object",self)

	var has bool = false
	for _,v := range self.data.Balance {
	    if strings.EqualFold(v.Cointype,cointype) {
		has = true
		self.db.journal.append(dcrmAddrChange{
			account: &self.address,
			prev:    v.DcrmAddr,
			cointype:cointype,
		})
		self.setDcrmAddr(dcrmaddr,cointype)
		break
	    }
	}
	if !has {
		self.db.journal.append(dcrmAddrChange{
				account: &self.address,
				cointype: cointype,
		})
		self.addDcrmAddr(dcrmaddr,cointype)
	}
}

func (self *stateObject) addDcrmAddr(dcrmaddr string,cointype string) {
	zero,_ := new(big.Int).SetString("0",10)
	self.data.Balance = append(self.data.Balance, &DcrmBalance{Cointype:cointype, DcrmAddr:dcrmaddr, Hashkey:"", Balance:zero, OutSideBalance:zero})
}

func (self *stateObject) setMatchNonce(nonce string) {
	self.data.MatchNonce = nonce
}

func (self *stateObject) CodeHash() []byte {
	return self.data.CodeHash
}

func (self *stateObject) Balance(cointype string) *big.Int {
    if cointype == "" {
	return new(big.Int)
    }

    for _,v := range self.data.Balance {
	if strings.EqualFold(v.Cointype,cointype) {
	    return v.Balance
	}
    }

    return new(big.Int)
}

func (self *stateObject) DcrmAddr(cointype string) string {
    if cointype == "" {
	return ""
    }

    for _,v := range self.data.Balance {
	if strings.EqualFold(v.Cointype,cointype) {
	    return v.DcrmAddr
	}
    }

    return ""
}

func (self *stateObject) Hashkey(cointype string) string {
    if cointype == "" {
	return ""
    }

    for _,v := range self.data.Balance {
	if strings.EqualFold(v.Cointype,cointype) {
	    return v.Hashkey
	}
    }

    return ""
}

func (self *stateObject) Nonce() uint64 {
	return self.data.Nonce
}

func (self *stateObject) MatchNonce() string {
	return self.data.MatchNonce
}

// Never called, but must be present to allow stateObject to be used
// as a vm.Account interface that also satisfies the vm.ContractRef
// interface. Interfaces are awesome.
func (self *stateObject) Value() *big.Int {
	panic("Value on stateObject should never be called")
}
