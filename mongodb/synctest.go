package mongodb

import (
	"fmt"
	//"time"
	//"github.com/syndtr/goleveldb/leveldb"
	//"github.com/davecgh/go-spew/spew"
)

func openLevelDb(path string) {
}

func StartSync(path string) {
	fmt.Printf("==== StartSync() ====\npath '%+v'\n", path)
	//path = "/root/project/testnet/node2/gdcrm/chaindata"
	//levelDb, err := leveldb.OpenFile(path, nil)
	//defer levelDb.Close()
	//if err != nil {
	//	fmt.Printf("ERROR: Cannot open LevelDB from [%s], with error=[%v]\n", path, err)
	//	return
	//}
	//iter := levelDb.NewIterator(nil, nil)
	//for iter.Next() {
	//	key := string(iter.Key())
	//	value := string(iter.Value())
	//	spew.Printf("key is #v,\nvalue is '%#v'\n", key, value)
	//}

	//for {
	//	getPendingTxs()
	//	time.Sleep(time.Duration(1) * time.Second)
	//}
}
