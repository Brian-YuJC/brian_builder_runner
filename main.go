package main

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/ethdb"
)

func check(e error) {
	if e != nil {
		panic(e)
	}
}

// 读取disk中的数据库并建立区块链
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
	bc, _ := core.NewBlockChain(db, core.DefaultCacheConfigWithScheme(rawdb.HashScheme), nil, nil, ethash.NewFaker(), vm.Config{}, nil, nil)

	return db, bc
}

// 测试根据地址列表prefetch的运行时间是否变快
// TODO 能否打印一下各个SLOAD的命中率
func TestAddrPrefetch(db ethdb.Database, bc *core.BlockChain, block_num uint64, do_prefetch bool) {
	//读取区块准备环境
	former_block_num := block_num - 1
	block_hash := rawdb.ReadCanonicalHash(db, block_num)
	former_block_hash := rawdb.ReadCanonicalHash(db, former_block_num)
	block := rawdb.ReadBlock(db, block_hash, block_num)
	former_block := rawdb.ReadBlock(db, former_block_hash, former_block_num)
	statedb, _ := bc.StateAt(former_block.Root())

	// //如果需要预取则进行相应操作(builder自带函数)
	// if do_prefetch {
	// 	fmt.Println(block_num, " do_prefetch")
	// 	// do former block prefetch
	// 	prefetch_list := make([][]byte, 0)        //未来我们的工作应该就是维护一个addr prefetch_list
	// 	for _, tx := range block.Transactions() { //看每个Transaction的To都访问了那些地址
	// 		prefetch_list = append(prefetch_list, tx.To().Bytes()) //把将要访问的地址存起来
	// 	}
	// 	prefetcher := state.NewTriePrefetcher(statedb.GetDB(), statedb.GetOriginalRoot(), "")          //新建一个预取器
	// 	prefetcher.Prefetch(common.Hash{}, statedb.GetOriginalRoot(), common.Address{}, prefetch_list) //这里只用到了prefetch 指定addr的功能，还可以指定storage slot的
	// } else {
	// 	fmt.Println(block_num, " without_prefetch")
	// }

	//自己写的Prefetch函数
	if do_prefetch {
		fmt.Println(block_num, " do_prefetch")
		Prefetch(statedb, ReadPrefetchList())
	} else {
		fmt.Println(block_num, " without_prefetch")
	}

	start_time := time.Now()
	_, _, _, err := bc.Processor().Process(block, statedb, vm.Config{})
	end_time := time.Now()
	exec_time := end_time.Sub(start_time).Seconds()
	fmt.Println("Execute time: ", exec_time, "s")
	if err != nil {
		fmt.Println("BC Process err!", err)
	}
	fmt.Println()
}

// 读取需要prefetch的Address-slot
func ReadPrefetchList() map[common.Address][]common.Hash {
	path := "./output/addr_hash.log"
	file, err := os.Open(path)
	if err != nil {
		fmt.Println(err)
	}
	prefetch_map := make(map[common.Address][]common.Hash)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		s := strings.Split(strings.Split(line, "\n")[0], " ")
		var addr common.Address
		var hash common.Hash
		byte1, err := hex.DecodeString(s[0][2:])
		check(err)
		addr.SetBytes(byte1)
		byte2, err := hex.DecodeString(s[1][2:])
		check(err)
		hash.SetBytes(byte2)
		//fmt.Fprintln(os.Stderr, addr, hash)
		if _, ok := prefetch_map[addr]; !ok {
			prefetch_map[addr] = make([]common.Hash, 0)
		}
		prefetch_map[addr] = append(prefetch_map[addr], hash)
	}
	return prefetch_map
}

func Prefetch(statedb *state.StateDB, prefetch_map map[common.Address][]common.Hash) {
	for addr, v := range prefetch_map {
		for _, hash := range v {
			statedb.GetState(addr, hash)
		}
	}
}

func main() {

	// // TODO看不太出来那个好些，需要打印一下SLOAD命中情况
	// run_cnt := 5
	// for i := 0; i < run_cnt; i++ {
	// 	fmt.Println("### Run Count", i+1, "###")
	// 	db, bc := GetBlockChain()
	// 	TestAddrPrefetch(db, bc, 19795703, true)
	// 	db.Close()
	// 	db, bc = GetBlockChain()
	// 	TestAddrPrefetch(db, bc, 19795703, false)
	// 	db.Close()
	// }

	//Test print log of SLOAD
	//prefetch.LOG.Init()
	db, bc := GetBlockChain()
	TestAddrPrefetch(db, bc, 19774797, false)
	//prefetch.PrintLogLinear(prefetch.LOG)

	//ReadPrefetchList()
}

//go run main.go 2> a.log
//Use to run builder functions
//Run specific block with(using provided predetch list) or without prefetch and output the log
//TODO improve the address collection in opSload() (now is using err output)
