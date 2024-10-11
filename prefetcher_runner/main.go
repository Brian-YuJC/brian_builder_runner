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

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/txpool"
	"github.com/ethereum/go-ethereum/core/txpool/blobpool"
	"github.com/ethereum/go-ethereum/core/txpool/legacypool"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto/kzg4844"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/miner"
	"github.com/ethereum/go-ethereum/prefetch"
	"github.com/google/uuid"
	"github.com/holiman/uint256"
)

func check(e error) {
	if e != nil {
		panic(e)
	}
}

// -------------------用于接受txpool_content返回值的结构体------------------------------
type TxPoolData struct {
	Data map[string]map[common.Address]map[int]TransactionArgs
}

type ResData struct {
	Id      int                                                   `json:"id"`
	Jsonrpc string                                                `json:"jsonrpc"`
	Result  map[string]map[common.Address]map[int]TransactionArgs `json:"result"`
}

// ----------------------用于接受tx信息返回值的结构体------------------------------------
type TxData struct {
	Data TransactionArgs
}
type ReqTxResData struct {
	Id      int             `json:"id"`
	Jsonrpc string          `json:"jsonrpc"`
	Result  TransactionArgs `json:"result"`
}

var TestBlockChainCacheConfig = &core.CacheConfig{
	TrieCleanLimit:  256,
	TrieDirtyLimit:  256,
	TrieTimeLimit:   5 * time.Minute,
	SnapshotLimit:   256,
	SnapshotWait:    true,
	StateScheme:     rawdb.HashScheme,
	SnapshotNoBuild: true,
}

// 读取disk中的数据库并建立区块链
func GetBlockChain() (ethdb.Database, *core.BlockChain) {
	//datadir := "/home/user/common/docker/volumes/eth-docker_geth-eth1-data/_data/geth/chaindata"
	datadir := "/home/user/common/docker/volumes/cp1_eth-docker_geth-eth1-data/_data/geth/chaindata"
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
		// head_block_hash := common.Hash{}
		//default: 0x0000000000000000000000000000000000000000000000000000000000000000
		// head_block_hash.SetBytes([]byte("0xae6ddd150fb4278af16832bc91f34a81a2aafefde30fecf8da4cdcbcd8ec331f"))
		// rawdb.WriteHeadBlockHash(db, head_block_hash)
		// fmt.Println("Set Head Block Hash!")
		fmt.Println("Blockchain DB Scheme:", rawdb.ReadStateScheme(db))
	}

	bc, err := core.NewBlockChain(db, TestBlockChainCacheConfig, nil, nil, ethash.NewFaker(), vm.Config{}, nil, nil)
	check(err)
	fmt.Println("New Blockchain Success!")

	return db, bc
}

// 测试txpool的功能
// bc所在区块链，update是否将txpool最新的交易加入池中
// 注意获取到的tx会保存在本地
func GetTxPool(bc *core.BlockChain, update bool) *txpool.TxPool {
	//新建txpool
	blobPool := blobpool.New(blobpool.DefaultConfig, bc)
	legacyPool := legacypool.New(legacypool.DefaultConfig, bc)
	//sbundlePool := txpool.NewSBundlePool(params.MainnetChainConfig)
	tx_pool, err := txpool.New(legacypool.DefaultConfig.PriceLimit, bc, []txpool.SubPool{legacyPool, blobPool})
	check(err)

	//需要添加最新pool中的tx
	if update {
		//获取最新的txpool交易，并添加进我们的实验交易池
		txpool_data := GetTxInPool().Data //调用接口
		var txs []*types.Transaction
		for /*type_*/ _, addr_map := range txpool_data { //pending, basefee, queued
			//fmt.Fprintln(os.Stderr, "Type", type_)
			for /*addr*/ _, tx_map := range addr_map {
				//fmt.Fprintln(os.Stderr, "\tSender address", addr)
				for /*nonce*/ _, tx := range tx_map {
					//fmt.Fprintln(os.Stderr, "\t\tNonce:", nonce)
					// fmt.Fprintln(os.Stderr, "\t\t\t",
					// 	"From:", tx.From,
					// 	"To:", tx.To,
					// 	"Gas:", tx.Gas,
					// 	"GasPrice:", tx.GasPrice,
					// 	"MaxFeePerGas:", tx.MaxFeePerGas,
					// 	"MaxPriorityFeePerGas:", tx.MaxPriorityFeePerGas,
					// 	"Value:", tx.Value)
					txs = append(txs, tx.toTransaction())
				}
			}
		}
		fmt.Fprintln(os.Stderr, "Total tx in txpool data:", len(txs))

		//添加进交易池
		err_list := tx_pool.Add(txs, true, false, false)
		fmt.Fprintln(os.Stderr, "Fail to add into pool:", len(err_list))
		for _, err := range err_list { //打印错误信息
			fmt.Println(err)
		}
	}

	//获取pending的tx
	pending := tx_pool.Pending(txpool.PendingFilter{
		MinTip: uint256.MustFromBig(big.NewInt(0)),
	})
	fmt.Println("Total Tx in Pool:", len(pending))

	return tx_pool
}

