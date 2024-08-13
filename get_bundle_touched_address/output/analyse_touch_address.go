package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"sort"

	"github.com/ethereum/go-ethereum/common"
)

// var INPUT_PATH string = "./test.json"
// var INPUT_PATH string = "./19700000_19800000_touched_address.json"
// var OUTPUT_PATH string = "./19700000_19800000_touched_address_sort.json"
var INPUT_PATH string = "./19700000_20200000_touched_address.json"
var OUTPUT_PATH string = "./19700000_19800000_touched_address_sort.json.debug"
var LABLE_PATH string = "./AllLabels_from_brianleect.json"

func check(e error) {
	if e != nil {
		panic(e)
	}
}

// 用于方便数据分析创建的结构体
type KeyTouchData struct {
	Address    common.Address //属于哪个Smart contract
	Label      AddressLabel
	Key        common.Hash
	TouchCount int
}

// 根据BrianLeect的label数据构建的结构体
type AddressLabel struct {
	Name  string   `json:"name"`
	Label []string `json:"labels"`
}

// 用于方便数据分析创建的结构体
type AddressTouchData struct {
	Address    common.Address
	Label      AddressLabel
	InvokeCnt  uint64
	Proportion float64        //占比,暂时没用到 TODO
	KeyList    []KeyTouchData //用于排序后的Keylist
}

// 用于读取文件，映射json对象创建的结构体
type TouchAddress struct {
	//Address   common.Address
	InvokeCnt uint64
	KeyMap    map[common.Hash]int
	//KeyList []KeyTouchCount
}

var SortedTouchAddressList []AddressTouchData       //排过序的Address访问结果， 以Address为视角排序
var TotalAddressTouchCnt uint64 = 0                 //总共Address访问的次数，用于计算某些Address访问数量的占比
var TotalKeyTouchCnt uint64 = 0                     //总共Key访问的次数，用于计算某些Key访问数量的占比
var SortedTouchKeyList []KeyTouchData               //排序过的key访问结果，以Key为视角排序
var AddressLabelMap map[common.Address]AddressLabel //address和其label的映射

func ReadLable() {
	input, err := os.Open(LABLE_PATH)
	check(err)
	defer input.Close()

	// 读取文件内容
	input_bytes, err := ioutil.ReadAll(input)
	check(err)

	json.Unmarshal([]byte(input_bytes), &AddressLabelMap)

	//fmt.Println(AddressLabelMap["0xd9fb12fa2886e5ff6280e77bb5f0f518889327d4"].Name) //Test
}

// 获取address对应的标签
func (at *AddressTouchData) GetLabel() {
	//fmt.Println(at.Address)
	if _, ok := AddressLabelMap[at.Address]; !ok {
		at.Label = AddressLabel{Name: "Unknow", Label: nil}
		return
	}
	at.Label = AddressLabelMap[at.Address]
}

// 获取address对应的标签
func (kt *KeyTouchData) GetLabel() {
	//fmt.Println(kt.Address)
	if _, ok := AddressLabelMap[kt.Address]; !ok {
		kt.Label = AddressLabel{Name: "Unknow", Label: nil}
		return
	}
	kt.Label = AddressLabelMap[kt.Address]
}

