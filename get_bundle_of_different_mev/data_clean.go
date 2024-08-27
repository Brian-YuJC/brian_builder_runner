package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/triedb"
)

func check(e error) {
	if e != nil {
		panic(e)
	}
}

// 用于解析JSON文件
type MEVTx struct {
	Block_number              uint64    `json:"block_number"`
	Tx_index                  int       `json:"tx_index"`
	Mev_type                  string    `json:"mev_type"`
	Protocol                  string    `json:"protocol"`
	User_loss_usd             float64   `json:"user_loss_usd"`
	Extractor_profit_usd      float64   `json:"extractor_profit_usd"`
	User_swap_volume_usd      float64   `json:"user_swap_volume_usd"`
	User_swap_count           int       `json:"user_swap_count"`
	Extractor_swap_volume_usd float64   `json:"extractor_swap_volume_usd"`
	Extractor_swap_count      int       `json:"extractor_swap_count"`
	Imbalance                 float64   `json:"imbalance"`
	Address_from              string    `json:"address_from"`
	Address_to                string    `json:"address_to"`
	Arrival_time_us           time.Time `json:"arrival_time_us"`
}

// 提取我们需要的数据
type CleanMEVTx struct {
	BlockNum         uint64
	Hash             common.Hash
	MEVType          string
	Protocol         string
	UserSwapCnt      int
	ExtractorSwapCnt int
	From             common.Address
	To               common.Address
}

func GetDatabase() ethdb.Database {
	datadir := "/home/user/common/docker/volumes/eth-docker_geth-eth1-data/_data/geth/chaindata"
	// datadir := "/home/user/data/ben/cp1_eth-docker_geth-eth1-data/_data/geth/chaindata"
	ancient := datadir + "/ancient"
	db, err := rawdb.Open(
		rawdb.OpenOptions{
			Directory:         datadir,
			AncientsDirectory: ancient,
			Ephemeral:         true,
		},
	)
	if err != nil {
		fmt.Println("rawdb.Open err!", err)
	}

	//维护全局变量
	CHAIN_CONFIG = GetChainConfig(db) //用于获取Tx 的sender 也就是from

	return db
}

var CHAIN_CONFIG *params.ChainConfig

// 获取chain_config
func GetChainConfig(db ethdb.Database) *params.ChainConfig {
	triedb := triedb.NewDatabase(db, &triedb.Config{})
	chainConfig, _, _ := core.SetupGenesisBlockWithOverride(db, triedb, nil, nil)
	return chainConfig
}

// 根据区块和tx以及chain_config获取Tx from地址
func GetTxFrom(chain_config *params.ChainConfig, block *types.Block, tx *types.Transaction) common.Address {
	s := types.MakeSigner(chain_config, block.Header().Number, block.Header().Time)
	from, err := types.Sender(s, tx)
	check(err)
	return from
}

// 根据块号、tx索引返回tx有关信息
func GetTxHashFromTo(db ethdb.Database, block_num uint64, index int) (common.Hash, common.Address, common.Address) {
	block_hash := rawdb.ReadCanonicalHash(db, block_num)
	block := rawdb.ReadBlock(db, block_hash, block_num)
	target_tx := block.Transactions()[index]

	hash := target_tx.Hash()
	from := GetTxFrom(CHAIN_CONFIG, block, target_tx)
	to := target_tx.To()
	if to == nil { //特殊情况处理，to为nil的交易是创建合约的交易
		fmt.Println("Nil To. Block_num:", block_num, "tx hash:", hash)
		var empty_to common.Address
		empty_to_byte, err := hex.DecodeString("0000000000000000000000000000000000000000") //这里直接把nil赋值为0
		check(err)
		empty_to.SetBytes(empty_to_byte)
		fmt.Println(empty_to)
		return hash, from, empty_to
	}
	return hash, from, *to
}

func GetCleanMEVTx(db ethdb.Database) {
	for i, mev_tx := range MEVTX_LIST {
		var tmp = CleanMEVTx{}
		tmp.BlockNum = mev_tx.Block_number
		hash, from, to := GetTxHashFromTo(db, tmp.BlockNum, mev_tx.Tx_index)
		tmp.Hash = hash
		tmp.MEVType = mev_tx.Mev_type
		tmp.Protocol = mev_tx.Protocol
		tmp.UserSwapCnt = mev_tx.User_swap_count
		tmp.ExtractorSwapCnt = mev_tx.Extractor_swap_count
		// var from, to common.Address
		// fmt.Println(tmp.BlockNum)
		// from_byte, err := hex.DecodeString(mev_tx.Address_from[2:]) //类型转换
		// check(err)
		// to_byte, err := hex.DecodeString(mev_tx.Address_to[2:]) //类型转换
		// from.SetBytes(from_byte)
		// to.SetBytes(to_byte) //
		// _, from, to = GetTxHashFromTo(db, tmp.BlockNum, mev_tx.Tx_index)
		tmp.From = from
		tmp.To = to
		CLEANMEVTX_LIST = append(CLEANMEVTX_LIST, tmp) //维护全局变量
		fmt.Println("Finish: ", i)
	}
}

var INPUT_PATH string = "./output/"
var OUTPUT_PATH string = "./output/"

var MEVTX_LIST []MEVTx
var CLEANMEVTX_LIST []CleanMEVTx

func main() {

	file_name := os.Args[1]

	//读取文件到DATA
	input, err := os.Open(INPUT_PATH + file_name + ".json")
	check(err)
	defer input.Close()
	// 读取文件内容
	input_bytes, err := ioutil.ReadAll(input)
	check(err)
	err = json.Unmarshal([]byte(input_bytes), &MEVTX_LIST)
	check(err)
	fmt.Println("MEVTX_LIST length:", len(MEVTX_LIST))

	GetCleanMEVTx(GetDatabase())

	// 输出成json格式
	jsonData, err := json.MarshalIndent(CLEANMEVTX_LIST, "", "  ")
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	//fmt.Println(string(jsonData))
	output, err := os.Create(OUTPUT_PATH + file_name + "_clean.json")
	check(err)
	output.Write(jsonData)

}

//go run get_bundle_touched_address.go 19700000_20200000_bundle.json >log
