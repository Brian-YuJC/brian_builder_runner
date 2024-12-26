package main

import (
	"bytes"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"os"
	"reflect"
	"sort"
	"time"

	"github.com/ethereum/go-ethereum/builder"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/txpool"
	"github.com/ethereum/go-ethereum/core/txpool/legacypool"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto/kzg4844"
	"github.com/ethereum/go-ethereum/eth/ethconfig"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/miner"
	"github.com/ethereum/go-ethereum/prefetch/metric"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/google/uuid"
	"github.com/holiman/uint256"
)

// BlockChain custom cache config
var TestBlockChainCacheConfig = &core.CacheConfig{
	TrieCleanLimit:      256,
	TrieCleanNoPrefetch: true,
	TrieDirtyLimit:      256,
	TrieTimeLimit:       5 * time.Minute,
	SnapshotLimit:       256,
	SnapshotWait:        true,
	StateScheme:         rawdb.HashScheme,
	SnapshotNoBuild:     true, //snapconfig：NoBuild
	//TrieDirtyDisabled: true,
}

// 读取disk中的数据库并建立区块链
func GetBlockChain() (ethdb.Database, *core.BlockChain) {

	//打开数据库
	datadir := "/home/user/common/docker/volumes/eth-docker_geth-eth1-data/_data/geth/chaindata"
	//datadir := "/home/user/common/docker/volumes/cp1_eth-docker_geth-eth1-data/_data/geth/chaindata"
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
		fmt.Println(ts(), "Open Database Success!")
		fmt.Println(ts(), "Blockchain DB Scheme:", rawdb.ReadStateScheme(db))
	}

	//建立Engine
	// Transfer mining-related config to the ethash config.
	genesis, err := core.ReadGenesis(db)
	check(err)
	chainConfig, err := core.LoadChainConfig(db, genesis)
	check(err)
	engine, err := ethconfig.CreateConsensusEngine(chainConfig, db)
	check(err)

	//bc, err := core.NewBlockChain(db, TestBlockChainCacheConfig, nil, nil, ethash.NewFaker(), vm.Config{}, nil, nil)
	bc, err := core.NewBlockChain(db, TestBlockChainCacheConfig, genesis, nil, engine, vm.Config{}, nil, nil)
	check(err)
	fmt.Println(ts(), "New Blockchain Success!")

	return db, bc
}

// 模拟builder运行
// CustomHeadHash 当前区块高度头
// CustomTimestamp 当前虚拟时间戳
// step模拟执行几个slot
func SimulateRunBuilder(CustomHeadHash common.Hash, CustomTimestamp hexutil.Uint64, step uint64) {
	// //持续接收真实Beacon Chain的Payload Attributes信息
	// var BeaconClient *builder.BeaconClient = builder.InitBeaconClient()
	// payloadAttrC := make(chan types.BuilderPayloadAttributes)
	// go BeaconClient.SubscribeToPayloadAttributesEvents(payloadAttrC)

	//模拟发送虚假的Payload Attributes信息
	payloadAttrC := make(chan types.BuilderPayloadAttributes) //BuilderPayloadAttributes event信号
	stop := make(chan struct{})                               //停止信号
	go func(C chan types.BuilderPayloadAttributes) {
		var slot uint64 = 1 //模拟slot
		for {
			//模拟BuilderPayloadAttributes
			attr := types.BuilderPayloadAttributes{
				Random:                common.HexToHash("0x6e8596e96c30014519a7043ac7e1c0edb45a6794410fcf4e8262872288a0a68f"),
				HeadHash:              CustomHeadHash,  //当前实验环境的Head
				Timestamp:             CustomTimestamp, //当前实验环境的Timestemp
				SuggestedFeeRecipient: common.HexToAddress("0x451DcAcfd7e92A3604Ed68E063BFa1200c3D3aF3"),
				Slot:                  slot,
				GasLimit:              uint64(0), //30_000_000
				//ParentBeaconBlockRoot: common.HexToHash("0x267235046593beb1ac376d3c89d0a7171038bcd3f22d4b1661fb024edb2c0829"),
			}
			C <- attr                    //发送信号
			slot++                       //模拟slot递增
			time.Sleep(14 * time.Second) //等待时间

			//根据step发送停止信号
			if slot == step+1 {
				stop <- struct{}{}
			}

		}
	}(payloadAttrC)

	// 来自 func (b *Builder) Start() error {} 稍加改动
	ignoreLatePayloadAttributes := false
	currentSlot := uint64(0)
	fmt.Println(ts(), "Waiting PayloadAttributesEvent")
	for {
		select {
		case <-stop:
			return
		case payloadAttributes := <-payloadAttrC:
			fmt.Println(ts(), hl("payloadAttributes:", "blue"), payloadAttributes) //Brian Add
			// fmt.Println("GasLimit", payloadAttributes.GasLimit)                           //Brian Add
			// fmt.Println("Random", payloadAttributes.Random)                               //Brian Add
			// fmt.Println("SuggestedFeeRecipient", payloadAttributes.SuggestedFeeRecipient) //Brian Add
			// fmt.Println("Slot", payloadAttributes.Slot)                                   //Brian Add
			// fmt.Println("Withdrawals", payloadAttributes.Withdrawals)                     //Brian Add
			// fmt.Println("ParentBeaconBlockRoot", payloadAttributes.ParentBeaconBlockRoot) //Brian Add
			if payloadAttributes.Slot < currentSlot {
				//fmt.Println("payloadAttributes.Slot < currentSlot") // Brian Add
				continue
			} else if payloadAttributes.Slot == currentSlot {
				// Subsequent sse events should only be canonical!
				//fmt.Println("payloadAttributes.Slot == currentSlot") //Brian Add
				if ignoreLatePayloadAttributes {
					builder.PrefetchOnPayloadAttribute(&payloadAttributes)
				}
			} else if payloadAttributes.Slot > currentSlot {
				//fmt.Println("payloadAttributes.Slot > currentSlot") //Brian Add
				currentSlot = payloadAttributes.Slot
				builder.PrefetchOnPayloadAttribute(&payloadAttributes)
			}
		}
	}
}