func ReadAndSort() {
	input, err := os.Open(INPUT_PATH)
	check(err)
	defer input.Close()

	// 读取文件内容
	input_bytes, err := ioutil.ReadAll(input)
	check(err)

	var touch_map map[common.Address]TouchAddress
	json.Unmarshal([]byte(input_bytes), &touch_map)

	for address, data := range touch_map {
		//维护全局变量
		TotalAddressTouchCnt += data.InvokeCnt

		//结构体TouchAddress转AddressTouchData结构体
		//TODO: 获取Address对应的类别（DEX？Liquidation？）
		tmp := AddressTouchData{}
		tmp.Address = address
		tmp.GetLabel() //获取Address对应的类别
		tmp.InvokeCnt = data.InvokeCnt
		tmp.KeyList = make([]KeyTouchData, 0)
		for key, cnt := range data.KeyMap {
			TotalKeyTouchCnt += uint64(cnt) //维护全局变量

			tmp.KeyList = append(tmp.KeyList, KeyTouchData{Address: address, Key: key, TouchCount: cnt})

			//维护全局变量 SortedTouchKeyList
			kt := KeyTouchData{Address: address, Key: key, TouchCount: cnt}
			kt.GetLabel()
			SortedTouchKeyList = append(SortedTouchKeyList, kt)
		}
		sort.Slice(tmp.KeyList, func(i, j int) bool { //address中访问次数最多的key放前面
			return tmp.KeyList[i].TouchCount > tmp.KeyList[j].TouchCount
		})

		SortedTouchAddressList = append(SortedTouchAddressList, tmp)
		//fmt.Println("Done ", address)
	}

	// 访问最多的address排前面
	sort.Slice(SortedTouchAddressList, func(i, j int) bool {
		return SortedTouchAddressList[i].InvokeCnt > SortedTouchAddressList[j].InvokeCnt
	})

	//访问最多的Key排在前面
	sort.Slice(SortedTouchKeyList, func(i, j int) bool {
		return SortedTouchKeyList[i].TouchCount > SortedTouchKeyList[j].TouchCount
	})

	//fmt.Println(SortedTouchAddressList)

	// // 输出成json格式
	// jsonData, err := json.MarshalIndent(SortedTouchAddressList, "", "  ")
	// if err != nil {
	// 	fmt.Println("Error:", err)
	// 	return
	// }
	// //fmt.Println(string(jsonData))
	// output, err := os.Create(OUTPUT_PATH)
	// check(err)
	// output.Write(jsonData)
}

func GetTopTouchAddressInRange(from int, to int) float64 { //top表示需要获取前几位的数据，如果是前十 from=1 to=10
	//处理边界
	if to <= 0 {
		return 0
	}
	if from <= 0 {
		from = 1
	}

	var count uint64 = 0
	for i := from; i <= to; i++ {
		count += SortedTouchAddressList[i-1].InvokeCnt
		fmt.Println("Top", i, SortedTouchAddressList[i-1].Address, "[Address label]", SortedTouchKeyList[i-1].Label, "[Invoke Count]", SortedTouchAddressList[i-1].InvokeCnt)
	}
	var proportion float64 = 0
	proportion = float64(count) / float64(TotalAddressTouchCnt)
	fmt.Println("Proportion of visits:", proportion*100, "%")

	return proportion
}

// // 获取排名第i的Address占比
// func GetAddressProportion(i int) float64 {
// 	return GetTopTouchAddressInRange(i-1, i)
// }

func GetTopTouchKeyInRange(from int, to int) float64 { //top表示需要获取前几位的数据，如果是前十 from=1 to=10
	//处理边界
	if to <= 0 {
		return 0
	}
	if from <= 0 {
		from = 1
	}

	var count uint64 = 0
	for i := from; i <= to; i++ {
		count += uint64(SortedTouchKeyList[i-1].TouchCount)
		fmt.Println("Top", i, SortedTouchKeyList[i-1].Key, "[Belonging Address]", SortedTouchKeyList[i-1].Address, "[Address label]", SortedTouchKeyList[i-1].Label, "[Invoke Count]", SortedTouchKeyList[i-1].TouchCount)
	}
	var proportion float64 = 0
	proportion = float64(count) / float64(TotalKeyTouchCnt)
	fmt.Println("Proportion of visits:", proportion*100, "%")

	return proportion
}

// func GetKeyProportion(i int) float64 {
// 	return GetTopTouchKeyInRange(i-1, i)
// }

func main() {
	ReadLable()
	ReadAndSort()
	GetTopTouchAddressInRange(1, 1000)
	//GetTopTouchKeyInRange(1, 20000)
}
