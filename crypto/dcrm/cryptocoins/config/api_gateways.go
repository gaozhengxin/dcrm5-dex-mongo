// 提供api的节点地址
package config

import (
	"github.com/BurntSushi/toml"
	"github.com/fusion/go-fusion/log"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"github.com/astaxie/beego/logs"
	"encoding/json"
)

type SimpleApiConfig struct {
	ApiAddress string
}

type RpcClientConfig struct {
	ElectrsAddress string
	Host string
	Port int
	User string
	Passwd string
	Usessl bool
}

type EosConfig struct {
	Nodeos string
	ChainID string
	BalanceTracker string
}

type ApiGatewayConfigs struct {
	RPCCLIENT_TIMEOUT int
	CosmosGateway *SimpleApiConfig
	TronGateway *SimpleApiConfig
	BitcoinGateway *RpcClientConfig
	OmniGateway *RpcClientConfig
	BitcoincashGateway *RpcClientConfig
	EthereumGateway *SimpleApiConfig
	EosGateway *EosConfig
	EvtGateway *SimpleApiConfig
	RippleGateway *SimpleApiConfig
}

var ApiGateways *ApiGatewayConfigs

const RPCCLIENT_TIMEOUT = 30

var datadir string

func SetDatadir(data string) {
	datadir = data
}

var Loaded bool = false

/*func init() {
	if err := LoadApiGateways(); err != nil {
		log.Error(err.Error())
	}
}*/

func Init () {
    err := LoadApiGateways()
    if err != nil {
	    log.Error(err.Error())
    }
}

func PrintLogToFile() {
    config:=make(map[string]interface{})
    config["fileName"]="/work/logcoolect.log" 
    //输出文件路径,不存在  默认创建
    config["level"]=logs.LevelDebug
    //设置日志级别
    configStr,err:=json.Marshal(config)
    if err != nil {
	    panic(err)
	    return
    }

    logs.SetLogger("file",string(configStr))
}

func LoadApiGateways () error {
	if datadir == "" {
		datadir = DefaultDataDir()
	}
	PrintLogToFile()
	logs.Info("!!!!!!!!LoadApiGateways!!!!!!!!", "config dir", datadir)
	if ApiGateways == nil {
		ApiGateways = new(ApiGatewayConfigs)
	}
	logs.Debug("========LoadApiGateways===========","ApiGateways",ApiGateways)

	configfilepath := filepath.Join(datadir, "gateways.toml")

	logs.Debug("========LoadApiGateways===========","configfilepath",configfilepath)

	if exists, _ := PathExists(configfilepath); exists {
		logs.Debug("========LoadApiGateways,exist===========")
		_, err := toml.DecodeFile(configfilepath, ApiGateways)
		if err == nil {
			logs.Debug("========LoadApiGateways,toml decodefile===========","ApiGateways",ApiGateways)
			Loaded = true
		}
		logs.Debug("========LoadApiGateways,toml decodefile===========","ApiGateways",ApiGateways,"err",err)
		return err
	} else {
		toml.Decode(defaultConfig, ApiGateways)
		logs.Debug("========LoadApiGateways,toml decode===========","ApiGateways",ApiGateways,"defaultConfig",defaultConfig)
		f, e1 := os.Create(configfilepath)
		if f == nil {
			logs.Debug("cannot create config file.","error",e1)
			return nil
		}
		_, e2 := io.WriteString(f, defaultConfig)
		if e2 != nil {
			logs.Debug("write config file error.", "error", e2)
			return nil
		}
		Loaded = true
	}

	return nil
}

func PathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// DefaultDataDir is the default data directory to use for the databases and other
// persistence requirements.
func DefaultDataDir() string {
	// Try to place the data folder in the user's home dir
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

func homeDir() string {
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	if usr, err := user.Current(); err == nil {
		return usr.HomeDir
	}
	return ""
}


































// ===================== OLD CONFIGS =========================
/*
// eth rinkeby testnet
const (
	ETH_GATEWAY = "http://54.183.185.30:8018"
)

// eos kylincrypto testnet
const (
	//eos api nodes support get actions (filter-on=*)
	EOS_NODEOS = "https://api.kylin.alohaeos.com"
	EOS_CHAIN_ID = "5fff1dae8dc8e2fc4d5b23b2c7665c97f9e9d8edf2b6485a86ba311c25639191"  // cryptokylin test net
	BALANCE_SERVER = "http://127.0.0.1:7000/"
)

// ripple testnet
const (
	XRP_GATEWAY = "https://s.altnet.rippletest.net:51234"
)

// cosmos atom cosmoshub-2
var CosmosHost = "https://stargate.cosmos.network"

// tron testnet
const (
	TRON_SOLIDITY_NODE_HTTP = "https://api.shasta.trongrid.io"
)

// bitcoin testnet
const (
	ELECTRSHOST            = "http://5.189.139.168:4000"
	BTC_SERVER_HOST        = "47.107.50.83"
	BTC_SERVER_PORT        = 8000
	BTC_USER               = "xxmm"
	BTC_PASSWD             = "123456"
	BTC_USESSL             = false
)

const (
	OMNI_SERVER_HOST        = "5.189.139.168"
	OMNI_SERVER_PORT        = 9772
	OMNI_USER               = "xxmm"
	OMNI_PASSWD             = "123456"
	OMNI_USESSL             = false
)

// bitcoin cash
const (
	BCH_SERVER_HOST        = "5.189.139.168"
	BCH_SERVER_PORT        = 9552
	BCH_USER               = "xxmm"
	BCH_PASSWD             = "123456"
	BCH_USESSL             = false
)
*/
