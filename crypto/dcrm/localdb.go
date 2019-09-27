// Copyright 2019 The fusion-dcrm 
//Author: caihaijun@fusion.org

package dcrm

import(
	"strings"
	"errors"
	"runtime"
	"path/filepath"
	"os"
	"os/user"
	"sync"
	"github.com/fusion/go-fusion/ethdb"
	"github.com/fusion/go-fusion/crypto"
	"github.com/fusion/go-fusion/log"
)

var lock2 sync.Mutex

func GetDbDir() string {
    if datadir != "" {
	return datadir+"/dcrmdata/dcrmdb"
    }

    dir := DefaultDataDir()
    log.Debug("==========GetDbDir,","datadir",dir,"","===========")
    dir += "/dcrmdata/dcrmdb"
    return dir
}

func SetDatadir (data string) {
    datadir = data
}

//========================================================

//eos_init---> eos account
//key: crypto.Keccak256Hash([]byte("eossettings"))
//value: pubkey+eos account
func GetEosDbDir() string {
    if datadir != "" {
	return datadir+"/dcrmdata/eosdb"
    }

    dir := DefaultDataDir()
    log.Debug("==========GetEosDbDir,","datadir",dir,"","===========")
    dir += "/dcrmdata/eosdb"
    return dir
}

//===========================================================

func homeDir() string {
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	if usr, err := user.Current(); err == nil {
		return usr.HomeDir
	}
	return ""
}

func DefaultDataDir() string {
	home := homeDir()
	if home != "" {
		if runtime.GOOS == "darwin" {
			return filepath.Join(home, "Library", "Fusion")
		} else if runtime.GOOS == "windows" {
			return filepath.Join(home, "AppData", "Roaming", "Fusion")
		} else {
			return filepath.Join(home, ".fusion")
		}
	}
	// As we cannot guess a stable location, return empty and handle later
	return ""
}

//==========================================================
//all lockout tx status

//all lockout tx status
func GetDbDirForLockoutStatus() string {

    if datadir != "" {
	return datadir+"/LockOutTxStatus"
    }

    s := DefaultDataDir()
    log.Debug("==========GetDbDirForLockoutStatus,","datadir",s,"","===========")
    s += "/LockOutTxStatus"
    return s
}

//key: lockout tx hash
//value: {Txid:xxx,Status:xxxxx,error:xxxxx,BlockHeight:xxxxx,RealFusionAccount:xxxxx,RealDcrmAccount:xxxxx,Cointype:xxxxx,OutSideTxHash:xxxxxx,TimeStamp:xxxxxx}
func GetLockoutTxStatusFromLocalDB(hash string) (string,error) {
    if hash == "" {
	return "",errors.New("param error get lockout tx status from local db by hash.")
    }
    
    lock2.Lock()
    path := GetDbDirForLockoutStatus()
    db,_ := ethdb.NewLDBDatabase(path, 0, 0)
    if db == nil {
	log.Debug("==============GetLockoutTxStatusFromLocalDB,create db fail.============")
	lock2.Unlock()
	return "",errors.New("create db fail.")
    }
    
    value,has:= db.Get([]byte(hash))
    if string(value) != "" && has == nil {
	db.Close()
	lock2.Unlock()
	return string(value),nil
    }

    db.Close()
    lock2.Unlock()
    return "",nil
}

func WriteLockoutTxStatusToLocalDB(hash string,value string) (bool,error) {

    if hash == "" || value == "" {
	return false,errors.New("param error in write lockout tx status to local db.")
    }

    lock2.Lock()
    path := GetDbDirForLockoutStatus()
    db,_ := ethdb.NewLDBDatabase(path, 0, 0)
    if db == nil {
	log.Debug("==============WriteLockoutTxStatusToLocalDB,create db fail.============")
	lock2.Unlock()
	return false,errors.New("create db fail.")
    }
    
    db.Put([]byte(hash),[]byte(value))
    db.Close()
    lock2.Unlock()
    return true,nil
}

//==========================================================

//lockout real fee

//lockout real fee path
func GetDbDirForLockoutRealFee() string {

    if datadir != "" {
	return datadir+"/dcrmdata/lofee"
    }

    s := DefaultDataDir()
    log.Debug("==========GetDbDirForLockoutRealFee,","datadir",s,"","===========")
    s += "/dcrmdata/lofee"
    return s
}

