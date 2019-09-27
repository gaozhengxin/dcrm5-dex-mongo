package dcrm

import (
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
	"time"
	"github.com/fusion/go-fusion/crypto/dcrm/cryptocoins"
	"github.com/fusion/go-fusion/crypto/dcrm/cryptocoins/config"
	"github.com/fusion/go-fusion/crypto/dcrm/cryptocoins/eos"
	"github.com/fusion/go-fusion/crypto/dcrm/cryptocoins/rpcutils"
	"github.com/fusion/go-fusion/log"
	"github.com/fusion/go-fusion/ethdb"
)

func CreateRealEosAccount(accountName string, ownerkey string, activekey string) error {
	log.Debug("==========create eos account Start!!==========","account name",accountName,"ownerkey",ownerkey,"activekey",activekey)
	av := cryptocoins.NewAddressValidator("EOS_NORMAL")
	if !av.IsValidAddress(accountName) {
		return errors.New("eos account name format error")
	}
	owner, e1 := eos.HexToPubKey(ownerkey)
	active, e2 := eos.HexToPubKey(activekey)
	if e1 != nil || e2 != nil {
		log.Debug("==========create eos account ==========","ownerkey error",e1,"active error",e2)
		return errors.New("cannot convert to eos pubkey format.")
	}
	log.Debug("==========create eos account==========","owner",owner,"active",active)
///*
	ojbk, err := eos.CreateNewAccount(eos.CREATOR_ACCOUNT,eos.CREATOR_PRIVKEY,accountName,owner.String(),active.String(),eos.InitialRam)
	if ojbk == false || err != nil {
		log.Debug("create eos account failed","error",err)
		return errors.New("create eos account failed")
	}
	ojbk2, err := eos.DelegateBW(eos.CREATOR_ACCOUNT,eos.CREATOR_PRIVKEY,accountName,eos.InitialCPU,eos.InitialStakeNet,true)
	if ojbk2 == false || err != nil {
		log.Debug("delegate cpu and net failed","error",err)
		return errors.New("delegate cpu and net failed")
	}

	return err
//*/
//	return nil
}

var trytimes = 50

func CheckRealEosAccount(accountName, ownerkey, activekey string) (ok bool) {
	ok = false
	defer func () {
		if r := recover(); r != nil {
			log.Debug("check eos account","error",r)
		}
	}()
	log.Debug("==========check eos account Start!!==========")
	// 1. check if account exists
	api := "v1/chain/get_account"
	data := `{"account_name":"` + accountName + `"}`
	var ret string
	info := new(AccountInfo)
	var err error
	for i := 0; i < trytimes; i++ {
		ret = rpcutils.DoCurlRequest(config.ApiGateways.EosGateway.Nodeos, api, data)
		log.Debug("========check eos account========","ret",ret)
		err = json.Unmarshal([]byte(ret), info)
		if err != nil {
			log.Debug("========check eos account========","decode error",err)
			time.Sleep(time.Duration(1)*time.Second)
			continue
		}
		break
	}
	if err != nil {
		log.Debug("decode account info error")
		return false
	}
	if info.Error != nil || info.AccountName != accountName {
		log.Debug("check real eos account error", "error", info.Error)
		return false
	}
	// 2. check owner key
	// 3. check active key
	// 4. check no other keys authorized

	owner, e1 := eos.HexToPubKey(ownerkey)  //EOS8JXJf7nuBEs8dZ8Pc5NpS8BJJLt6bMAmthWHE8CSqzX4VEFKtq
	active, e2 := eos.HexToPubKey(activekey)
	if e1 != nil || e2 != nil {
		log.Debug("public key error","owner key error", e1, "active key error", e2)
		return false
	}

	perm := info.Perms
	objPerm := Permissions([]Permission{
		Permission{PermName:"owner",Parent:"",RequiredAuth:Auth{Threshold:1,Keys:[]Key{Key{Key:owner.String(),Weight:1}}}},
		Permission{PermName:"active",Parent:"owner",RequiredAuth:Auth{Threshold:1,Keys:[]Key{Key{Key:active.String(),Weight:1}}}},
	})
	sort.Sort(objPerm)
	sort.Sort(perm)
	if reflect.DeepEqual(perm, objPerm) == false {
		log.Debug("account permissions not match","have",perm,"required",objPerm)
		return false
	}
	// 5. enough ram cpu net
	if info.RamQuato - info.RamUsage < eos.InitialRam / 2 {
		log.Debug("account ram is too low")
		return false
	}
	if int64(info.CpuLimit.Max) < eos.InitialCPU * 5 {
		log.Debug("account cpu is too low")
		return false
	}
	if int64(info.NetLimit.Max) < eos.InitialStakeNet * 5 {
		log.Debug("account net bandwidth is too low")
		return false
	}
	log.Debug("==========check eos account Success!!==========")
	return true
}

type AccountInfo struct {
	AccountName string                 `json:"account_name"`
	RamQuato    uint32                 `json:"ram_quato"`
	NetWeight   uint32                 `json:"net_weight"`
	CpuWeight   uint32                 `json:"cpu_weight"`
	NetLimit    Limit                  `json:"net_limit"`
	CpuLimit    Limit                  `json:"cpu_limit"`
	RamUsage    uint32                 `json:"ram_usage"`
	Perms       Permissions            `json:"permissions"`
	Error       map[string]interface{} `json:"error"`
}

type Permissions []Permission

func (p Permissions) Len() int {
	return len([]Permission(p))
}

func (p Permissions) Less(i, j int) bool {
	if []Permission(p)[i].PermName == "owner" {
		return true
	}
	return false
}

func (p Permissions) Swap(i, j int) {
	tmp := []Permission(p)[i]
	[]Permission(p)[i] = []Permission(p)[j]
	[]Permission(p)[j] = tmp
}

type Permission struct {
	PermName     string `json:"perm_name"`
	Parent       string `json:"parent"`
	RequiredAuth Auth   `json:"required_auth"`
}

type Auth struct {
	Threshold int    `json:"threshold"`
	Keys       []Key `json:"keys"`
}

type Key struct {
	Key    string `json:"key"`
	Weight int    `json:"weight"`
}

type Limit struct {
	Used      int64 `used`
	Available int64 `available`
	Max       int64 `max`
}

func GetEosAccount() (acct, owner, active string) {
	lock.Lock()
	dir := GetEosDbDir()
	db,_ := ethdb.NewLDBDatabase(dir, 0, 0)
	if db == nil {
		log.Debug("==============open db fail.============")
		lock.Unlock()
		return "", "", ""
	}
	data, _ := db.Get([]byte("eossettings"))
	datas := strings.Split(string(data),":")
	log.Debug("======== GetEosAccount","datas",datas,"","========")
	if len(datas) == 5 && datas[0] == "EOS_INITIATE" {
		db.Close()
		lock.Unlock()
		return datas[1], datas[2], datas[3]
	}
	db.Close()
	lock.Unlock()
	return "", "", ""
}
