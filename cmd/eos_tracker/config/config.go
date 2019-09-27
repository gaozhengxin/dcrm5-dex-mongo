package config

var EOS_ACCOUNT string
const NODEOS="https://api-kylin.eoslaomao.com"

var DbPath string

func SetEosBase (eb string) {
	EOS_ACCOUNT = eb
}

func SetDbPath (dbpath string) {
	DbPath = dbpath
}
