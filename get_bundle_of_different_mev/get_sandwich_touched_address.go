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

type Sandwich struct {
	FrontRun        CleanMEVTx
	VictimTx        []CleanMEVTx
	BackRun         CleanMEVTx
	TouchAddressMap map[common.Address]TouchAddress
}

type TouchAddress struct {
	//Address   common.Address
	InvokeCnt uint64
	KeyMap    map[common.Hash]int
}

func ReadSandwich(path string) (map[string]bool, map[common.Hash]bool) {
	// 读取文件到DATA
	input, err := os.Open(path)
	check(err)
	defer input.Close()
	input_bytes, err := ioutil.ReadAll(input)
	check(err)
	var sandwich_list []Sandwich
	err = json.Unmarshal([]byte(input_bytes), &sandwich_list)
	check(err)
	fmt.Println("sandwich_list length:", len(sandwich_list))

	//fmt.Println(len(data))
	check_block_map := make(map[string]bool)   //用于判断哪个区块需要执行
	check_tx_map := make(map[common.Hash]bool) //用于执行区块的时候判断那些tx需要执行
	for _, bundle := range sandwich_list {
		//fmt.Println(bundle.BlockNumber)
		if bundle.FrontRun.BlockNum != bundle.BackRun.BlockNum { //判断三文治的交易是否都在一个区块内
			fmt.Println("Illigal Bundle!")
			continue
		}
		if _, ok := check_block_map[strconv.Itoa(int(bundle.FrontRun.BlockNum))]; !ok { //因为一个bundle的交易都在同一个区块，所以记录一个就好
			check_block_map[strconv.Itoa(int(bundle.FrontRun.BlockNum))] = true
		}
		if _, ok := check_tx_map[bundle.FrontRun.Hash]; !ok {
			check_tx_map[bundle.FrontRun.Hash] = true
		}
		if _, ok := check_tx_map[bundle.BackRun.Hash]; !ok {
			check_tx_map[bundle.BackRun.Hash] = true
		}
		for _, victim := range bundle.VictimTx {
			check_tx_map[victim.Hash] = true
		}
		if _, ok := SANDWICH_MAP[bundle.FrontRun.BlockNum]; !ok {
			SANDWICH_MAP[bundle.FrontRun.BlockNum] = make([]Sandwich, 0)
		}
		bundle.TouchAddressMap = make(map[common.Address]TouchAddress)
		SANDWICH_MAP[bundle.FrontRun.BlockNum] = append(SANDWICH_MAP[bundle.FrontRun.BlockNum], bundle)
		//SANDWICH_LIST[len(SANDWICH_LIST)-1].TouchAddressMap = make(map[common.Address]TouchAddress)
		//ARB_PTR_MAP[arb_tx.Tx.Hash] = &ARB_LIST[len(ARB_LIST)-1]
	}

	return check_block_map, check_tx_map

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

func RunSandwich(db ethdb.Database, bc *core.BlockChain, block_num uint64, check_block_map map[string]bool, check_tx_map map[common.Hash]bool) {
	// 如果不是MEV block则跳过
	if _, ok := check_block_map[strconv.Itoa(int(block_num))]; !ok {
		return
	}

	// 准备State
	former_block_num := block_num - 1
	block_hash := rawdb.ReadCanonicalHash(db, block_num)
	former_block_hash := rawdb.ReadCanonicalHash(db, former_block_num)
	block := rawdb.ReadBlock(db, block_hash, block_num)
	former_block := rawdb.ReadBlock(db, former_block_hash, former_block_num)
	statedb, _ := bc.StateAt(former_block.Root())

	prefetch.TOUCH_ADDR_CH = make(chan prefetch.TouchLog, 100000)

	waited_sandwich_list := SANDWICH_MAP[block_num] //当前执行块中的所有sandwich bundle
	tx_sandwich_map := make(map[common.Hash]int)    //用tx定位当前执行的sandwich bundle
	for i, bundle := range waited_sandwich_list {
		tx_sandwich_map[bundle.FrontRun.Hash] = i
		tx_sandwich_map[bundle.BackRun.Hash] = i
		for _, victim := range bundle.VictimTx {
			tx_sandwich_map[victim.Hash] = i
		}
	}
	//current := 0 //当前执行到第几个sandwich

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
			if _, ok := check_tx_map[touch_log.WhichTx]; !ok { //该Tx不属于sandwich tx
				break
			}
			current_tx := touch_log.WhichTx
			address := touch_log.Address
			key := touch_log.Key
			if _, ok := waited_sandwich_list[tx_sandwich_map[current_tx]].TouchAddressMap[address]; !ok {
				waited_sandwich_list[tx_sandwich_map[current_tx]].TouchAddressMap[address] = TouchAddress{InvokeCnt: 0, KeyMap: make(map[common.Hash]int)}
			}
			obj := waited_sandwich_list[tx_sandwich_map[current_tx]].TouchAddressMap[address]
			obj.InvokeCnt += 1
			if _, ok := obj.KeyMap[key]; !ok {
				obj.KeyMap[key] = 0
			}
			obj.KeyMap[key] += 1
			waited_sandwich_list[tx_sandwich_map[current_tx]].TouchAddressMap[address] = obj //维护全局变量
			//time.Sleep(time.Second)
		default:
			if is_done && len(prefetch.TOUCH_ADDR_CH) == 0 {
				DONE_SANDWICH_LIST = append(DONE_SANDWICH_LIST, waited_sandwich_list...) //维护全局变量
				fmt.Println("Done", block_num)
				return
			}
		}
	}
}

var INPUT_PATH = "./output/"
var OUTPUT_PATH = "./output/"
var SANDWICH_MAP = make(map[uint64][]Sandwich, 0) //存放合法的三文治交易(按块号索引)，等待被执行
var DONE_SANDWICH_LIST = make([]Sandwich, 0)      //执行完获取到touched Address的sandwich bundle（有时候不需要全部执行完所有sandwich bundle）
var BEGIN = uint64(19731000)
var END = uint64(19733000)

func main() {

	file_name := os.Args[1]
	check_block_map, check_tx_map := ReadSandwich(INPUT_PATH + file_name + ".json")
	fmt.Println("MEV_block_map length:", len(check_block_map))
	fmt.Println("MEV_tx_map length:", len(check_tx_map))

	db, bc := GetBlockChain()
	for i := BEGIN; i <= END; i++ {
		RunSandwich(db, bc, i, check_block_map, check_tx_map)
	}

	//fmt.Println(DONE_SANDWICH_LIST)
	// 输出成json格式
	jsonData, err := json.MarshalIndent(DONE_SANDWICH_LIST, "", "  ")
	check(err)
	//fmt.Println(string(jsonData))
	output, err := os.Create(OUTPUT_PATH + file_name + "_touched.json")
	check(err)
	output.Write(jsonData)

}

//go run get_sandwich_touched_address.go 19731000_20350000_clean_sandwich