// 模拟添加普通tx
func TxPoolSimulation(db ethdb.Database, bc *core.BlockChain, block_num uint64, tx_pool *txpool.TxPool, pending_list []*types.Transaction) {

	// //重构tx GasPrice
	// ReconstructGasPrice := func(pending_list []*types.Transaction) {
	// 	for _, tx := range pending_list {
	// 		inner := tx.GetInner().(*types.LegacyTx)
	// 		inner.GasPrice = big.NewInt(int64(400000000)) //重构GasPrice 保证满足BaseFee要求
	// 	}
	// }

	//重构Transaction 的Nonce
	_ReconstructNonce := func(db ethdb.Database, bc *core.BlockChain, block_num uint64, pending_list []*types.Transaction) {
		//初始化重建环境
		block_hash := rawdb.ReadCanonicalHash(db, block_num)
		block := rawdb.ReadBlock(db, block_hash, block_num)
		state, err := bc.StateAt(block.Root())
		check(err)
		header := block.Header()
		config := bc.Config()

		//维护sender和其所有的tx
		account_map := make(map[common.Address][]*types.Transaction)

		for _, tx := range pending_list {
			sender, err := types.Sender(types.MakeSigner(config, header.Number, header.Time), tx)
			check(err)
			if _, ok := account_map[sender]; !ok { //该sender第一次出现
				account_map[sender] = make([]*types.Transaction, 0) //创建sender为当前send的tx指针列表
			}
			account_map[sender] = append(account_map[sender], tx) //维护map
		}

		//按顺序重构同一个sender的所有tx nonce
		for sender, tx_list := range account_map {
			// 同一个sender发送的tx，按原来的nonce从小到大排序
			// 因为重构的时候会从小到大分配新nonce
			sort.Slice(tx_list, func(i, j int) bool {
				var nonce_i uint64 = TxNonceGetter(tx_list[i])
				var nonce_j uint64 = TxNonceGetter(tx_list[j])
				return nonce_i < nonce_j
			})
			//同一个sender，多个tx，按原来nonce排序重构为新nonce
			nonce := state.GetNonce(sender) //当前状态下sender的nonce
			for _, tx := range tx_list {
				TxNonceSetter(tx, nonce) // 重构每个tx的nonce
				nonce += 1               //下个tx nonce+1
			}
		}
	}

	//开始模拟往txpool添加tx， frequency单位毫秒
	_Start := func(pending []*types.Transaction, frequency uint64) {
		for _, tx := range pending {
			errs := tx_pool.Add([]*types.Transaction{tx}, true, false, false)
			for _, err := range errs {
				if err != nil {
					fmt.Println(hl(err.Error(), "red"))
				}
			}
			time.Sleep(time.Duration(frequency) * time.Millisecond)
		}
	}

	//	如果是用的transaction.rlp文件，没有直接提供pending_list
	if pending_list == nil {
		temp_pool := legacypool.New(legacypool.DefaultConfig, bc)
		temp_txpool, err := txpool.New(legacypool.DefaultConfig.PriceLimit, bc, []txpool.SubPool{temp_pool})
		check(err)
		pending, _ := temp_txpool.Content()
		for _, txs := range pending {
			pending_list = append(pending_list, txs...) //pending字典扁平化
		}
	}
	_ReconstructNonce(db, bc, block_num, pending_list) //用当前模拟的状态重构tx的Nonce，让tx能正常运行
	fmt.Println(ts(), "Simulation Pending List:", len(pending_list))
	//PrintTxInfo(pending_list) //Debug
	//ReconstructGasPrice(pending_list) //重构tx GasPrice
	go _Start(pending_list, 100)

}

