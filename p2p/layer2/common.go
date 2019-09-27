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

package layer2

import (
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	mapset "github.com/deckarep/golang-set"
	"github.com/fusion/go-fusion/common"
	"github.com/fusion/go-fusion/crypto/sha3"
	"github.com/fusion/go-fusion/log"
	"github.com/fusion/go-fusion/p2p"
	"github.com/fusion/go-fusion/p2p/discover"
	"github.com/fusion/go-fusion/rlp"
)

func BroadcastToGroup(msg string, p2pType int) {
	emitter.Lock()
	defer emitter.Unlock()

	log.Debug("==== BroadcastToGroup() ====\n")
	if msg == "" || emitter == nil {
		return
	}
	log.Debug("BroadcastToGroup", "sendMsg", msg)
	dccpGroupfail := NewGroup()
	msgCode := peerMsgCode
	broatcast := func(dccpGroup *Group, fail bool) int {
		if dccpGroup == nil {
			return 0
		}
		var ret int = 0
		log.Debug("emitter", "peer: ", emitter)
		for _, g := range dccpGroup.group {
			log.Debug("group", "g: ", g)
			//if selfid == g.id {
			//	continue
			//}
			if fail == false {
				if dccpGroupfail.group[g.id.String()] == nil {
					continue
				}
			}
			p := emitter.peers[g.id]
			if p == nil {
				log.Debug("BroadastToGroup", "NodeID: ", g.id, "not in peers\n")
				continue
			}
			if err := p2p.Send(p.ws, uint64(msgCode), msg); err != nil {
				log.Debug("send to node(group) failed", "g = ", g, "p.peer = ", p.peer)
				log.Error("BroadcastToGroup", "p2p.Send err", err, "peer id", p.peer.ID())
				ret += 1
				if fail == true {
					dccpGroupfail.group[g.id.String()] = &group{id: g.id, ip: g.ip, port: g.port, enode: g.enode}
				}
			} else {
				tx := Transaction{Payload: []byte(msg)}
				p.knownTxs.Add(tx.hash())
				log.Debug("send to node(group) success", "g = ", g, "p.peer = ", p.peer)
				if fail == false {
					dccpGroupfail.group[g.id.String()] = nil
				}
			}
		}
		return ret
	}
	var xvcGroup *Group
	switch p2pType {
	case DcrmProtocol_type:
		if dccpGroup != nil {
			xvcGroup = dccpGroup
			msgCode = Dcrm_msgCode
		}
	case Xprotocol_type:
		if xpGroup != nil {
			xvcGroup = xpGroup
			msgCode = Xp_msgCode
		}
	default:
		return
	}
	log.Debug("BroadcastToGroup", "group: ", xvcGroup)
	failret := broatcast(xvcGroup, true)
	if failret != 0 {
		go func() {
			if broatcastFailTimes == 0 {
				return
			}
			log.Debug("BroatcastToGroupFail", "group: ", dccpGroupfail)
			var i int
			for i = 0; i < broatcastFailTimes; i++ {
				log.Debug("BroatcastToGroupFail", "group times", i+1)
				if broatcast(dccpGroupfail, false) == 0 {
					break
				}
				time.Sleep(time.Duration(broatcastFailOnce) * time.Second)
			}
			if i > broatcastFailTimes {
				log.Debug("BroatcastToGroupFail", "group: ", "failed")
				log.Debug("BroatcastToGroupFail", "group: ", dccpGroupfail)
			} else {
				log.Debug("BroatcastToGroupFail", "group: ", "success")
			}
		}()
	}
}

func init() {
	glogger := log.NewGlogHandler(log.StreamHandler(os.Stderr, log.TerminalFormat(false)))
	//glogger.Verbosity(log.Lvl(*verbosity))
	log.Root().SetHandler(glogger)

	emitter = NewEmitter()
	discover.RegisterGroupCallback(recvGroupInfo)
	//TODO callback
	//RegisterRecvCallback(recvPrivkeyInfo)
}
func NewEmitter() *Emitter {
	//fmt.Println("========  NewEmitter()  ========")
	return &Emitter{peers: make(map[discover.NodeID]*peer)}
}
func NewGroup() *Group {
	return &Group{group: make(map[string]*group)}
}

