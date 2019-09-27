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

package state

import (
	"math/big"

	"github.com/fusion/go-fusion/common"
	//"github.com/fusion/go-fusion/core/types"
)

// journalEntry is a modification entry in the state change journal that can be
// reverted on demand.
type journalEntry interface {
	// revert undoes the changes introduced by this journal entry.
	revert(*StateDB)

	// dirtied returns the Ethereum address modified by this journal entry.
	dirtied() *common.Address
}

// journal contains the list of state modifications applied since the last state
// commit. These are tracked to be able to be reverted in case of an execution
// exception or revertal request.
type journal struct {
	entries []journalEntry         // Current changes tracked by the journal
	dirties map[common.Address]int // Dirty accounts and the number of changes
}

// newJournal create a new initialized journal.
func newJournal() *journal {
	return &journal{
		dirties: make(map[common.Address]int),
	}
}

// append inserts a new modification entry to the end of the change journal.
func (j *journal) append(entry journalEntry) {
	j.entries = append(j.entries, entry)
	if addr := entry.dirtied(); addr != nil {
		j.dirties[*addr]++
	}
}

// revert undoes a batch of journalled modifications along with any reverted
// dirty handling too.
func (j *journal) revert(statedb *StateDB, snapshot int) {
	for i := len(j.entries) - 1; i >= snapshot; i-- {
		// Undo the changes made by the operation
		j.entries[i].revert(statedb)

		// Drop any dirty tracking induced by the change
		if addr := j.entries[i].dirtied(); addr != nil {
			if j.dirties[*addr]--; j.dirties[*addr] == 0 {
				delete(j.dirties, *addr)
			}
		}
	}
	j.entries = j.entries[:snapshot]
}

// dirty explicitly sets an address to dirty, even if the change entries would
// otherwise suggest it as clean. This method is an ugly hack to handle the RIPEMD
// precompile consensus exception.
func (j *journal) dirty(addr common.Address) {
	j.dirties[addr]++
}

// length returns the current number of entries in the journal.
func (j *journal) length() int {
	return len(j.entries)
}

type (
	// Changes to the account trie.
	createObjectChange struct {
		account *common.Address
	}
	resetObjectChange struct {
		prev *stateObject
	}
	suicideChange struct {
		account     *common.Address
		prev        bool // whether account had already suicided
		prevbalance []*DcrmBalance
	}

	// Changes to individual accounts.
	balanceChange struct {
		account *common.Address
		prev    *big.Int
		cointype string
	}
	outsidebalanceChange struct {
		account *common.Address
		prev    *big.Int
		cointype string
	}
	nonceChange struct {
		account *common.Address
		prev    uint64
	}

	matchnonceChange struct {
		account *common.Address
		prev    string
	}

	dcrmAddrChange struct {
		account *common.Address
		prev    string
		cointype string
	}

	hashkeyChange struct {
		account *common.Address
		prev    string
		cointype string
	}

	storageDcrmAccountDataChange struct {
		account  *common.Address
		key      common.Hash
		prevalue []byte
	}

	storageChange struct {
		account       *common.Address
		key, prevalue common.Hash
	}
	codeChange struct {
		account            *common.Address
		prevcode, prevhash []byte
	}

	// Changes to other state values.
	refundChange struct {
		prev uint64
	}
	addLogChange struct {
		txhash common.Hash
	}
	addPreimageChange struct {
		hash common.Hash
	}
	touchChange struct {
		account   *common.Address
		prev      bool
		prevDirty bool
	}
)

func (ch createObjectChange) revert(s *StateDB) {
	//delete(s.stateObjects, *ch.account)
	//delete(s.stateObjectsDirty, *ch.account)
	if s.stateObjects == nil || s.stateObjectsDirty == nil {
	    return
	}
	s.stateObjects.DeleteMap(*ch.account)
	s.stateObjectsDirty.DeleteMap(*ch.account)
}

func (ch createObjectChange) dirtied() *common.Address {
	return ch.account
}

func (ch resetObjectChange) revert(s *StateDB) {
	s.setStateObject(ch.prev)
}

