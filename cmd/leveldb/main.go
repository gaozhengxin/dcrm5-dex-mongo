package main

import (
    "fmt"
    "flag"
    "bytes"
    "math/big"
    "github.com/fusion/go-fusion/rlp"
    //"strings"
    //"encoding/json"
    "github.com/syndtr/goleveldb/leveldb"
    //"github.com/fusion/go-fusion/crypto/dcrm"
)

var (
    channel     string
    chaincode   string
    key         string

    dbpath      string
    sep = "dcrmparm"
    pn int32
    dn int32
    eth int32
)

func init() { 
    flag.StringVar(&channel, "channel", "mychannel", "Channel name") 
    flag.StringVar(&chaincode, "chaincode", "mychaincode", "Chaincode name") 
    flag.StringVar(&key, "key", "", "Key to query; empty query all keys") 
    flag.StringVar(&dbpath, "dbpath", "", "Path to LevelDB") 
    pn = 0
    dn = 0
    eth = 0
}

func readKey(db *leveldb.DB, key string) { 
    var b bytes.Buffer 
    b.WriteString(channel) 
    b.WriteByte(0) 
    b.WriteString(chaincode) 
    b.WriteByte(0) 
    b.WriteString(key) 
    value, err := db.Get(b.Bytes(), nil) 
    if err != nil { 
	fmt.Printf("ERROR: cannot read key[%s], error=[%v]\n", key, err) 
	return 
    } 
    
    fmt.Printf("Key[%s]=[%s]\n", key, string(value)) 
}

/*func readAll(db *leveldb.DB) { 
    var b bytes.Buffer 
    b.WriteString(channel) 
    b.WriteByte(0) 
    b.WriteString(chaincode) 
    //prefix := b.String() 
    iter := db.NewIterator(nil, nil) 
    for iter.Next() { 
	key := string(iter.Key())
	fmt.Printf("======caihaijun,key is %s====\n",key)
	value := string(iter.Value())

	s := strings.Split(value,sep)
	if len(s) != 0 {
	    var m dcrm.AccountListInfo
	    ok := json.Unmarshal([]byte(s[0]), &m)
	    if ok == nil {
		pn++	
	    } else {
		dcrmaddrs := []rune(key)
		if len(dcrmaddrs) == 42 {
		    eth++
		}
		dn++
	    }
	}

	//if strings.HasPrefix(key, prefix) { 
	    //fmt.Printf("Key[%s]=[%s]\n", key, value); 
	//} 
    } 
    
    iter.Release() 
    fmt.Printf("======caihaijun,pubkey num is %d====\n",pn)
    fmt.Printf("======caihaijun,eth addr num is %d====\n",eth)
    fmt.Printf("======caihaijun,dcrm addr num is %d====\n",dn)
    //err := iter.Error() 
}*/

type BBB struct {
    C *big.Int
    S string
}

type CCC struct {
    Ddd *big.Int
    Eee  []*BBB
}

type AAA struct {
    Num *big.Int
    Aa  *BBB
    Ss string
    Ks []*CCC
}

func main() { 
    /*flag.Parse() 
    if channel == "" && chaincode== "" && dbpath == "" { 
	fmt.Printf("ERROR: Neither of channel, chaincode, key nor dbpath could be empty\n") 
	return 
    } 
    fmt.Printf("channel=",channel)   
    fmt.Printf("chaincode=",chaincode)   
    fmt.Printf("dbpath=",dbpath)   
    fmt.Printf("======caihaijun====\n")
    db, err := leveldb.OpenFile(dbpath, nil) 
    if err != nil { 
	fmt.Printf("ERROR: Cannot open LevelDB from [%s], with error=[%v]\n", dbpath, err); 
    } 
    
    defer db.Close() 
    
    fmt.Printf("======caihaijun11111====\n")
    if key == "" { 
	fmt.Printf("======caihaijun222222====\n")
	readAll(db) 
    } else { 
	fmt.Printf("======caihaijun3333333====\n")
	readKey(db, key) 
    } */

    num,_ := new(big.Int).SetString("1234",10)
    c1,_ := new(big.Int).SetString("0xdc",0)
    bbb1 := BBB{C:c1,S:"test1"}
    c2,_ := new(big.Int).SetString("0xa",0)
    bbb2 := BBB{C:c2,S:"test2"}
    c3,_ := new(big.Int).SetString("0xgc",0)
    bbb3 := BBB{C:c3,S:"test3"}
    c4,_ := new(big.Int).SetString("107",0)
    aa := BBB{C:c4,S:"test4"}
    ss := "aaaaaa"
    ddd,_ := new(big.Int).SetString("0xdd",0)
    tmp := make([]*BBB,0)
    tmp = append(tmp,&bbb1)
    tmp = append(tmp,&bbb2)
    tmp = append(tmp,&bbb3)
    ks1 := CCC{Ddd:ddd,Eee:tmp}
    ks2 := CCC{Ddd:ddd,Eee:tmp}
    t := make([]*CCC,0)
    t = append(t,&ks1)
    t = append(t,&ks2)
    aaa := AAA{Num:num,Aa:&aa,Ss:ss,Ks:t}
    msg, err := rlp.EncodeToBytes(&aaa)
    if err != nil {
	fmt.Printf("=======error.========\n")
    }

    var M AAA
    if err = rlp.DecodeBytes([]byte(msg), &M); err == nil {
	fmt.Printf("===M.Num=%v,M.Aa.C=%v,M.Ss=%s,(M.Ks[0].Eee)[1].C=%v,M.Ks[1].Ddd=%v=========\n",M.Num,M.Aa.C,M.Ss,(M.Ks[0].Eee)[1].C,M.Ks[1].Ddd)
    }

}