// update p2p
func (e *Emitter) addPeer(p *p2p.Peer, ws p2p.MsgReadWriter) {
	fmt.Println("==== addPeer() ====")
	fmt.Printf("id: %+v ...\n", p.ID().String()[:8])
	log.Debug("addPeer", "p: ", p, "ws: ", ws)
	e.Lock()
	defer e.Unlock()
	e.peers[p.ID()] = &peer{ws: ws, peer: p, peerInfo: &peerInfo{int(ProtocolVersion)}, knownTxs: mapset.NewSet()}
}

func HandlePeer(peer *p2p.Peer, rw p2p.MsgReadWriter) error {
	log.Debug("==== HandlePeer() ====\n")
	emitter.addPeer(peer, rw)
	log.Debug("emitter", "emitter.peers: ", emitter.peers)
	for {
		msg, err := rw.ReadMsg()
		log.Debug("HandlePeer", "ReadMsg", msg)
		if err != nil {
			return err
		}
		log.Debug("HandlePeer", "receive Msgs msg.Payload", msg.Payload)
		switch msg.Code {
		case peerMsgCode:
			var recv []byte
			err := rlp.Decode(msg.Payload, &recv)
			log.Debug("Decode", "rlp.Decode", recv)
			if err != nil {
				fmt.Printf("Err: decode msg err %+v\n", err)
			} else {
				log.Debug("HandlePeer", "callback(msg): ", recv)
				go callEvent(string(recv))
			}
		case Dcrm_msgCode:
			var recv []byte
			err := rlp.Decode(msg.Payload, &recv)
			log.Debug("Decode", "rlp.Decode", recv)
			if err != nil {
				fmt.Printf("Err: decode msg err %+v\n", err)
			} else {
				log.Debug("HandlePeer", "callback(msg): ", recv)
				go Dcrm_callEvent(string(recv))
			}
		case Xp_msgCode:
			var recv []byte
			err := rlp.Decode(msg.Payload, &recv)
			log.Debug("Decode", "rlp.Decode", recv)
			if err != nil {
				fmt.Printf("Err: decode msg err %+v\n", err)
			} else {
				log.Debug("HandlePeer", "callback(msg): ", recv)
				go Xp_callEvent(string(recv))
			}
		default:
			fmt.Println("unkown msg code")
		}
	}
	return nil
}

// receive message form peers
func RegisterCallback(recvFunc func(interface{})) {
	callback = recvFunc
}
func callEvent(msg string) {
	callback(msg)
}

func GetSelfID() discover.NodeID {
	return discover.GetLocalID()
}

func getGroup(p2pType int) (int, string) {
	var xvcGroup *Group
	switch p2pType {
	case DcrmProtocol_type:
		if dccpGroup == nil {
			return 0, ""
		}
		xvcGroup = dccpGroup
	case Xprotocol_type:
		if xpGroup == nil {
			return 0, ""
		}
		xvcGroup = xpGroup
	default:
		return 0, ""
	}
	enode := ""
	count := 0
	for i, e := range xvcGroup.group {
		log.Debug("GetGroup", "i", i, "e", e)
		if enode != "" {
			enode += discover.Dcrmdelimiter
		}
		enode += e.enode
		count++
	}
	log.Debug("group", "count = ", count, "enode = ", enode)
	//TODO
	return count, enode
}

func recvGroupInfo(req interface{}, p2pType int) {
	log.Debug("==== recvGroupInfo() ====\n")
	selfid = discover.GetLocalID()
	log.Debug("recvGroupInfo", "local ID: ", selfid)
	var xvcGroup *Group
	switch (p2pType) {
	case DcrmProtocol_type:
		dccpGroup = NewGroup()
		xvcGroup = dccpGroup
	case Xprotocol_type:
		xpGroup = NewGroup()
		xvcGroup = xpGroup
	default:
		return
	}
	for i, enode := range req.([]*discover.Node) {
		log.Debug("recvGroupInfo", "i: ", i, "e: ", enode)
		node, _ := discover.ParseNode(enode.String())
		xvcGroup.group[node.ID.String()] = &group{id: node.ID, ip: node.IP, port: node.UDP, enode: enode.String()}
		log.Debug("recvGroupInfo", "xvcGroup.group", xvcGroup.group[node.ID.String()])
	}
	log.Debug("recvGroupInfo", "xvcGroup", xvcGroup)
	log.Info("recvGroupInfo", "Group", p2pType, "enodes", xvcGroup)
}