func (ch resetObjectChange) dirtied() *common.Address {
	return nil
}

func (ch suicideChange) revert(s *StateDB) {
	obj := s.getStateObject(*ch.account)
	if obj != nil {
		obj.suicided = ch.prev
		if len(ch.prevbalance) != 0 {
		    obj.data.Balance = make([]*DcrmBalance,len(ch.prevbalance))
		    for k,v := range ch.prevbalance {
			ba := new(big.Int).SetBytes(v.Balance.Bytes())
			outba := new(big.Int).SetBytes(v.OutSideBalance.Bytes())
			tmp := &DcrmBalance{Cointype:v.Cointype,DcrmAddr:v.DcrmAddr,Hashkey:v.Hashkey,Balance:ba,OutSideBalance:outba}
			obj.data.Balance[k] = tmp
		    }
		} else {
		    obj.data.Balance = make([]*DcrmBalance,len(ch.prevbalance))
		}
	}
}

func (ch suicideChange) dirtied() *common.Address {
	return ch.account
}

func (ch outsidebalanceChange) revert(s *StateDB) {
    s.getStateObject(*ch.account).setOutSideBalance(ch.prev,ch.cointype)
}

func (ch outsidebalanceChange) dirtied() *common.Address {
	return ch.account
}

func (ch storageDcrmAccountDataChange) revert(s *StateDB) {
	s.getStateObject(*ch.account).setStateDcrmAccountData(ch.key, ch.prevalue)
}

func (ch storageDcrmAccountDataChange) dirtied() *common.Address {
	return ch.account
}

var ripemd = common.HexToAddress("0000000000000000000000000000000000000003")

func (ch touchChange) revert(s *StateDB) {
}

func (ch touchChange) dirtied() *common.Address {
	return ch.account
}

func (ch balanceChange) revert(s *StateDB) {
	s.getStateObject(*ch.account).setBalance(ch.prev,ch.cointype)
}

func (ch balanceChange) dirtied() *common.Address {
	return ch.account
}

func (ch nonceChange) revert(s *StateDB) {
	s.getStateObject(*ch.account).setNonce(ch.prev)
}

func (ch nonceChange) dirtied() *common.Address {
	return ch.account
}

func (ch matchnonceChange) revert(s *StateDB) {
	s.getStateObject(*ch.account).setMatchNonce(ch.prev)
}

func (ch matchnonceChange) dirtied() *common.Address {
	return ch.account
}

func (ch codeChange) revert(s *StateDB) {
	s.getStateObject(*ch.account).setCode(common.BytesToHash(ch.prevhash), ch.prevcode)
}

func (ch codeChange) dirtied() *common.Address {
	return ch.account
}

func (ch storageChange) revert(s *StateDB) {
	s.getStateObject(*ch.account).setState(ch.key, ch.prevalue)
}

func (ch storageChange) dirtied() *common.Address {
	return ch.account
}

func (ch refundChange) revert(s *StateDB) {
	s.refund = ch.prev
}

func (ch refundChange) dirtied() *common.Address {
	return nil
}

func (ch addLogChange) revert(s *StateDB) {
	logs := s.logs[ch.txhash]
	if len(logs) == 1 {
		delete(s.logs, ch.txhash)
	} else {
		s.logs[ch.txhash] = logs[:len(logs)-1]
	}
	s.logSize--
}

func (ch addLogChange) dirtied() *common.Address {
	return nil
}

func (ch addPreimageChange) revert(s *StateDB) {
	delete(s.preimages, ch.hash)
}

func (ch addPreimageChange) dirtied() *common.Address {
	return nil
}

func (ch dcrmAddrChange) revert(s *StateDB) {
	s.getStateObject(*ch.account).setDcrmAddr(ch.prev,ch.cointype)
}

func (ch dcrmAddrChange) dirtied() *common.Address {
	return ch.account
}

func (ch hashkeyChange) revert(s *StateDB) {
	s.getStateObject(*ch.account).setHashkey(ch.prev,ch.cointype)
}

func (ch hashkeyChange) dirtied() *common.Address {
	return ch.account
}