func main() {

	//搭建环境
	db, bc := GetBlockChain() //打开数据库建立区块链

	RebuildSnapshotDiffLayer(db, bc, 20306539, 50)
	return

	// //Test ：只允许一个block。判断有无Snapshot以及Sload运行状态
	// //fmt.Println("CurrentBlock", bc.CurrentBlock().Number)
	// //metric.GLOBAL_HIT_MONITOR.Start()
	// RunSingleBlock(db, bc, 20306539, true)
	// // metric.GLOBAL_HIT_MONITOR.Stop()
	// // metric.GLOBAL_HIT_MONITOR.OutputRecord("./sload_.log") //TODO 检查命中情况
	// return

	env := miner.InitEnv(db, bc) //初始化miner环境
	worker := env.Worker
	tx_pool := worker.GetEth().TxPool() //获取worker的tx_pool, 由于配置过一开始txpool是空的
	// filter := txpool.PendingFilter{
	// 	MinTip: worker.GetTip(),
	// }
	// pending := tx_pool.Pending(filter)
	//PrintTxInfo(pending) //Debug
	//return
	//读取数据集，并往txpool添加bundle
	// bundles := ReadBundleDataset(bc, "./dataset/20300000_20310000_0_9978.csv", "csv", "enhance")
	// for _, bundle := range bundles { //往txpool添加bundle
	// 	err := tx_pool.AddMevBundle(bundle.Txs, bundle.BlockNumber, bundle.Uuid, bundle.SigningAddress, bundle.MinTimestamp, bundle.MaxTimestamp, bundle.RevertingTxHashes)
	// 	check(err)
	// }
	//fmt.Println(ts(), "Finish Init!", "Pending Tx in Txpool: ", len(pending), "MEVBundles in Txpool:", 0)

	//开始模拟运行
	TxPoolSimulation(db, bc, 20306538, tx_pool /*GetBlockTransactionsDB(db, bc, 20306538)*/, GetBlockRPC(20306539).Transactions()) //模拟往txpool添加东西
	//metric.GLOBAL_HIT_MONITOR.Start()
	SimulateRunBuilder(bc.CurrentBlock().Hash(), hexutil.Uint64(bc.CurrentBlock().Time+1), 1) //模拟builder打包区块
	//metric.GLOBAL_HIT_MONITOR.Stop()
	//metric.GLOBAL_HIT_MONITOR.OutputRecord("./sload_.log") //TODO 检查命中情况

	//TODO 看看为什么pending tx不用访问sload
	//TODO 为什么snapshot limit不同差这么多 可能是基本都是transfer的交易，所以执行得快
	//TODO 同样的transfer交易在有无snapshot的情况下执行时间对比
	//TODO applyTransaction 写了print TxPoolSimulation没完成（将20306538的块中tx丢到有snapshot的环境运行，比较执行时间）

	//
	//db.Close()

}

