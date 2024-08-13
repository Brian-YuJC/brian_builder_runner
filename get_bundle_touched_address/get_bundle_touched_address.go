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

var INPUT_PATH string = "../get_flashbots_historical_bundle/output/"
var OUTPUT_PATH string = "./output/"

func check(e error) {
	if e != nil {
		panic(e)
	}
}

type Bundle struct {
	BlockNumber string
	Tx          []Transaction
}

type Transaction struct {
	Hash        common.Hash
	BundleType  string
	BundleIndex int //tx在block中的哪个bundle
	TxIndex     int //tx在block中的位置
	From        common.Address
	To          common.Address
}

type TouchAddress struct {
	//Address   common.Address
	InvokeCnt uint64
	KeyMap    map[common.Hash]int
}

// 读取disk中的数据库并建立区块链
var CustomConfig = &core.CacheConfig{
	TrieCleanLimit: 1024,
	TrieDirtyLimit: 1024,
	TrieTimeLimit:  5 * time.Minute,
	SnapshotLimit:  1024,
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

var TouchMap map[common.Address]TouchAddress = make(map[common.Address]TouchAddress) //MEV tx中Address的调用次数以及其key的调用次数

func ReadBundle(path string) (map[string]bool, map[common.Hash]bool) {
	input, err := os.Open(path)
	check(err)
	defer input.Close()

	// 读取文件内容
	input_bytes, err := ioutil.ReadAll(input)
	check(err)

	var data []Bundle //直接映射成Bundle结构体
	err = json.Unmarshal([]byte(input_bytes), &data)
	check(err)

	//fmt.Println(len(data))
	check_block_map := make(map[string]bool)   //用于判断哪个区块需要执行
	check_tx_map := make(map[common.Hash]bool) //用于执行区块的时候判断那些tx需要执行
	for _, bundle := range data {
		//fmt.Println(bundle.BlockNumber)
		if _, ok := check_block_map[bundle.BlockNumber]; !ok {
			check_block_map[bundle.BlockNumber] = true
		}
		for _, tx := range bundle.Tx {
			if _, ok := check_tx_map[tx.Hash]; !ok {
				check_tx_map[tx.Hash] = true
			}
		}
	}

	return check_block_map, check_tx_map
}

func RunBlock(db ethdb.Database, bc *core.BlockChain, block_num uint64, check_block_map map[string]bool, check_tx_map map[common.Hash]bool) {

	//如果不是MEV block则跳过
	if _, ok := check_block_map[strconv.Itoa(int(block_num))]; !ok {
		return
	}
	//fmt.Println("OK block number: ", block_num)

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
			// if _, ok := check_tx_map[touch_log.WhichTx]; !ok { //该touch address不属于mev tx中的
			// 	break
			// }
			address := touch_log.Address
			key := touch_log.Key
			//fmt.Println(address)
			if _, ok := TouchMap[address]; !ok {
				TouchMap[address] = TouchAddress{InvokeCnt: 0, KeyMap: make(map[common.Hash]int)}
			}
			obj := TouchMap[address]
			obj.InvokeCnt += 1
			if _, ok := obj.KeyMap[key]; !ok {
				obj.KeyMap[key] = 0
			}
			obj.KeyMap[key] += 1
			TouchMap[address] = obj
			//time.Sleep(time.Second)
		default:
			if is_done && len(prefetch.TOUCH_ADDR_CH) == 0 {
				fmt.Println("Done", block_num)
				return
			}
		}
	}
}

func main() {
	check_block_map, check_tx_map := ReadBundle(INPUT_PATH + os.Args[1])
	fmt.Println("check_block_map length:", len(check_block_map))
	fmt.Println("check_tx_map length:", len(check_tx_map))

	db, bc := GetBlockChain()
	//TODO这里加一个循环
	//RunBlock(db, bc, 19774797, check_block_map, check_tx_map)
	begin := uint64(19731000)
	end := uint64(20200000)
	for i := begin; i <= end; i++ {
		RunBlock(db, bc, i, check_block_map, check_tx_map)
	}

	fmt.Println("Touch Address count: ", len(TouchMap))

	// 输出成json格式
	jsonData, err := json.MarshalIndent(TouchMap, "", "  ")
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	//fmt.Println(string(jsonData))
	output, err := os.Create(OUTPUT_PATH + os.Args[1])
	check(err)
	output.Write(jsonData)
}