// 调用RPC的txpool_content接口获取最新的txpool中的tx信息
func GetTxInPool() TxPoolData {
	// 要发送的请求数据
	requestData := map[string]interface{}{
		"method":  "txpool_content",
		"id":      1,
		"jsonrpc": "2.0",
	}
	// 将数据编码为 JSON
	jsonData, err := json.Marshal(requestData)
	if err != nil {
		log.Fatalf("Error encoding JSON: %v", err)
	}
	// 发送 POST 请求
	response, err := http.Post("", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Fatalf("Error sending POST request: %v", err)
	}
	defer response.Body.Close()
	// 检查响应状态
	if response.StatusCode != http.StatusOK {
		log.Fatalf("Error: received status code %d", response.StatusCode)
	}
	// 处理响应
	var responseBody ResData //解码成ResData格式的结构体
	if err := json.NewDecoder(response.Body).Decode(&responseBody); err != nil {
		log.Fatalf("Error decoding response: %v", err)
	}
	return TxPoolData{Data: responseBody.Result} //返回TxPoolData结构体数据
}

// // ！！！！！非常危险的操作！！！！！
// 用于rawdb在检测数据库类型时rawdb.ReadStateScheme(db)将原本的path检测为hash
// 该函数主要检测两个key的位置一个是key为“A”的位置表示AccountTrie的根，和“LastStateID”表示当前状态id
// func DeletePathDB(db ethdb.Database) {
//  //强制删除AccountTrie的根的数据
// 	SavePathDBParam(db)
// 	db.Delete([]byte("A"))
// 	//强制重置stateID为0
// 	var id uint64 = 0
// 	buf := make([]byte, 8)
// 	binary.BigEndian.PutUint64(buf, id)
// 	db.Put([]byte("LastStateID"), buf)
// 	db.Close()
// }

// // 保存PathDB重要的数据
// 用于DeletePathDB时先保存数据
// func SavePathDBParam(db ethdb.Database) {
// 	path := "./pathdb.log"
// 	output, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
// 	check(err)
// 	defer output.Close()
// 	state_id_byte, err := db.Get([]byte("LastStateID"))
// 	check(err)
// 	state_id := hex.EncodeToString(state_id_byte)
// 	account_trie_root_byte, err := db.Get(append([]byte("A"), nil...))
// 	account_trie_root := hex.EncodeToString(account_trie_root_byte)
// 	check(err)
// 	_, err = output.WriteString("LastStateID " + state_id)
// 	check(err)
// 	_, err = output.WriteString("AccountTrieRoot " + account_trie_root)
// 	check(err)
// }