// Test
func testBeaconClient() {
	// 测试：从consensus节点获取payload信息
	var BeaconClient *builder.BeaconClient = builder.InitBeaconClient()
	payloadAttrC := make(chan types.BuilderPayloadAttributes)
	go BeaconClient.SubscribeToPayloadAttributesEvents(payloadAttrC)
	for {
		payloadAttributes := <-payloadAttrC
		fmt.Println(payloadAttributes)
	}
}

// Utils
func ReadBundleDataset(bc *core.BlockChain, dataset_path string, dataset_type string, reconstruct_method string) []types.MevBundle {
	var bundles []types.MevBundle

	//读取数据集
	next_block_number := bc.CurrentBlock().Number.Uint64() + 1
	state, err := bc.State()     //获取区块链最新状态
	header := bc.CurrentHeader() //获取当前区块头
	check(err)
	if dataset_type == "csv" { //csv文件
		bundles = ReadBundleDatasetCSV(dataset_path, next_block_number) //读取bundle csv数据
	} else {
		fmt.Println("Err Dataset Type!")
	}

	//选择重构方案用于数据集状态范围和当前状态相差很多的情况（不然不改的话都是非法交易）
	if reconstruct_method == "enhance" {
		ReconstructNonceEnhance(bc, bundles, state, header) //跟据当前状态重构bundle的nonce（因为数据集的bundle不一定是当前状态下的）
	} else if reconstruct_method == "normal" {
		ReconstructNonce(bc, bundles, state, header) //跟据当前状态重构bundle的nonce（因为数据集的bundle不一定是当前状态下的）
	} else if reconstruct_method == "" { //不进行任何的nonce重构

	} else {
		fmt.Println("Err Reconstruct Method!")
	}

	//fmt.Println("Bundle in Dataset:", len(bundles))
	return bundles
}

// Utils
// 基于某个状态重构bundle中tx的Nonce
// 如果同一个发送者有多条tx在MEVbundle pool，Nonce全部一样都是重构为当前状态下发送者的下一个Nonce
func ReconstructNonce(bc *core.BlockChain, bundles []types.MevBundle, state *state.StateDB, header *types.Header) {
	config := bc.Config()
	account_map := make(map[common.Address]uint64)
	for _, bundle := range bundles {
		for _, tx := range bundle.Txs {
			sender, err := types.Sender(types.MakeSigner(config, header.Number, header.Time), tx)
			check(err)
			if _, ok := account_map[sender]; !ok {
				account_map[sender] = state.GetNonce(sender)
			}
			TxNonceSetter(tx, account_map[sender]) //设置新Nonce
		}
	}
}

// Utils
// 基于某个状态重构bundle中tx的Nonce
func ReconstructNonceEnhance(bc *core.BlockChain, bundles []types.MevBundle, state *state.StateDB, header *types.Header) {
	config := bc.Config()
	//维护sender和其所有的tx
	account_map := make(map[common.Address][]*types.Transaction)
	for _, bundle := range bundles {
		for _, tx := range bundle.Txs {
			sender, err := types.Sender(types.MakeSigner(config, header.Number, header.Time), tx)
			check(err)
			if _, ok := account_map[sender]; !ok { //该sender第一次出现
				account_map[sender] = make([]*types.Transaction, 0) //创建sender为当前send的tx指针列表
			}
			account_map[sender] = append(account_map[sender], tx) //维护map
		}
	}
	//按顺序重构同一个sender的所有tx nonce
	for sender, tx_list := range account_map {
		// 同一个sender发送的tx，按原来的nonce从小到大排序
		// 因为重构的时候会从小到大分配新nonce
		sort.Slice(tx_list, func(i, j int) bool {
			var nonce_i uint64 = TxNonceGetter(tx_list[i])
			var nonce_j uint64 = TxNonceGetter(tx_list[j])
			return nonce_i < nonce_j
		})
		//同一个sender，多个tx，按原来nonce排序重构为新nonce
		nonce := state.GetNonce(sender) //当前状态下sender的nonce
		for _, tx := range tx_list {
			TxNonceSetter(tx, nonce) // 重构每个tx的nonce
			nonce += 1               //下个tx nonce+1
		}
	}
}