//key: lockout tx hash
//value: real fee
func GetLockoutRealFeeFromLocalDB(hash string) (string,error) {
    if hash == "" {
	return "",errors.New("param error get lockout real fee from local db by hash.")
    }
    
    lock2.Lock()
    path := GetDbDirForLockoutRealFee()
    db,_ := ethdb.NewLDBDatabase(path, 0, 0)
    if db == nil {
	log.Debug("==============GetLockoutRealFeeFromLocalDB,create db fail.============")
	lock2.Unlock()
	return "",errors.New("create db fail.")
    }
    
    value,has:= db.Get([]byte(hash))
    if string(value) != "" && has == nil {
	db.Close()
	lock2.Unlock()
	return string(value),nil
    }

    db.Close()
    lock2.Unlock()
    return "",nil
}

func WriteLockoutRealFeeToLocalDB(hash string,value string) (bool,error) {

    if hash == "" || value == "" {
	return false,errors.New("param error in write lockout info to local db.")
    }

    lock2.Lock()
    path := GetDbDirForLockoutRealFee()
    db,_ := ethdb.NewLDBDatabase(path, 0, 0)
    if db == nil {
	log.Debug("==============WriteLockoutRealFeeToLocalDB,create db fail.============")
	lock2.Unlock()
	return false,errors.New("create db fail.")
    }
    
    db.Put([]byte(hash),[]byte(value))
    db.Close()
    lock2.Unlock()
    return true,nil
}

//==========================================================

//lockout info

//lockout info save path 
func GetDbDirForLockoutInfo() string {

    if datadir != "" {
	return datadir+"/dcrmdata/lockoutinfo"
    }

    s := DefaultDataDir()
    log.Debug("==========GetDbDirForLockoutInfo,","datadir",s,"","===========")
    s += "/dcrmdata/lockoutinfo"
    return s
}

//key: lockout tx hash
//value: lockout_txhash + sep10 + realfusionfrom + sep10 + realdcrmfrom
func GetLockoutInfoFromLocalDB(hash string) (string,error) {
    if hash == "" {
	return "",errors.New("param error get lockout info from local db by hash.")
    }
    
    lock2.Lock()
    path := GetDbDirForLockoutInfo()
    db,_ := ethdb.NewLDBDatabase(path, 0, 0)
    if db == nil {
	log.Debug("==============GetLockoutInfoFromLocalDB,create db fail.============")
	lock2.Unlock()
	return "",errors.New("create db fail.")
    }
    
    value,has:= db.Get([]byte(hash))
    if string(value) != "" && has == nil {
	db.Close()
	lock2.Unlock()
	return string(value),nil
    }

    db.Close()
    lock2.Unlock()
    return "",nil
}

func WriteLockoutInfoToLocalDB(hash string,value string) (bool,error) {
    //if !IsInGroup() {
//	return false,errors.New("it is not in group.")
  //  }

    if hash == "" || value == "" {
	return false,errors.New("param error in write lockout info to local db.")
    }

    lock2.Lock()
    path := GetDbDirForLockoutInfo()
    db,_ := ethdb.NewLDBDatabase(path, 0, 0)
    if db == nil {
	log.Debug("==============WriteLockoutInfoToLocalDB,create db fail.============")
	lock2.Unlock()
	return false,errors.New("create db fail.")
    }
    
    db.Put([]byte(hash),[]byte(value))
    db.Close()
    lock2.Unlock()
    return true,nil
}

//==========================================================

//DcrmAddr

//dcrmaddr save path
func GetDbDirForWriteDcrmAddr() string {

    if datadir != "" {
	return datadir+"/dcrmdata/dcrmaddrs"
    }

    s := DefaultDataDir()
    log.Debug("==========GetDbDirForWriteDcrmAddr,","datadir",s,"","===========")
    s += "/dcrmdata/dcrmaddrs"
    return s
}

//key:  kec256(fusionaddr:cointype)
//value: dcrmaddr1:dcrmaddr2:dcrmaddr3............
func ReadDcrmAddrFromLocalDBByIndex(fusion string,cointype string,index int) (string,error) {

    if fusion == "" || cointype == "" || index < 0 {
	return "",errors.New("param error.")
    }

    lock2.Lock()
    path := GetDbDirForWriteDcrmAddr()
    db,_ := ethdb.NewLDBDatabase(path, 0, 0)
    if db == nil {
	log.Debug("==============ReadDcrmAddrFromLocalDBByIndex,create db fail.============")
	lock2.Unlock()
	return "",errors.New("create db fail.")
    }
    
    hash := crypto.Keccak256Hash([]byte(strings.ToLower(fusion) + ":" + strings.ToLower(cointype))).Hex()
    value,has:= db.Get([]byte(hash))
    if string(value) != "" && has == nil {
	    v := strings.Split(string(value),":")
	    if len(v) < (index + 1) {
		db.Close()
		lock2.Unlock()
		return "",errors.New("has not dcrmaddr in local DB.")
	    }

	    db.Close()
	    lock2.Unlock()
	    return v[index],nil
    }
	db.Close()
	lock2.Unlock()
	return "",errors.New("has not dcrmaddr in local DB.")
}