func Broadcast(msg string) {
	log.Debug("==== Broadcast() ====\n")
	if msg == "" || emitter == nil {
		return
	}
	log.Debug("Broadcast", "sendMsg", msg)
	emitter.Lock()
	defer emitter.Unlock()
	func() {
		log.Debug("peer", "emitter", emitter)
		for _, p := range emitter.peers {
			log.Debug("Broadcast", "to , p", p, "msg", p, msg)
			log.Debug("Broadcast", "p.ws", p.ws)
			if err := p2p.Send(p.ws, peerMsgCode, msg); err != nil {
				log.Error("Broadcast", "p2p.Send err", err, "peer id", p.peer.ID())
				continue
			}
		}
	}()
}

func SendMsgToPeer(enode string, msg string) error {
	log.Debug("==== SendMsgToPeer() ====\n")
	node, _ := discover.ParseNode(enode)
	p := emitter.peers[node.ID]
	if p == nil {
		log.Debug("Failed: SendToPeer peers mismatch peerID", "peerID", node.ID)
		return errors.New("peerID mismatch!")
	}
	if err := p2p.Send(p.ws, peerMsgCode, msg); err == nil {
		log.Debug("Failed: SendToPeer", "peerID", node.ID, "msg", msg)
		return err
	}
	log.Debug("Success: SendToPeer", "peerID", node.ID, "msg", msg)
	return nil
}

//func SendMsg(msg string) {
//	Dcrmrotocol_broadcastToGroup(msg)
//}

func SendToPeer(enode string, msg string) {
	log.Debug("==== DCCP SendToPeer ====\n")
	log.Debug("SendToPeer", "enode: ", enode, "msg: ", msg)
	node, _ := discover.ParseNode(enode)
	//log.Debug("node.id: %+v, node.IP: %+v, node.UDP: %+v\n", node.ID, node.IP, node.UDP)
	ipa := &net.UDPAddr{IP: node.IP, Port: int(node.UDP)}
	discover.SendMsgToNode(node.ID, ipa, msg)
}


// broadcastInGroup will propagate a batch of message to all peers which are not known to
// already have the given message.
func (e *Emitter) broadcastInGroup(tx Transaction) {
	e.Lock()
	defer e.Unlock()

	var txset = make(map[*peer][]Transaction)

	// Broadcast message to a batch of peers not knowing about it
	peers := e.peersWithoutTx(tx.hash(), true)
	log.Debug("broadcastInGroup", "peers", peers)
	for _, peer := range peers {
		txset[peer] = append(txset[peer], tx)
	}
	log.Trace("Broadcast transaction", "hash", tx.hash(), "recipients", len(peers))

	for peer, txs := range txset {
		peer.sendTx(txs)
	}
}

// group: true, in group
//        false, peers
func (e *Emitter) peersWithoutTx(hash common.Hash, group bool) []*peer {
	list := make([]*peer, 0, len(e.peers))
	if group == true {
		if dccpGroup == nil || len(dccpGroup.group) == 0 {
			return list
		}
		for _, g := range dccpGroup.group {
			if g.id == selfid {
				continue
			}
			log.Debug("peersWithoutTx", "emitter", e)
			log.Debug("peersWithoutTx", "g.id", g.id)
			p := e.peers[g.id]
			if p != nil && !p.knownTxs.Contains(hash) {
				list = append(list, p)
			}
		}
	} else {
		for _, p := range e.peers {
			if !p.knownTxs.Contains(hash) {
				list = append(list, p)
			}
		}
	}
	return list
}

// SendTransactions sends transactions to the peer and includes the hashes
// in its transaction hash set for future reference.
func (p *peer) sendTx(txs []Transaction) {
	for _, tx := range txs {
		if err := p2p.Send(p.ws, Dcrm_msgCode, string(tx.Payload)); err == nil {
			if len(p.queuedTxs) >= maxKnownTxs {
				p.knownTxs.Pop()
			}
			p.knownTxs.Add(tx.hash())
		}
	}
}

// Hash hashes the RLP encoding of tx.
// It uniquely identifies the transaction.
func (tx *Transaction) hash() common.Hash {
	if hash := tx.Hash.Load(); hash != nil {
		return hash.(common.Hash)
	}
	v := rlpHash(tx.Payload)
	tx.Hash.Store(v)
	return v
}

func rlpHash(x interface{}) (h common.Hash) {
	hw := sha3.NewKeccak256()
	rlp.Encode(hw, x)
	hw.Sum(h[:0])
	return h
}