// Utils
// 读取MEVbundle的csv文件，然后解码16进制的tx并打包成MEVBundle
func ReadBundleDatasetCSV(database_path string, next_block_number uint64) []types.MevBundle {
	// 打开文件
	input, err := os.Open(database_path)
	check(err)
	defer input.Close()

	reader := csv.NewReader(input)

	// 增加缓冲区大小（可选）
	reader.FieldsPerRecord = -1 // 允许不定字段数量

	// 读取bundle
	res := make([]types.MevBundle, 0)
	for {
		row, err := reader.Read()
		if err == io.EOF { //文件读取完成跳出循环
			break
		}
		txs := types.Transactions{}
		for _, tx_hex := range row[1:] {
			tx := new(types.Transaction)
			tx_byte, err := hex.DecodeString(tx_hex[2:]) //将Tx 16进制字符串转字节流
			check(err)
			err = tx.UnmarshalBinary([]byte(tx_byte)) //将Tx字节流解码成Transaction对象
			check(err)
			txs = append(txs, tx) //加入bundle tx列表
		}
		// 新建bundle
		var blockNumber uint64 = next_block_number //这里将全部bundle设置为当前区块号的下一个区块，一遍模拟执行下一个区块时能取出bundle
		var replacementUuid uuid.UUID
		var signingAddress common.Address
		var minTimestamp uint64
		var maxTimestamp uint64
		var revertingTxHashes []common.Hash
		bundle := types.MevBundle{
			BlockNumber:       new(big.Int).SetUint64(blockNumber),
			Uuid:              replacementUuid,
			SigningAddress:    signingAddress,
			MinTimestamp:      minTimestamp,
			MaxTimestamp:      maxTimestamp,
			RevertingTxHashes: revertingTxHashes,
			Txs:               txs,
		}
		res = append(res, bundle) //加入返回的数据集bundle列表
		//fmt.Println(len(bundle.Txs))
	}
	return res
}

// Utils
// 通过Transaction反射获取Nonce
func TxNonceGetter(tx *types.Transaction) uint64 {
	type_ := reflect.TypeOf(tx.GetInner()).String()
	var nonce uint64
	if type_ == "*types.LegacyTx" {
		inner := tx.GetInner().(*types.LegacyTx)
		nonce = inner.Nonce
	} else if type_ == "*types.DynamicFeeTx" {
		inner := tx.GetInner().(*types.DynamicFeeTx)
		nonce = inner.Nonce
	} else if type_ == "*types.AccessListTx" {
		inner := tx.GetInner().(*types.AccessListTx)
		nonce = inner.Nonce
	} else if type_ == "*types.BlobTx" {
		inner := tx.GetInner().(*types.BlobTx)
		nonce = inner.Nonce
	} else {
		panic("unknow type:" + type_)
	}
	return nonce
}

// Utils
// 通过反射设置tx的nonce为新nonce
func TxNonceSetter(tx *types.Transaction, nonce uint64) {
	type_ := reflect.TypeOf(tx.GetInner()).String()
	if type_ == "*types.LegacyTx" {
		inner := tx.GetInner().(*types.LegacyTx)
		inner.Nonce = nonce
	} else if type_ == "*types.DynamicFeeTx" {
		inner := tx.GetInner().(*types.DynamicFeeTx)
		inner.Nonce = nonce
	} else if type_ == "*types.AccessListTx" {
		inner := tx.GetInner().(*types.AccessListTx)
		inner.Nonce = nonce
	} else if type_ == "*types.BlobTx" {
		inner := tx.GetInner().(*types.BlobTx)
		inner.Nonce = nonce
	} else {
		panic("unknow type:" + type_)
	}
}

// Utils
func check(e error) {
	if e != nil {
		panic(e)
	}
}

// Util
// Test highlight
func hl(s string, c string) string {
	switch c {
	case "red":
		return "\033[31m" + s + "\033[0m"
	case "blue":
		return "\033[34m" + s + "\033[0m"
	case "green":
		return "\033[32m" + s + "\033[0m"
	default:
		return "\033[32m" + s + "\033[0m"
	}
}

// Util
// Time string highlight
func ts() string {
	return hl(time.Now().Format("【2006-01-02 15:04:05.000】"), "")
}

// Debug
// func PrintTxInfo(pending map[common.Address][]*txpool.LazyTransaction) {
// 	for _, txs := range pending {
// 		for _, tx := range txs {
// 			fmt.Println(hl("Tx:", "red"), tx.Hash.Hex())
// 			fmt.Println(hl("Amount of gas required by the transaction", ""), tx.Gas)
// 			fmt.Println(hl("Maximum miner tip per gas the transaction can pay", ""), tx.GasTipCap)
// 			fmt.Println(hl("Gas price of the transaction", ""), tx.GasPrice)
// 			fmt.Println(hl("Maximum fee per gas the transaction may consume", ""), tx.GasFeeCap)
// 		}
// 	}
// }