func IsFusionAccountExsitDcrmAddr(fusion string,cointype string,dcrmaddr string) (bool,string,error) {
    if fusion == "" || cointype == "" {
	return false,"",errors.New("param error")
    }
    
    lock2.Lock()
    path := GetDbDirForWriteDcrmAddr()
    db,_ := ethdb.NewLDBDatabase(path, 0, 0)
    if db == nil {
	log.Debug("==============IsFusionAccountExsitDcrmAddr,create db fail.============")
	lock2.Unlock()
	return false,"",errors.New("create db fail.")
    }
    
    hash := crypto.Keccak256Hash([]byte(strings.ToLower(fusion) + ":" + strings.ToLower(cointype))).Hex()
    if dcrmaddr == "" {
	has,_ := db.Has([]byte(hash))
	if has == true {
		log.Debug("========IsFusionAccountExsitDcrmAddr,has req dcrmaddr.==============")
		value,_:= db.Get([]byte(hash))
		v := strings.Split(string(value),":")
		db.Close()
		lock2.Unlock()
		return true,string(v[0]),nil
	}

	log.Debug("========IsFusionAccountExsitDcrmAddr,has not req dcrmaddr.==============")
	db.Close()
	lock2.Unlock()
	return false,"",nil
    }
    
    value,has:= db.Get([]byte(hash))
    log.Debug("========IsFusionAccountExsitDcrmAddr,==============","hash",hash,"value",value,"fusionaddr",fusion,"cointype",cointype)
    if has == nil && string(value) != "" {
	v := strings.Split(string(value),":")
	if len(v) < 1 {
	    log.Debug("========IsFusionAccountExsitDcrmAddr,data error.==============")
	    db.Close()
	    lock2.Unlock()
	    return false,"",errors.New("data error.")
	}

	for _,item := range v {
	    if strings.EqualFold(item,dcrmaddr) {
		log.Debug("========IsFusionAccountExsitDcrmAddr,success get dcrmaddr.==============")
		db.Close()
		lock2.Unlock()
		return true,dcrmaddr,nil
	    }
	}
    }
   
    log.Debug("========IsFusionAccountExsitDcrmAddr,fail get dcrmaddr.==============")
    db.Close()
    lock2.Unlock()
    return false,"",nil
}

func WriteDcrmAddrToLocalDB(fusion string,cointype string,dcrmaddr string) (bool,error) {
    if !IsInGroup() {
	log.Debug("WriteDcrmAddrToLocalDB,111111111111111111")
	return false,errors.New("it is not in group.")
    }

    if fusion == "" || cointype == "" || dcrmaddr == "" {
	log.Debug("WriteDcrmAddrToLocalDB,2222222222222222222")
	return false,errors.New("param error in write dcrmaddr to local db.")
    }

    lock2.Lock()
    path := GetDbDirForWriteDcrmAddr()
    db,_ := ethdb.NewLDBDatabase(path, 0, 0)
    if db == nil {
	log.Debug("==============WriteDcrmAddrToLocalDB,create db fail.============")
	lock2.Unlock()
	return false,errors.New("create db fail.")
    }
    
    hash := crypto.Keccak256Hash([]byte(strings.ToLower(fusion) + ":" + strings.ToLower(cointype))).Hex()
    has,_ := db.Has([]byte(hash))
    if has != true {
	log.Debug("WriteDcrmAddrToLocalDB,333333333333333333")
	db.Put([]byte(hash),[]byte(dcrmaddr))
	db.Close()
	lock2.Unlock()
	return true,nil
    }
    
    value,_:= db.Get([]byte(hash))
    v := string(value)
    v += ":"
    v += dcrmaddr
    db.Put([]byte(hash),[]byte(v))
    db.Close()
    lock2.Unlock()
    return true,nil
}

//=======================================================

