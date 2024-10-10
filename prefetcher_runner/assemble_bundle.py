import csv
import datetime
import time
from web3 import Web3

#链接到RPC
w3 = Web3(Web3.HTTPProvider(''))
print(w3.is_connected())

#Transaction 类
class Transaction:
    def __init__(self, tx_index, mev_type):
        self.tx_index = tx_index #int
        self.mev_type = mev_type

#block 类
class Block:
    def __init__(self, block_number):
        self.block_number = block_number #int
        self.txs = []
    def AddTx(self, tx):
        self.txs.append(tx)

#bundle 类
class Bundle:
    def __init__(self, mev_type):
        self.mev_type = mev_type
        self.txs = [] #这里的面装的是16进制字符串表示的Raw Tx
    def AddTx(self, tx):
        self.txs.append(tx)
    def ToRow(self):
        return [self.mev_type] + self.txs

def ReadMEVBundleCSV(inptut_path):
    global ALL_BLOCK_DATA
    with open(inptut_path, 'r') as input:
        reader = csv.reader(input)
        is_header = True
        current_block = Block(-1) 
        for row in reader:
            if is_header: #读入表头
                is_header = False
                continue
            block_number = int(row[0])
            tx_index = int(row[1])
            mev_type = row[2]
            tx = Transaction(tx_index, mev_type)
            if block_number != current_block.block_number: #该tx不属于当前正在写入的block
                if current_block.block_number != -1: #边界情况处理
                    ALL_BLOCK_DATA.append(current_block) #将当前block全部tx已经读取完毕写入全局变量
                current_block = Block(block_number) #新建block实例
            current_block.AddTx(tx) #写入tx

#以block为单位，还原bundle
#一般liquid，arbi，swap都是一个bundle一个
#sandwich包含front_run, victim txs 和 back_run
#函数处理ALL_BLOCK_DATA里面范围在：[from_, to_]的闭区间的数据（方便以后并行）
#一个block内bundle组装原则：如果mev_type为swap arb liquid则一个tx一个bundle（无论这些交易是否夹在某sandwich中间，都独立组装为一个bundle）
#                        对于sandwich：从frontrun开始到最近一个backrun结束，以及中间所有的标签为sandwich的交易都打包进同一个bundle
#                                     对于没有frontrun匹配的backrun交易我们忽略它
#                                     对于没有backrun匹配的frontrun交易我们也忽略它
def AssembleBundle(from_, to_, output_path):
    global ALL_BLOCK_DATA
    with open(output_path, 'w') as output:
        writer = csv.writer(output)
        for block in ALL_BLOCK_DATA[from_:to_+1]: #遍历block
            sandwich_bundle = None
            for tx in block.txs: #遍历block里面的tx
                #只要出现swap arb liquid就打包成bundle
                if tx.mev_type == 'swap' or  tx.mev_type == 'arb' or  tx.mev_type == 'liquid':
                    bundle = Bundle(tx.mev_type) #单独组装bundle
                    bundle.AddTx(GetTxByBlockNumberAndIndex(block.block_number, tx.tx_index))
                    writer.writerow(bundle.ToRow()) #输出
                elif tx.mev_type == 'frontrun':
                    if sandwich_bundle == None: #如果当前没有正在进行的sandwich组装工作
                        sandwich_bundle = Bundle('sandwich') #新开启一个组装工作
                        sandwich_bundle.AddTx(GetTxByBlockNumberAndIndex(block.block_number, tx.tx_index))
                    else:
                        print("double frontrun!")
                        pass
                elif tx.mev_type == 'sandwich':
                    if sandwich_bundle != None: #如果当前有正在进行的sandwich组装工作
                        sandwich_bundle.AddTx(GetTxByBlockNumberAndIndex(block.block_number, tx.tx_index))
                    else:
                        print("sandwich without frontrun!")
                        pass
                elif tx.mev_type == 'backrun':
                    if sandwich_bundle != None: #如果当前有正在进行的sandwich组装工作，则这是第一个遇到的backrun
                        sandwich_bundle.AddTx(GetTxByBlockNumberAndIndex(block.block_number, tx.tx_index))
                        writer.writerow(sandwich_bundle.ToRow()) #打包好完整的sandwich bundle输出
                        sandwich_bundle = None #标记当前没有正在进行的sandwich打包工作
                    else:
                        #如果当前有sandwich在组装但是没有backrun则该sandwich是不会输出的
                        print("backrun without frontrun!")
                        pass


#调用API以字节流输出raw tx数据
def GetTxByBlockNumberAndIndex(block_num, index):
    raw_tx = None
    try:
        raw_tx = w3.eth.get_raw_transaction_by_block(block_num, index)
    except Exception as err:
        print("GetTxByBlockNumberAndIndex Err:", err)
        time.sleep(0.5)
        return GetTxByBlockNumberAndIndex(block_num, index)
    return raw_tx.hex()

#各种MEV type数量统计
def MevTypeCnt():
    global ALL_BLOCK_DATA
    dict = {}
    for block in ALL_BLOCK_DATA:
        for tx in block.txs:
            mev_type = tx.mev_type
            if mev_type not in dict.keys():
                dict[mev_type] = 0
            dict[mev_type] += 1
    for k, v in dict.items():
        print("MEV_type:", k, "Cnt:", v)

ALL_BLOCK_DATA = [] #按block分割数据，便于以后并行处理
OUTPUT_PATH = './dataset/assembled_bundle/'
INPUT_PATH = './dataset/mevbundle/19731189_20260000.csv'
if __name__ == '__main__':
    print("Start Time:", datetime.datetime.now())

    #读取数据
    ReadMEVBundleCSV(INPUT_PATH)
    #print([tx.tx_index for tx in ALL_BLOCK_DATA[0].txs])

    #组装bundle
    from_ = 0
    to_ = len(ALL_BLOCK_DATA)
    AssembleBundle(from_, to_, OUTPUT_PATH+str(from_)+'_'+str(to_)+'.csv')

    print("Finish Time:", datetime.datetime.now())

    #ReadMEVBundleCSV(INPUT_PATH)
    #GetTxByBlockNumberAndIndex(19733603, 103)


#Step 2，在通过zeroMEV API获取到bundle原始数据后，这里通过blocknum和txIndex并调用getTransactionByBlockNumberAndIndex重新还原bundle,并输出成csv格式
#格式为 mev_type, bundle_tx1, bundle_tx2, ..., bundle_txn
#这个文件只输出我们模拟一个bundle最简形式的数据，即bundle里的tx 16进制字符串列表与MEV标签