// Debug
func PrintTxInfo(pending []*types.Transaction) {
	for _, tx := range pending {
		inner := tx.GetInner().(*types.LegacyTx)
		fmt.Println(hl("Gas", ""), inner.Gas)
		fmt.Println(hl("Gas", ""), inner.GasPrice)
	}
}

// Debug Utils
// 从db中获取某个区块的所有tx
func GetBlockTransactionsDB(db ethdb.Database, bc *core.BlockChain, block_num uint64) []*types.Transaction {
	block_hash := rawdb.ReadCanonicalHash(db, block_num)
	block := rawdb.ReadBlock(db, block_hash, block_num)
	var pending_list []*types.Transaction
	for _, tx := range block.Transactions() {
		pending_list = append(pending_list, tx)
	}
	return pending_list
}

// Debug Util
// 进行简单测试要用
func RunSingleBlock(db ethdb.Database, bc *core.BlockChain, block_num uint64, from_rpc bool) {
	former_block_num := block_num - 1
	block_hash := rawdb.ReadCanonicalHash(db, block_num)
	former_block_hash := rawdb.ReadCanonicalHash(db, former_block_num)
	//当前block获取方式
	var block *types.Block
	if !from_rpc {
		block = rawdb.ReadBlock(db, block_hash, block_num)
	} else {
		block = GetBlockRPC(block_num)
	}
	former_block := rawdb.ReadBlock(db, former_block_hash, former_block_num)
	statedb, _ := bc.StateAt(former_block.Root())
	_, _, _, err := bc.Processor().Process(block, statedb, vm.Config{})
	check(err)
}

// 构建从base state（20306538）往后n个区块的difflayer环境
// 同时还能测试构建diffkayer时每个后续block sload命中情况
func RebuildSnapshotDiffLayer(db ethdb.Database, bc *core.BlockChain, block_num uint64, round int) *state.StateDB {

	//加载初始状态
	former_block_num := block_num - 1
	former_block_hash := rawdb.ReadCanonicalHash(db, former_block_num)
	former_block := rawdb.ReadBlock(db, former_block_hash, former_block_num)
	statedb, _ := bc.StateAt(former_block.Root())

	cnt := 0
	for {

		//如果运行次数超过限制则退出
		if cnt == round {
			break
		}

		//获取将要执行区块的信息
		block := GetBlockRPC(block_num)

		//执行区块（并测量时间）
		metric.GLOBAL_HIT_MONITOR.Start()
		start := time.Now()
		_, _, _, err := bc.Processor().Process(block, statedb, vm.Config{})
		end := time.Since(start)
		check(err)
		fmt.Println("Block:", block_num, "Time:", end.Milliseconds(), "ms") //打印区块运行时间
		metric.GLOBAL_HIT_MONITOR.Stop()
		metric.GLOBAL_HIT_MONITOR.OutputRecord(fmt.Sprintf("./log_/sload_log/%d_sload.log", block_num)) //TODO 检查命中情况
		metric.GLOBAL_HIT_MONITOR.Clear()                                                               //监控完一个区块清空全局监视器

		//这里要调用IntermediateRoot，确保调用updateStateObject将account数据更新到statedb.accounts
		statedb.IntermediateRoot(true)

		//根据运行后statedb添加新的difflayer
		//fmt.Println("Snap root", statedb.GetSnap().Root())
		err = statedb.GetSnaps().Update(block.Root(), former_block.Root(), statedb.ConvertAccountSet(statedb.GetStateObjectsDestruct()), statedb.GetAccounts(), statedb.GetStorages())
		check(err)

		//维护状态
		statedb = state.CopyKeyParam(statedb, block.Root())
		cnt += 1
		former_block = block
		block_num += 1
	}
	return statedb
}

// 以下是从RPC获取block所需的一些函数和结构体
// ------------------------------------------------------RPC Request Params and Functions------------------------------------------------------
type ReqTxResRPCBlock struct {
	Id      int      `json:"id"`
	Jsonrpc string   `json:"jsonrpc"`
	Result  rpcBlock `json:"result"`
}

