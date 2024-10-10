#调用Zeromev API获取bundle信息

import csv
import math
import threading
import time
import requests

OUTPUT_ROOT = './dataset/mevbundle/'

class ResData:
    header = ['block_number', 'tx_index', 'mev_type', 'protocol', 'user_loss_usd', 'extractor_profit_usd',\
              'user_swap_volume_usd', 'user_swap_count', 'extractor_swap_volume_usd', 'extractor_swap_count',\
              'imbalance', 'address_from', 'address_to', 'arrival_time_us', 'arrival_time_eu', 'arrival_time_as']
    def __init__(self, map):
        self.block_number = map['block_number']
        self.tx_index = map['tx_index']
        self.mev_type = map['mev_type']
        self.protocol = map['protocol']
        self.user_loss_usd = map['user_loss_usd']
        self.extractor_profit_usd = map['extractor_profit_usd']
        self.user_swap_volume_usd = map['user_swap_volume_usd']
        self.user_swap_count = map['user_swap_count']
        self.extractor_swap_volume_usd = map['extractor_swap_volume_usd']
        self.extractor_swap_count = map['extractor_swap_count']
        self.imbalance = map['imbalance']
        self.address_from = map['address_from']
        self.address_to = map['address_to']
        self.arrival_time_us = map['arrival_time_us']
        self.arrival_time_eu = map['arrival_time_eu']
        self.arrival_time_as = map['arrival_time_as']
    def ToRow(self):
        return [self.block_number, self.tx_index, self.mev_type, self.protocol, self.user_loss_usd, self.extractor_profit_usd,\
                self.user_swap_volume_usd, self.user_swap_count, self.extractor_swap_volume_usd, self.extractor_swap_count,\
                self.imbalance, self.address_from, self.address_to, self.arrival_time_us, self.arrival_time_eu, self.arrival_time_as]


#请求zeromevAPI获取从指定block number后100个有mevbundle的block以及对应bundle信息范围：[block_num, block_num+99]
def GetBlockBundle(block_num):
    # 目标 URL
    url = 'https://data.zeromev.org/v1/mevBlock'
    # 查询参数
    params = {
        'block_number': block_num,
        'count': 100
    }
    response = None
    success = False #判断是否请求成功
    while not success:
        # 发起 GET 请求
        response = requests.get(url, params=params)
        # 检查请求是否成功,不成功就重复请求
        if response.status_code == 200:
            #print("响应内容:", response.json())
            success = True
        else:
            print(f"请求失败，状态码: {response.status_code}")
            time.sleep(0.5)
    return response.json()

#获取指定范围的数据，并写入文件 [from_, to_]
def GetBlockBundleInRange(from_, to_, output_path):
    with open(output_path, 'w') as output:
        writer = csv.writer(output)
        writer.writerow(ResData.header)
        current_pos = from_
        while from_ <= to_: #保证数据包含from_和to_
            res = GetBlockBundle(from_) #[from_, from_+99]
            for data in res:
                res_data = ResData(data)
                if res_data.block_number <= to_:#筛选大于to_的删掉
                    writer.writerow(res_data.ToRow())
            from_ += 100

#整合并行输出的文件为一个文件
def MergeFile(thread_output_list, merge_output_path):
    #打开合并到的文件
    with open(merge_output_path, 'w') as output:
        writer = csv.writer(output)
        writer.writerow(ResData.header)
        #打开每个线程写入的文件
        for thread_output_path in thread_output_list:
            with open(thread_output_path, 'r') as input:
                reader = csv.reader(input)
                is_header = True
                for row in reader: 
                    if is_header: #先读个表头
                        is_header = False
                        continue
                    writer.writerow(row) #写到合并文件

#[FROM, TO]
FROM = 19731189
TO = 20260000
if __name__ == '__main__':
    # res = GetBlockBundle(9900085)
    # print(res[0]['block_number'])
    # GetBlockBundleInRange(9900152, 9900552, OUTPUT_ROOT)
    
    thread_output_list = [] #用于合并文件读取的时候

    #并行获取数据
    thread_cnt = 10
    threads = []
    block_range_num = TO-FROM+1 #范围内区块总数
    divide_part_cnt =  math.ceil(block_range_num / thread_cnt) #每个线程分到的区块数量
    for i in range(thread_cnt):
        from_ = FROM+(i*divide_part_cnt)
        to_ = min(from_+divide_part_cnt-1, TO)
        #print(from_, to_)
        #新建线程
        output_path = OUTPUT_ROOT + str(from_)+'_'+str(to_)+'.csv'
        thread_output_list.append(output_path) #用于合并文件读取的时候
        thread = threading.Thread(target=GetBlockBundleInRange, args=(from_, to_, output_path))
        threads.append(thread)
        thread.start()
    #等待线程运行完毕
    for thread in threads:
        thread.join()

    #合并到一个文件
    merge_output_path = OUTPUT_ROOT+str(FROM)+'_'+str(TO)+'.csv'
    MergeFile(thread_output_list, merge_output_path)

#Step 1从zeroMEV的API并行爬取原始数据，数据是原始格式，导出为csv，字段和返回时一样
#python get_mev_bundle_parallel.py
#改动 FROM TO