// 根据哈希获取block数据
// 目的是为了获取已经打包的tx进行实验
func GetTxByBlockNumberAndIndex(block_num uint64, tx_index uint64) TxData {
	// 要发送的请求数据
	requestData := map[string]interface{}{
		"method":  "eth_getTransactionByBlockNumberAndIndex",
		"params":  []string{"0xc5043f", "0x0"}, //[]string{string(block_num), string(tx_index)},
		"id":      1,
		"jsonrpc": "2.0",
	}
	// 将数据编码为 JSON
	jsonData, err := json.Marshal(requestData)
	if err != nil {
		log.Fatalf("Error encoding JSON: %v", err)
	}
	// 发送 POST 请求
	response, err := http.Post("", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Fatalf("Error sending POST request: %v", err)
	}
	defer response.Body.Close()
	// 检查响应状态
	if response.StatusCode != http.StatusOK {
		log.Fatalf("Error: received status code %d", response.StatusCode)
	}
	// 处理响应
	var responseBody ReqTxResData
	if err := json.NewDecoder(response.Body).Decode(&responseBody); err != nil {
		log.Fatalf("Error decoding response: %v", err)
	}
	//fmt.Println(responseBody)
	return TxData{responseBody.Result} //返回TxPoolData结构体数据
}

// 方法一通过MEV API添加bundle
// 尝试添加bundle信息到txpool
func AddMEVBundleTest(bc *core.BlockChain, txs []*types.Transaction, blockNumber uint64) {
	//新建txpool
	blobPool := blobpool.New(blobpool.DefaultConfig, bc)
	legacyPool := legacypool.New(legacypool.DefaultConfig, bc)
	//sbundlePool := txpool.NewSBundlePool(params.MainnetChainConfig)
	tx_pool, err := txpool.New(legacypool.DefaultConfig.PriceLimit, bc, []txpool.SubPool{legacyPool, blobPool})
	check(err)

	// 尝试添加bundle到txpool
	// txs -> Array[String], A list of signed transactions to execute in an atomic bundle
	// blockNumber -> String, a hex encoded block number for which this bundle is valid on
	var replacementUuid uuid.UUID // (Optional) String, UUID that can be used to cancel/replace this bundle
	var signingAddress common.Address
	var minTimestamp uint64             // (Optional) Number, the minimum timestamp for which this bundle is valid, in seconds since the unix epoch
	var maxTimestamp uint64             // (Optional) Number, the maximum timestamp for which this bundle is valid, in seconds since the unix epoch
	var revertingTxHashes []common.Hash // (Optional) Array[String], A list of tx hashes that are allowed to revert
	err = tx_pool.AddMevBundle(txs, new(big.Int).SetUint64(blockNumber), replacementUuid, signingAddress, minTimestamp, maxTimestamp, revertingTxHashes)
	check(err)

	var blockTimestamp uint64
	mev_bundle, _ := tx_pool.MevBundles(new(big.Int).SetUint64(blockNumber), blockTimestamp)
	fmt.Println("MEV bundle in txpool cnt:", len(mev_bundle))
}

// // 方法二：通过flashbotsextra.IDatabaseService添加bundle
// // TODO
// func AddMEVbundleToDB() {
// 	var ds flashbotsextra.IDatabaseService
// 	dbDSN := ""
// 	ds, err := flashbotsextra.NewDatabaseService(dbDSN)
// 	check(err)
// }

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
		//fmt.Println(bundle.Txs)
	}
	return res
}

// 通过Transaction获取Nonce
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
	} else {
		panic("unknow type")
	}
	return nonce
}

// 设置tx的nonce为新nonce
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
	} else {
		panic("unknow type")
	}
}