type ReqTxResRPCHeader struct {
	Id      int          `json:"id"`
	Jsonrpc string       `json:"jsonrpc"`
	Result  types.Header `json:"result"`
}

// TransactionArgs represents the arguments to construct a new transaction
// or a message call.
type TransactionArgs struct {
	From                 *common.Address `json:"from"`
	To                   *common.Address `json:"to"`
	Gas                  *hexutil.Uint64 `json:"gas"`
	GasPrice             *hexutil.Big    `json:"gasPrice"`
	MaxFeePerGas         *hexutil.Big    `json:"maxFeePerGas"`
	MaxPriorityFeePerGas *hexutil.Big    `json:"maxPriorityFeePerGas"`
	Value                *hexutil.Big    `json:"value"`
	Nonce                *hexutil.Uint64 `json:"nonce"`

	// We accept "data" and "input" for backwards-compatibility reasons.
	// "input" is the newer name and should be preferred by clients.
	// Issue detail: https://github.com/ethereum/go-ethereum/issues/15628
	Data  *hexutil.Bytes `json:"data"`
	Input *hexutil.Bytes `json:"input"`

	// Introduced by AccessListTxType transaction.
	AccessList *types.AccessList `json:"accessList,omitempty"`
	ChainID    *hexutil.Big      `json:"chainId,omitempty"`

	// For BlobTxType
	BlobFeeCap *hexutil.Big  `json:"maxFeePerBlobGas"`
	BlobHashes []common.Hash `json:"blobVersionedHashes,omitempty"`

	// For BlobTxType transactions with blob sidecar
	Blobs       []kzg4844.Blob       `json:"blobs"`
	Commitments []kzg4844.Commitment `json:"commitments"`
	Proofs      []kzg4844.Proof      `json:"proofs"`

	V *hexutil.Big `json:"v"` //Brian Add
	R *hexutil.Big `json:"r"` //Brian Add
	S *hexutil.Big `json:"s"` //Brian Add

	// This configures whether blobs are allowed to be passed.
	blobSidecarAllowed bool
}

type rpcBlock struct {
	Hash         common.Hash       `json:"hash"`
	Transactions []TransactionArgs `json:"transactions"`
}

// RPC地址
var RPC_URL string = "http://10.119.187.21:8545"

// 使用外部RPC接口获取区块信息
func GetBlockRPC(block_num uint64) *types.Block {
	var pending_list []*types.Transaction
	block_num_hex := fmt.Sprintf("0x%x", block_num)
	//fmt.Println(block_num_hex)

	// 要发送的请求数据
	requestData := map[string]interface{}{
		"method":  "eth_getBlockByNumber",
		"params":  []interface{}{block_num_hex, true}, //The method returns the full transaction objects when this value is true otherwise, it returns only the hashes of the transactions
		"id":      1,
		"jsonrpc": "2.0",
	}
	// 将数据编码为 JSON
	jsonData, err := json.Marshal(requestData)
	if err != nil {
		log.Fatalf("Error encoding JSON: %v", err)
	}
	// 发送 POST 请求
	response, err := http.Post(RPC_URL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Fatalf("Error sending POST request: %v", err)
	}
	defer response.Body.Close()
	//拷贝两份原io输入数据,因为要解码两次，分别解码到不同结构体
	var buf bytes.Buffer
	_, err = io.Copy(&buf, response.Body)
	check(err)
	responseBody1 := bytes.NewReader(buf.Bytes())
	responseBody2 := bytes.NewReader(buf.Bytes())

	//解码response
	//解码区块体参数
	var rpc_block ReqTxResRPCBlock
	if err := json.NewDecoder(responseBody1).Decode(&rpc_block); err != nil {
		log.Fatalf("Failed to decode response: %v", err)
	}
	//解码区块头参数
	var rpc_header ReqTxResRPCHeader
	if err := json.NewDecoder(responseBody2).Decode(&rpc_header); err != nil {
		log.Fatalf("Failed to decode response: %v", err)
	}
	//转化Tx格式（TransactionArgs -> types.Transaction）
	for _, tx := range rpc_block.Result.Transactions {
		pending_list = append(pending_list, tx.toTransaction())
	}

	//根据区块头和tx list构建Block结构体
	block := types.NewBlock(&rpc_header.Result, pending_list, nil, nil, trie.NewStackTrie(nil))
	//fmt.Println(block)

	return block
}

