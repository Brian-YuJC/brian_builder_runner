package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"sort"
	"strings"

	"github.com/ethereum/go-ethereum/common"
)

func check(e error) {
	if e != nil {
		panic(e)
	}
}

type TouchAddress struct {
	//Address   common.Address
	InvokeCnt uint64
	LabelInfo LabelInfo
	// Label     []string
	// Tag       string //Accounr use only
	// Name      string //Token use only
	// Symbol    string //Token use only
	// Type      string //Account or Token
	KeyMap map[common.Hash]int
}

// // 提取我们需要的数据
// type CleanMEVTx struct {
// 	BlockNum         uint64
// 	Hash             common.Hash
// 	MEVType          string
// 	Protocol         string
// 	UserSwapCnt      int
// 	ExtractorSwapCnt int
// 	From             common.Address
// 	To               common.Address
// }

// // 单笔arbitrage交易
// type Arbitrage struct {
// 	Tx              CleanMEVTx `json:"Tx"`
// 	TouchAddressMap map[common.Address]TouchAddress
// }

// type Sandwich struct {
// 	FrontRun        CleanMEVTx
// 	VictimTx        []CleanMEVTx
// 	BackRun         CleanMEVTx
// 	TouchAddressMap map[common.Address]TouchAddress
// }

// 用于读sandwich或arbitrage结构体中的TouchAddressMap
type ReadObj struct {
	TouchAddressMap map[common.Address]TouchAddress
}

type TokenLabel struct {
	Address string `json:"address"`
	ChainID int    `json:"chainId"`
	Label   string `json:"label"`
	Name    string `json:"name"`
	Symbol  string `json:"symbol"`
}

type AccountLabel struct {
	Address string `json:"address"`
	ChainID int    `json:"chainId"`
	Label   string `json:"label"`
	Tag     string `json:"nameTag"`
}

type LabelInfo struct {
	Label  []string
	Tag    string //Accounr use only
	Name   string //Token use only
	Symbol string //Token use only
	Type   string //Account or Token
}

// 以LabelInfo格式读取标签
func ReadLabel(file_name string, type_ string) {
	input, err := os.Open("./" + file_name + ".json")
	check(err)
	defer input.Close()
	input_bytes, err := ioutil.ReadAll(input)
	check(err)
	var token_label_list = make([]TokenLabel, 0)
	var account_label_list = make([]AccountLabel, 0)

	if type_ == "Token" {
		err = json.Unmarshal([]byte(input_bytes), &token_label_list)
		check(err)
		for _, label := range token_label_list {
			label.Address = strings.ToLower(label.Address) //转小写
			if label.ChainID != 1 {                        //不是以太坊
				continue
			}
			if _, ok := LABEL_MAP[label.Address]; !ok {
				LABEL_MAP[label.Address] = LabelInfo{Label: make([]string, 0), Type: "Token"}
			}
			obj := LABEL_MAP[label.Address]
			obj.Label = append(obj.Label, label.Label)
			obj.Name = label.Name
			obj.Symbol = label.Symbol
			LABEL_MAP[label.Address] = obj
		}
	} else if type_ == "Account" {
		err = json.Unmarshal([]byte(input_bytes), &account_label_list)
		check(err)
		for _, label := range account_label_list {
			label.Address = strings.ToLower(label.Address) //转小写
			if label.ChainID != 1 {                        //不是以太坊
				continue
			}
			if _, ok := LABEL_MAP[label.Address]; !ok {
				LABEL_MAP[label.Address] = LabelInfo{Label: make([]string, 0), Type: "Account"}
			}
			obj := LABEL_MAP[label.Address]
			obj.Label = append(obj.Label, label.Label)
			obj.Tag = label.Tag
			LABEL_MAP[label.Address] = obj
		}
	} else {
		panic("Illigal type")
	}

}

// 读取 mev tx 的touch address map 到 TOUCH_ADDRESS_LIST
func ReadTouchedAddressMap(path string) {
	// 读取文件到DATA
	input, err := os.Open(path)
	check(err)
	defer input.Close()
	input_bytes, err := ioutil.ReadAll(input)
	check(err)
	var arb_list []ReadObj
	err = json.Unmarshal([]byte(input_bytes), &arb_list)
	check(err)
	fmt.Println("Arb or liquid list length:", len(arb_list))

	for _, arb := range arb_list {
		TOUCH_ADDRESS_LIST = append(TOUCH_ADDRESS_LIST, arb.TouchAddressMap)
	}
}

