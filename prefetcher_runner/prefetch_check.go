package main

import (
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/prefetch"
	"github.com/ethereum/go-ethereum/prefetch/metric"
)

func check(e error) {
	if e != nil {
		panic(e)
	}
}

var TestBlockChainCacheConfig = &core.CacheConfig{
	TrieCleanLimit:      256,
	TrieCleanNoPrefetch: true,
	TrieDirtyLimit:      256,
	TrieTimeLimit:       5 * time.Minute,
	SnapshotLimit:       256,
	SnapshotWait:        true,
	StateScheme:         rawdb.HashScheme,
	SnapshotNoBuild:     true,
	//TrieDirtyDisabled: true,
}

// 读取disk中的数据库并建立区块链
func GetBlockChain() (ethdb.Database, *core.BlockChain) {
	datadir := "/home/user/common/docker/volumes/eth-docker_geth-eth1-data/_data/geth/chaindata"
	// datadir := "/home/user/common/docker/volumes/cp1_eth-docker_geth-eth1-data/_data/geth/chaindata"
	ancient := datadir + "/ancient"
	db, err := rawdb.Open(
		rawdb.OpenOptions{
			Directory:         datadir,
			AncientsDirectory: ancient,
			Ephemeral:         true,
			ReadOnly:          true,
		},
	)
	if err != nil {
		fmt.Println("rawdb.Open err!", err)
	} else {
		fmt.Println("Open Database Success!")
		fmt.Println("Blockchain DB Scheme:", rawdb.ReadStateScheme(db))
	}

	bc, err := core.NewBlockChain(db, TestBlockChainCacheConfig, nil, nil, ethash.NewFaker(), vm.Config{}, nil, nil)
	check(err)
	fmt.Println("New Blockchain Success!")

	return db, bc
}

// 进行简单测试要用
func ParallelRunBlock(db ethdb.Database, bc *core.BlockChain, start_block_num uint64, prefetch_type string) {
	former_block_num := start_block_num - 1
	block_hash := rawdb.ReadCanonicalHash(db, start_block_num)
	former_block_hash := rawdb.ReadCanonicalHash(db, former_block_num)
	block := rawdb.ReadBlock(db, block_hash, start_block_num)
	former_block := rawdb.ReadBlock(db, former_block_hash, former_block_num)
	statedb, _ := bc.StateAt(former_block.Root())

	//选择prefetch方法
	if prefetch_type == "Trie" {
		prefetch.Trie_Prefetch(statedb.GetTrie(), prefetch.ReadPrefetchList())
	} else if prefetch_type == "StateDB" {
		prefetch.StateDB_Prefetch(statedb, prefetch.ReadPrefetchList())
	} else if prefetch_type == "" {
		//不prefetch
	} else {
		panic("prefetch type err")
	}

	// var wg sync.WaitGroup
	// run := func(i uint64) {
	// 	defer wg.Done()
	// 	block_hash := rawdb.ReadCanonicalHash(db, start_block_num+i)
	// 	block := rawdb.ReadBlock(db, block_hash, start_block_num+i)
	// 	_, _, _, err := bc.Processor().Process(block, statedb.Copy(), vm.Config{})
	// 	check(err)
	// }

	// begin := time.Now()
	// for i := uint64(0); i < cnt; i++ {
	// 	wg.Add(1)
	// 	go run(i)
	// }
	// wg.Wait()
	// fmt.Println("Execute Time:", time.Since(begin))

	begin := time.Now()
	metric.GLOBAL_HIT_MONITOR.Start()
	_, _, _, err := bc.Processor().Process(block, statedb.Copy(), vm.Config{})
	metric.GLOBAL_HIT_MONITOR.Stop()
	metric.GLOBAL_HIT_MONITOR.OutputRecord()
	check(err)
	fmt.Println("Execute Time:", time.Since(begin))

}

func main() {

	db, bc := GetBlockChain()
	ParallelRunBlock(db, bc, 20300000, "StateDB")
	db.Close()
}

/*
用于分析如何进行prefetch
*/