//for node info save
/*func GetDbDirForNodeInfoSave() string {

    if datadir != "" {
	return datadir+"/nodeinfo"
    }

    s := DefaultDataDir()
    log.Debug("==========GetDbDirForNodeInfoSave,","datadir",s,"","===========")
    s += "/nodeinfo"
    return s
}

func ReadNodeInfoFromLocalDB(nodeinfo string) (string,error) {

    if nodeinfo == "" {
	return "",errors.New("param error in read nodeinfo from local db.")
    }

    lock3.Lock()
    path := GetDbDirForNodeInfoSave()
    db,_ := ethdb.NewLDBDatabase(path, 0, 0)
    if db == nil {
	log.Debug("==============ReadNodeInfoFromLocalDB,create db fail.============")
	lock3.Unlock()
	return "",errors.New("create db fail.")
    }
    
    value,has:= db.Get([]byte(nodeinfo))
    if string(value) != "" && has == nil {
	    db.Close()
	    lock3.Unlock()
	    return string(value),nil
    }
	db.Close()
	lock3.Unlock()
	return "",errors.New("has not nodeinfo in local DB.")
}

func IsNodeInfoExsitInLocalDB(nodeinfo string) (bool,error) {
    if nodeinfo == "" {
	return false,errors.New("param error in check local db by nodeinfo.")
    }
    
    lock3.Lock()
    path := GetDbDirForNodeInfoSave()
    db,_ := ethdb.NewLDBDatabase(path, 0, 0)
    if db == nil {
	log.Debug("==============IsNodeInfoExsitInLocalDB,create db fail.============")
	lock3.Unlock()
	return false,errors.New("create db fail.")
    }
    
    has,_ := db.Has([]byte(nodeinfo))
    if has == true {
	    db.Close()
	    lock3.Unlock()
	    return true,nil
    }

    db.Close()
    lock3.Unlock()
    return false,nil
}

func WriteNodeInfoToLocalDB(nodeinfo string,value string) (bool,error) {
    if !IsInGroup() {
	return false,errors.New("it is not in group.")
    }

    if nodeinfo == "" || value == "" {
	return false,errors.New("param error in write nodeinfo to local db.")
    }

    lock3.Lock()
    path := GetDbDirForNodeInfoSave()
    db,_ := ethdb.NewLDBDatabase(path, 0, 0)
    if db == nil {
	log.Debug("==============WriteNodeInfoToLocalDB,create db fail.============")
	lock3.Unlock()
	return false,errors.New("create db fail.")
    }
    
    db.Put([]byte(nodeinfo),[]byte(value))
    db.Close()
    lock3.Unlock()
    return true,nil
}*/

//===========================================================

//lockin hashkey

//lockin hashkey save path
/*func GetDbDirForLockin() string {
    if datadir != "" {
	return datadir+"/hashkeydb"
    }

    ss := []string{"dir",cur_enode}
    dir = strings.Join(ss,"-")
    dir += "-"
    dir += "hashkeydb"
    return dir
}

func IsHashkeyExsitInLocalDB(hashkey string) (bool,error) {
    if hashkey == "" {
	return false,errors.New("param error in check local db by hashkey.")
    }
    
    lock2.Lock()
    path := GetDbDirForLockin()
    db,_ := ethdb.NewLDBDatabase(path, 0, 0)
    if db == nil {
	log.Debug("==============IsHashkeyExsitInLocalDB,create db fail.============")
	lock2.Unlock()
	return false,errors.New("create db fail.")
    }
    
    has,_ := db.Has([]byte(hashkey))
    if has == true {
	    db.Close()
	    lock2.Unlock()
	    return true,nil
    }

	db.Close()
	lock2.Unlock()
	return false,nil
}

func WriteHashkeyToLocalDB(hashkey string,value string) (bool,error) {
    if !IsInGroup() {
	return false,errors.New("it is not in group.")
    }

    if hashkey == "" || value == "" {
	return false,errors.New("param error in write hashkey to local db.")
    }

    lock2.Lock()
    path := GetDbDirForLockin()
    db,_ := ethdb.NewLDBDatabase(path, 0, 0)
    if db == nil {
	log.Debug("==============WriteHashkeyToLocalDB,create db fail.============")
	lock2.Unlock()
	return false,errors.New("create db fail.")
    }
    
    db.Put([]byte(hashkey),[]byte(value))
    db.Close()
    lock2.Unlock()
    return true,nil
}
*/
