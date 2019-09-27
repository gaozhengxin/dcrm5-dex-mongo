package dao
import (
	"github.com/fusion/go-fusion/cmd/eos_tracker/config"
	"log"
	"github.com/syndtr/goleveldb/leveldb"
)

var dbPath string
var db *leveldb.DB

/*func init () {
	Open()
}*/

func Open () {
	var err error
	dbPath = config.DbPath
	db, err = leveldb.OpenFile(dbPath, nil)
	if err != nil {
		log.Fatal(err)
	}
}

func Close () {
	if r := recover(); r != nil {
		log.Println(r)
	}
	if db != nil {
		db.Close()
	}
}

func Put(key string, value string) {
	e := db.Put([]byte(key), []byte(value), nil)
	if e != nil {
		log.Print(e)
	}
	return
}

func Get(key string) string {
	data, e := db.Get([]byte(key), nil)
	if e != nil {
		if e.Error() == "leveldb: not found" {
			return ""
		}
		log.Print(e)
		return ""
	}
	return string(data)
}

func Delete(key string) {
	e := db.Delete([]byte(key),nil)
	if e != nil {
		log.Print(e)
	}
}
