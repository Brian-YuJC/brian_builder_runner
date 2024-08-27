package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/prefetch"
)

func check(e error) {
	if e != nil {
		panic(e)
	}
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

// 单笔arbitrage交易
type Arbitrage struct {
	Tx              CleanMEVTx `json:"Tx"`
	TouchAddressMap map[common.Address]TouchAddress
}

type TouchAddress struct {
	//Address   common.Address
	InvokeCnt uint64
	KeyMap    map[common.Hash]int
}

// 读取disk中的数据库并建立区块链
var CustomConfig = &core.CacheConfig{
	TrieCleanLimit: 4096,
	TrieDirtyLimit: 4096,
	TrieTimeLimit:  5 * time.Minute,
	SnapshotLimit:  4096,
	SnapshotWait:   true,
	StateScheme:    rawdb.HashScheme,
}

func GetBlockChain() (ethdb.Database, *core.BlockChain) {
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
	bc, _ := core.NewBlockChain(db, CustomConfig, nil, nil, ethash.NewFaker(), vm.Config{}, nil, nil)

	return db, bc
}

func RunArbitrageTx(db ethdb.Database, bc *core.BlockChain, block_num uint64, check_block_map map[string]bool, check_tx_map map[common.Hash]bool) {
	// block_num := arb.Tx.BlockNum
	// arb.TouchAddressMap = make(map[common.Address]TouchAddress)
	// fmt.Println(block_num)

	//如果不是MEV block则跳过
	if _, ok := check_block_map[strconv.Itoa(int(block_num))]; !ok {
		return
	}

	//准备State
	former_block_num := block_num - 1
	block_hash := rawdb.ReadCanonicalHash(db, block_num)
	former_block_hash := rawdb.ReadCanonicalHash(db, former_block_num)
	block := rawdb.ReadBlock(db, block_hash, block_num)
	former_block := rawdb.ReadBlock(db, former_block_hash, former_block_num)
	statedb, _ := bc.StateAt(former_block.Root())

	prefetch.TOUCH_ADDR_CH = make(chan prefetch.TouchLog, 100000)

	done_ch := make(chan bool)
	go func() {
		_, _, _, err := bc.Processor().Process(block, statedb, vm.Config{})
		if err != nil {
			fmt.Println(err)
			return
		}
		//fmt.Println("Finish", block_num)
		done_ch <- true
	}()

	is_done := false
	for {
		select {
		case done := <-done_ch:
			is_done = done
		case touch_log := <-prefetch.TOUCH_ADDR_CH:
			if _, ok := check_tx_map[touch_log.WhichTx]; !ok { //该Tx不属于arb tx
				break
			}
			address := touch_log.Address
			key := touch_log.Key
			//fmt.Println(address)
			arb := ARB_PTR_MAP[touch_log.WhichTx]
			if _, ok := ARB_VIS[arb.Tx.Hash]; !ok { //
				ARB_VIS[arb.Tx.Hash] = arb
			}
			if _, ok := arb.TouchAddressMap[address]; !ok { //维护全局变量
				arb.TouchAddressMap[address] = TouchAddress{InvokeCnt: 0, KeyMap: make(map[common.Hash]int)}
			}
			obj := arb.TouchAddressMap[address]
			obj.InvokeCnt += 1
			if _, ok := obj.KeyMap[key]; !ok {
				obj.KeyMap[key] = 0
			}
			obj.KeyMap[key] += 1
			arb.TouchAddressMap[address] = obj //维护全局变量
			//time.Sleep(time.Second)
		default:
			if is_done && len(prefetch.TOUCH_ADDR_CH) == 0 {
				fmt.Println("Done", block_num)
				return
			}
		}
	}
}

func ReadArbitrage(path string) (map[string]bool, map[common.Hash]bool) {
	// 读取文件到DATA
	input, err := os.Open(path)
	check(err)
	defer input.Close()
	input_bytes, err := ioutil.ReadAll(input)
	check(err)
	var arb_list []Arbitrage
	err = json.Unmarshal([]byte(input_bytes), &arb_list)
	check(err)
	fmt.Println("arb_list length:", len(arb_list))

	//fmt.Println(len(data))
	check_block_map := make(map[string]bool)   //用于判断哪个区块需要执行
	check_tx_map := make(map[common.Hash]bool) //用于执行区块的时候判断那些tx需要执行
	for _, arb_tx := range arb_list {
		//fmt.Println(bundle.BlockNumber)
		if _, ok := check_block_map[strconv.Itoa(int(arb_tx.Tx.BlockNum))]; !ok {
			check_block_map[strconv.Itoa(int(arb_tx.Tx.BlockNum))] = true
		}
		if _, ok := check_tx_map[arb_tx.Tx.Hash]; !ok {
			check_tx_map[arb_tx.Tx.Hash] = true
		}
		ARB_LIST = append(ARB_LIST, arb_tx)
		ARB_LIST[len(ARB_LIST)-1].TouchAddressMap = make(map[common.Address]TouchAddress)
		ARB_PTR_MAP[arb_tx.Tx.Hash] = &ARB_LIST[len(ARB_LIST)-1]
	}

	return check_block_map, check_tx_map

}

var INPUT_PATH = "./output/"
var OUTPUT_PATH = "./output/"
var BEGIN = uint64(19731000) //!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!
var END = uint64(19741000)   //!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!
var ARB_PTR_MAP = make(map[common.Hash]*Arbitrage)
var ARB_LIST = make([]Arbitrage, 0)               //用于存放文件中所有的arb tx信息
var ARB_VIS = make(map[common.Hash]*Arbitrage, 0) //用于存放执行执行完的（有时候我们不需要全部都执行完）

func main() {

	file_name := os.Args[1]

	check_block_map, check_tx_map := ReadArbitrage(INPUT_PATH + file_name + ".json")
	fmt.Println("MEV_block_map length:", len(check_block_map))
	fmt.Println("MEV_tx_map length:", len(check_tx_map))

	db, bc := GetBlockChain()
	for i := BEGIN; i <= END; i++ {
		RunArbitrageTx(db, bc, i, check_block_map, check_tx_map)
	}

	//只输出被执行到的
	output_arb := make([]Arbitrage, 0)
	for _, v := range ARB_VIS {
		output_arb = append(output_arb, *v)
	}

	// 输出成json格式
	jsonData, err := json.MarshalIndent(output_arb, "", "  ")
	check(err)
	//fmt.Println(string(jsonData))
	output, err := os.Create(OUTPUT_PATH + file_name + "_touched.json")
	check(err)
	output.Write(jsonData)
}

//这个文件 arb 和 liquidation都可以兼容
// go run get_arb_or_liquid_touched_address.go 19731000_20350000_clean_arb