// // 读取 mev tx 的touch address map 到 TOUCH_ADDRESS_LIST
// func ReadSandwichTouchedAddressMap(path string) {
// 	// 读取文件到DATA
// 	input, err := os.Open(path)
// 	check(err)
// 	defer input.Close()
// 	input_bytes, err := ioutil.ReadAll(input)
// 	check(err)
// 	var sandwich_list []Sandwich
// 	err = json.Unmarshal([]byte(input_bytes), &sandwich_list)
// 	check(err)
// 	fmt.Println("Sandwich list length:", len(sandwich_list))

// 	for _, bundle := range sandwich_list {
// 		TOUCH_ADDRESS_LIST = append(TOUCH_ADDRESS_LIST, bundle.TouchAddressMap)
// 	}
// }

// 用于排序的对象
type SortObj struct {
	Address    common.Address
	Count      uint64
	LabelInfo  LabelInfo
	Proportion float64
}

// 统计一个Address被多少个MEV tx共有，及其比例
func GetCommonAddress(path string) {
	var map_ = make(map[common.Address]uint64)
	for _, touch_map := range TOUCH_ADDRESS_LIST {
		for address, _ := range touch_map {
			if _, ok := map_[address]; !ok {
				map_[address] = 0
			}
			map_[address] += 1
		}
	}
	total_mev_sample := len(TOUCH_ADDRESS_LIST)
	var sorted_with_label = make([]SortObj, 0)
	for address, cnt := range map_ {
		sorted_with_label = append(sorted_with_label, SortObj{Address: address, Count: cnt, LabelInfo: LABEL_MAP[strings.ToLower(address.Hex())], Proportion: float64(cnt) / float64(total_mev_sample) * 100})
	}
	sort.Slice(sorted_with_label, func(i, j int) bool { //排序，大的在前
		return sorted_with_label[i].Count > sorted_with_label[j].Count
	})

	//输出
	jsonData, err := json.MarshalIndent(sorted_with_label, "", "  ")
	check(err)
	//fmt.Println(string(jsonData))
	output, err := os.Create(path)
	check(err)
	output.Write(jsonData)
}

// 统计被调用次数最多的
func GetTopInvokeAddress(path string) {
	var address_invoke_map = make(map[common.Address]uint64)
	var total_invoke_cnt = uint64(0)
	for _, touch_map := range TOUCH_ADDRESS_LIST {
		for address, value := range touch_map {
			if _, ok := address_invoke_map[address]; !ok {
				address_invoke_map[address] = 0
			}
			address_invoke_map[address] += value.InvokeCnt
			total_invoke_cnt += value.InvokeCnt
		}
	}
	var sorted_with_label = make([]SortObj, 0)
	for address, cnt := range address_invoke_map {
		sorted_with_label = append(sorted_with_label, SortObj{Address: address, Count: cnt, LabelInfo: LABEL_MAP[strings.ToLower(address.Hex())], Proportion: float64(cnt) / float64(total_invoke_cnt) * 100})
	}
	sort.Slice(sorted_with_label, func(i, j int) bool { //排序，大的在前
		return sorted_with_label[i].Count > sorted_with_label[j].Count
	})

	//输出
	jsonData, err := json.MarshalIndent(sorted_with_label, "", "  ")
	check(err)
	//fmt.Println(string(jsonData))
	output, err := os.Create(path)
	check(err)
	output.Write(jsonData)

}

var INPUT_PATH = "./output/"
var OUTPUT_PATH = "./output/"
var TOUCH_ADDRESS_LIST = make([]map[common.Address]TouchAddress, 0)
var LABEL_MAP = make(map[string]LabelInfo)

func main() {

	ReadLabel("tokens", "Token")
	ReadLabel("accounts", "Account")
	fmt.Println("Label map length:", len(LABEL_MAP))

	//往TOUCH_ADDRESS_LIST里面添加东西，好处是可以选择统计哪几种mev的数据，需要那种就读取那种的到TOUCH_ADDRESS_LIST
	ReadTouchedAddressMap(INPUT_PATH + "19731000_20350000_clean_liquid_touched" + ".json")
	//ReadTouchedAddressMap(INPUT_PATH + "19731000_19741000_clean_arb_touched" + ".json")
	//ReadTouchedAddressMap(INPUT_PATH + "19731000_19733000_clean_sandwich_touched" + ".json")
	fmt.Println(len(TOUCH_ADDRESS_LIST))

	//GetCommonAddress("./a.json")
	GetTopInvokeAddress("./a.json")
}

//go run analyse_touched_address.go