// 从geth复制过来的
// toTransaction converts the arguments to a transaction.
// This assumes that setDefaults has been called.
func (args *TransactionArgs) toTransaction() *types.Transaction {
	var data types.TxData
	switch {
	case args.BlobHashes != nil:
		al := types.AccessList{}
		if args.AccessList != nil {
			al = *args.AccessList
		}
		data = &types.BlobTx{
			To:         *args.To,
			ChainID:    uint256.MustFromBig((*big.Int)(args.ChainID)),
			Nonce:      uint64(*args.Nonce),
			Gas:        uint64(*args.Gas),
			GasFeeCap:  uint256.MustFromBig((*big.Int)(args.MaxFeePerGas)),
			GasTipCap:  uint256.MustFromBig((*big.Int)(args.MaxPriorityFeePerGas)),
			Value:      uint256.MustFromBig((*big.Int)(args.Value)),
			Data:       args.data(),
			AccessList: al,
			BlobHashes: args.BlobHashes,
			BlobFeeCap: uint256.MustFromBig((*big.Int)(args.BlobFeeCap)),
			V:          new(uint256.Int).SetBytes((*big.Int)(args.V).Bytes()), //Brian Add
			R:          new(uint256.Int).SetBytes((*big.Int)(args.R).Bytes()), //Brian Add
			S:          new(uint256.Int).SetBytes((*big.Int)(args.S).Bytes()), //Brian Add
		}
		if args.Blobs != nil {
			data.(*types.BlobTx).Sidecar = &types.BlobTxSidecar{
				Blobs:       args.Blobs,
				Commitments: args.Commitments,
				Proofs:      args.Proofs,
			}
		}

	case args.MaxFeePerGas != nil:
		al := types.AccessList{}
		if args.AccessList != nil {
			al = *args.AccessList
		}
		data = &types.DynamicFeeTx{
			To:         args.To,
			ChainID:    (*big.Int)(args.ChainID),
			Nonce:      uint64(*args.Nonce),
			Gas:        uint64(*args.Gas),
			GasFeeCap:  (*big.Int)(args.MaxFeePerGas),
			GasTipCap:  (*big.Int)(args.MaxPriorityFeePerGas),
			Value:      (*big.Int)(args.Value),
			Data:       args.data(),
			AccessList: al,
			V:          (*big.Int)(args.V), //Brian Add
			R:          (*big.Int)(args.R), //Brian Add
			S:          (*big.Int)(args.S), //Brian Add
		}

	case args.AccessList != nil:
		data = &types.AccessListTx{
			To:         args.To,
			ChainID:    (*big.Int)(args.ChainID),
			Nonce:      uint64(*args.Nonce),
			Gas:        uint64(*args.Gas),
			GasPrice:   (*big.Int)(args.GasPrice),
			Value:      (*big.Int)(args.Value),
			Data:       args.data(),
			AccessList: *args.AccessList,
			V:          (*big.Int)(args.V), //Brian Add
			R:          (*big.Int)(args.R), //Brian Add
			S:          (*big.Int)(args.S), //Brian Add
		}

	default:
		data = &types.LegacyTx{
			To:       args.To,
			Nonce:    uint64(*args.Nonce),
			Gas:      uint64(*args.Gas),
			GasPrice: (*big.Int)(args.GasPrice),
			Value:    (*big.Int)(args.Value),
			Data:     args.data(),
			V:        (*big.Int)(args.V), //Brian Add（这里要初始化一下V R S不然后面检查过不了）
			R:        (*big.Int)(args.R), //Brian Add
			S:        (*big.Int)(args.S), //Brian Add
		}
		//fmt.Println("New Tx data:", args.To, uint64(*args.Nonce), uint64(*args.Gas), (args.GasPrice), args.Value, args.data()) // Brian Add
	}

	return types.NewTx(data)
}

// 从geth复制过来的
// data retrieves the transaction calldata. Input field is preferred.
func (args *TransactionArgs) data() []byte {
	if args.Input != nil {
		return *args.Input
	}
	if args.Data != nil {
		return *args.Data
	}
	return nil
}
