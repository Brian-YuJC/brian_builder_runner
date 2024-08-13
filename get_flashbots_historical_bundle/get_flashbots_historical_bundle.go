package main

import (
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/ethereum/go-ethereum/common"
)

var INPUT_PATH string = "./input/mev_blocks/flashbots_info_from_19700000_to_20200000.csv"
var OUTPUT_PATH string = "./output/19700000_20200000_bundle.json"

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

func main() {
	// 打开 CSV 文件
	file, err := os.Open(INPUT_PATH)
	check(err)
	defer file.Close()

	// 创建 CSV 阅读器
	reader := csv.NewReader(file)

	// 读取标题行
	_, err = reader.Read()
	check(err)

	bundle_list := make([]Bundle, 0)
	// 读取数据行
	for { //一行一个block
		record, err := reader.Read()
		if err == io.EOF || record[0] == "" {
			break
		}
		if err != nil {
			check(err)
		}

		// 将 JSON 字符串转换为 map[string]interface{}
		var data map[string]interface{}
		err = json.Unmarshal([]byte(record[1]), &data)
		check(err)

		block_bundle_list := make([]Bundle, 0)
		for _, tx_map := range data["transactions"].([]interface{}) {
			var hash common.Hash
			var from, to common.Address
			tx_hash_string := string(tx_map.(map[string]interface{})["transaction_hash"].(string))
			tx_hash_byte, err := hex.DecodeString(tx_hash_string[2:])
			check(err)
			hash.SetBytes(tx_hash_byte)

			tx_from_string := string(tx_map.(map[string]interface{})["eoa_address"].(string))
			tx_from_byte, err := hex.DecodeString(tx_from_string[2:])
			check(err)
			from.SetBytes(tx_from_byte)

			tx_to_string := string(tx_map.(map[string]interface{})["to_address"].(string))
			tx_to_byte, err := hex.DecodeString(tx_to_string[2:])
			check(err)
			to.SetBytes(tx_to_byte)

			var bundle_type string = string(tx_map.(map[string]interface{})["bundle_type"].(string))
			var bundle_index int = int(tx_map.(map[string]interface{})["bundle_index"].(float64))
			var tx_index int = int(tx_map.(map[string]interface{})["tx_index"].(float64))

			transaction := Transaction{Hash: hash, From: from, To: to, BundleType: bundle_type, BundleIndex: bundle_index, TxIndex: tx_index}
			if len(block_bundle_list) == bundle_index { //发现一个新bundle
				block_bundle_list = append(block_bundle_list, Bundle{BlockNumber: record[0], Tx: make([]Transaction, 0)})
			}
			block_bundle_list[bundle_index].Tx = append(block_bundle_list[bundle_index].Tx, transaction)
		}
		bundle_list = append(bundle_list, block_bundle_list...)

		// 输出成json格式
		jsonData, err := json.MarshalIndent(bundle_list, "", "  ")
		if err != nil {
			fmt.Println("Error:", err)
			return
		}
		//fmt.Println(string(jsonData))
		output, err := os.Create(OUTPUT_PATH)
		check(err)
		output.Write(jsonData)
	}
}

//go run get_MEV_block.go
//读取./output/mev_blocks原始数据然后提取Transaction关键信息，并按Bundle输出