// 基于某个状态重构bundle中tx的Nonce
// 如果同一个发送者有多条tx在MEVbundle pool，Nonce全部一样都是重构为当前状态下发送者的下一个Nonce
func ReconstructNonce(bc *core.BlockChain, bundles []types.MevBundle, state *state.StateDB, header *types.Header) {
	config := bc.GetChainConfig()
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

// 基于某个状态重构bundle中tx的Nonce
func ReconstructNonceEnhence(bc *core.BlockChain, bundles []types.MevBundle, state *state.StateDB, header *types.Header) {
	config := bc.GetChainConfig()
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

func main() {
	// 功能：建立Blockchain
	db, bc := GetBlockChain()

	// 功能：调用接口获取最新的pending tx
	//GetTxPool(bc, true)

	// 功能：数据库Path模式转换为Hash模式
	//SavePathDBParam(db)
	//DeletePathDB(db)

	// 测试：获取当前状态stateDB
	// start_block_num := uint64(9900000)
	// start_block_hash := rawdb.ReadCanonicalHash(db, start_block_num)
	// start_block := rawdb.ReadBlock(db, start_block_hash, start_block_num)
	// state, err := bc.StateAt(start_block.Root())
	// header := start_block.Header()
	// // //fmt.Println(start_block.Hash())
	// check(err)
	// for _, tx := range start_block.Transactions() {
	// 	GetTxByHash(tx.Hash())
	// }
	// header := bc.CurrentBlock()
	// fmt.Println("Current Header:", header.Number)
	// // ⭐️TODO:现在的问题是数据库的current block是第0个块，导致gas limit只有5000，导致当前最新的txpool的交易没法放进池里

	// 功能：log and metric
	prefetch.LOG.Init()

	// 功能：模拟运行builder打包区块流程
	//dataset_path := "./dataset/assembled_bundle/19731189_20260000_0_526089.csv"
	dataset_path := "./dataset/assembled_bundle/9000000_10000000_0_25277.csv"
	next_block_number := bc.CurrentBlock().Number.Uint64() + 1
	bundles := ReadBundleDatasetCSV(dataset_path, next_block_number)
	state, err := bc.State()
	header := bc.CurrentHeader()
	check(err)
	ReconstructNonce(bc, bundles, state, header)
	//ReconstructNonceEnhence(bc, bundles, state, header)
	fmt.Println("Bundle in Dataset:", len(bundles))
	//运行builder
	miner.RunBuilder(db, bc, bundles)

	//功能：log and metric
	prefetch.PrintLogLinear(prefetch.LOG)
	//prefetch.DO_TOUCH_ADDR_LOG = false
	//GetTxSloadLog(db, bc, 19736427, "0x06ce016d1820e0616283a81b814b2bbd3c99d334bae0346a0456c8d0869f650a")

	// 测试：源码的按MevGasPrice排序测试
	// var simulatedBundles []types.SimulatedBundle
	// var a uint256.Int
	// a.SetFromBig(new(big.Int).SetUint64(3))
	// var b uint256.Int
	// b.SetFromBig(new(big.Int).SetUint64(1))
	// var c uint256.Int
	// c.SetFromBig(new(big.Int).SetUint64(2))
	// simulatedBundles = append(simulatedBundles, types.SimulatedBundle{MevGasPrice: &a})
	// simulatedBundles = append(simulatedBundles, types.SimulatedBundle{MevGasPrice: &b})
	// simulatedBundles = append(simulatedBundles, types.SimulatedBundle{MevGasPrice: &c})
	// sort.SliceStable(simulatedBundles, func(i, j int) bool {
	// 	return simulatedBundles[j].MevGasPrice.Cmp(simulatedBundles[i].MevGasPrice) < 0
	// })
	// for _, d := range simulatedBundles {
	// 	fmt.Println(d.MevGasPrice)
	// }

	// 测试：往txpool里添加bundle
	// res := GetTxByBlockNumberAndIndex(start_block.NumberU64(), 0)
	// AddMEVBundleTest(bc, []*types.Transaction{res.Data.toTransaction()}, 900000320)
	// AddMEVBundleTest(bc, bundles[0].Txs, 900000320)

	// 测试：go tx16进制表示解码
	// //tx的字节流可以通过get_raw_transaction_by_block rpc接口得到
	// //字节流转化为16进制字符串方便保存
	// tx_hex := "0x02f905340182053f840131a6c88505c8021c8283045f98943fc91a3afd70395cd496c647d5a6cc9d4b2b7fad80b904c43593564c000000000000000000000000000000000000000000000000000000000000006000000000000000000000000000000000000000000000000000000000000000a000000000000000000000000000000000000000000000000000000000662a884f00000000000000000000000000000000000000000000000000000000000000040a08060c00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000400000000000000000000000000000000000000000000000000000000000000800000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000032000000000000000000000000000000000000000000000000000000000000003a00000000000000000000000000000000000000000000000000000000000000160000000000000000000000000fdc58bbd4359c9d9b7ba2bcb3529366cb9cfb9e1000000000000000000000000ffffffffffffffffffffffffffffffffffffffff00000000000000000000000000000000000000000000000000000000665212fc00000000000000000000000000000000000000000000000000000000000000000000000000000000000000003fc91a3afd70395cd496c647d5a6cc9d4b2b7fad00000000000000000000000000000000000000000000000000000000662a8d0400000000000000000000000000000000000000000000000000000000000000e00000000000000000000000000000000000000000000000000000000000000041cd76019087592af626b4fe0c6a06e5603ad290a9fec31284988a9755129f81823070f6151508d34fda5f330dbb434cca70cd3482411fde1534ff70ee8409c6c51c000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000100000000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000002ed7df418960d6c9a12590000000000000000000000000000000000000000000000000280183a27a3c9bb00000000000000000000000000000000000000000000000000000000000000a000000000000000000000000000000000000000000000000000000000000000010000000000000000000000000000000000000000000000000000000000000002000000000000000000000000fdc58bbd4359c9d9b7ba2bcb3529366cb9cfb9e1000000000000000000000000c02aaa39b223fe8d0a0e5c4f27ead9083c756cc20000000000000000000000000000000000000000000000000000000000000060000000000000000000000000c02aaa39b223fe8d0a0e5c4f27ead9083c756cc200000000000000000000000037a8f295612602f2774d331e562be9e61b83a327000000000000000000000000000000000000000000000000000000000000001900000000000000000000000000000000000000000000000000000000000000400000000000000000000000000000000000000000000000000000000000000001000000000000000000000000000000000000000000000000027e7e910ca92377c001a0ec481aa8702bfa5251fbf3158713fb53de42f8f75b10f18fd27fccab3026d55fa0773d89eff11b537f6a636324065177f17e8da0292ea8a8df86e692f930d68153"
	// tx_byte, err := hex.DecodeString(tx_hex[2:])
	// check(err)
	// tx := new(types.Transaction)
	// err = tx.UnmarshalBinary(tx_byte)
	// check(err)
}

//---------------------------------------------------------以下为Geth中的Internal结构体和函数这里拿出来调用----------------------------------------------------
//有些结构体和函数更改过
// // RPCTransaction represents a transaction that will serialize to the RPC representation of a transaction
// type RPCTransaction struct {
// 	BlockHash   *common.Hash   `json:"blockHash"`
// 	BlockNumber *hexutil.Big   `json:"blockNumber"`
// 	From        common.Address `json:"from"`
// 	Gas         hexutil.Uint64 `json:"gas"`
// 	GasPrice    *hexutil.Big   `json:"gasPrice"`
// 	//GasFeeCap           *hexutil.Big      `json:"maxFeePerGas,omitempty"`
// 	GasTipCap *hexutil.Big `json:"maxPriorityFeePerGas,omitempty"`
// 	//MaxFeePerBlobGas    *hexutil.Big      `json:"maxFeePerBlobGas,omitempty"`
// 	Hash             common.Hash       `json:"hash"`
// 	Input            hexutil.Bytes     `json:"input"`
// 	Nonce            hexutil.Uint64    `json:"nonce"`
// 	To               *common.Address   `json:"to"`
// 	TransactionIndex *hexutil.Uint64   `json:"transactionIndex"`
// 	Value            *hexutil.Big      `json:"value"`
// 	Type             hexutil.Uint64    `json:"type"`
// 	Accesses         *types.AccessList `json:"accessList,omitempty"`
// 	ChainID          *hexutil.Big      `json:"chainId,omitempty"`
// 	//BlobVersionedHashes []common.Hash     `json:"blobVersionedHashes,omitempty"`
// 	V *hexutil.Big `json:"v"`
// 	R *hexutil.Big `json:"r"`
// 	S *hexutil.Big `json:"s"`
// 	//YParity             *hexutil.Uint64   `json:"yParity,omitempty"`
// }

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
