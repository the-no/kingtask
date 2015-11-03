
# 1. kingtask简介
kingtask([English document](./doc/README-EN.md))是一个由Go开发的异步任务系统。
主要特性包含以下几个部分：

1. 支持定时的异步任务。
2. 支持失败重试机制，重试时刻和次数可自定义。
3. 任务执行结果可查询。
4. 一个异步任务由一个可执行文件或者一个Web API组成，开发语言不限。
5. 任务是无状态的，执行异步任务之前，不需要向kingtask注册任务。
6. broker和worker通过redis解耦。
7. 通过配置redis为master-slave架构，可实现kingtask的高可用，因为worker是无状态的，redis的master宕机后，可以修改worker配置将其连接到slave上。

# 2. kingtask架构
kingtask架构图如下所示：
![架构图](./doc/kingtask_arch.png)

kingtask的实现步骤如下所述：

1. broker收到client发送过来的异步任务（一个异步任务由一个唯一的uuid标示）之后，判断异步任务是否定时，如果未定时，则直接将异步任务封装成一个结构体，存入redis。如果定时，则通过定时器触发，将异步任务封装成一个结构体，存入redis。
2. worker从redis中获取异步任务，或者到任务之后，执行该任务，并将任务结果存入redis。
3. 对于失败的任务，如果该任务有重试机制，broker会重新发送该任务到redis，然后worker会重新执行。

# 3. kingtask使用

## 3.1 配置broker

```
#broker地址
addr : 0.0.0.0:9595
#redis地址
redis : 127.0.0.1:6379
#log输出到文件，可不配置
#log_path: /Users/flike/src 
#日志级别
log_level: debug
```

# 3.2 配置worker

```
#redis地址
redis : 127.0.0.1:6379
#异步任务可执行文件目录
bin_path : /Users/flike/src
#日志输出目录，可不配置
#log_path : /Users/flike/src
#日志级别
log_level: debug

#每个任务执行时间间隔，单位为秒
period : 1
#结果保存时间，单位为秒
result_keep_time : 1000
#任务执行最长时间，单位秒
task_run_time: 30
```

## 3.3 运行broker和worker

```
#将异步任务的可执行文件放到bin_path目录
cp example /Users/flike/src
#转到kingtask目录
cd kingtask
#启动broker
./bin/broker -config=etc/broker.yaml
#启动worker
./bin/worker -config=etc/worker.yaml
```

### 3.3.1 example异步任务源码

异步任务的结果需要输出到标准输出(os.Stdout),出错信息需要输出到标准出错输出(os.Stderr)。

```
//example.go
package main

import (
	"fmt"
	"os"
	"strconv"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintf(os.Stderr, "args count must be two")
		return
	}
	left, err := strconv.ParseInt(os.Args[1], 10, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "err:%s", err.Error())
		return
	}
	right, err := strconv.ParseInt(os.Args[2], 10, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "err:%s", err.Error())
		return
	}
	sum := left + right
	fmt.Fprintf(os.Stdout, "%d", sum)
}

```

### 3.3.2 调用异步任务

kingtask异步任务系统提供Web API接口供客户端调用异步任务，主要有以下API接口：


(1). 执行脚本异步任务API接口

```
POST /api/v1/task/script

#请求参数
bin_name //字符串类型，表示异步对应的可执行文件名，必须提供
args //字符串类型，执行参数，多个参数用空格分隔，可为空
start_time //整型，异步任务开始执行时刻，为空表示立刻执行，可为空
time_interval //字符串类型，表示失败后重试的时间间隔序列，可为空
max_run_time //整型，异步任务最长运行时间（单位为秒),超过将会被系统kill，为空则使用系统统一的超时时长

#返回值
如果出错返回403和出错信息
如果调用成功返回200和标示该task的uuid，该uuid可用于查询任务结果
```

例如执行以下API调用：

```
通过httpie工具执行以下命令
http POST 127.0.0.1:9595/api/v1/task bin_name="mytask" args="12 hello" start_time=1445562622 time_interval="60 600 3600"

则kingtask会执行以下操作：

- 执行/Users/flike/src（由woker配置的目录）目录下的mytask可执行文件，参数是12和hello。
- 异步任务开始执行的时刻是1445562622（时间戳）。
- 如果该异步任务执行失败，kingtask会重试该异步任务，时间间隔是:60s,600s,3600s。
```

(2). 执行RPC异步任务API接口

```
POST /api/v1/task/rpc

#请求参数
method //请求类型：GET,PUT,POST,DELETE
url //异步任务对应的URL,需要加单引号
args //json Marshal后的字符串,需要加单引号
start_time //整型，异步任务开始执行时刻，为空表示立刻执行，可为空
time_interval //字符串类型，表示失败后重试的时间间隔序列，可为空
max_run_time //整型，异步任务最长运行时间（单位为秒),超过将会被系统kill，为空则使用系统统一的超时时长

#返回值
如果出错返回403和出错信息
如果调用成功返回200和标示该task的uuid，该uuid可用于查询任务结果
```

例如执行以下API调用：

```
通过httpie工具执行以下命令
http POST 127.0.0.1:9595/api/v1/task/rpc method="POST" url="http://127.0.0.1:1323/sum" args='{"a":132,"b":75}'

则kingtask会执行：POST 参数(args)到URL(http://127.0.0.1:1323/sum)

```

(3). 查看异步任务结果API接口

kingtask中的worker在执行完异步任务之后，都会将异步任务的结果存入redis，结果过期时间可配置。
客户端可通过以下API接口查看异步任务结果：

```
GET /api/v1/task/result/:uuid

参数是调用执行异步任务返回的uuid。
返回值
如果出错返回403和出错信息
如果调用成功返回200和和任务结果
例如
http GET 127.0.0.1:9595/api/v1/task/result/db3e0b22-a249-4ed2-9532-fc6318ccd321
```

(4). 统计休息查看

查看积压任务个数

```
http GET 127.0.0.1:9595/api/v1/task/count/undo
返回值
如果出错返回403和出错信息
如果调用成功返回200和和积压任务个数

```

查看某一天执行失败的异步任务个数

```
http GET 127.0.0.1:9595/api/v1/task/result/failure/:date
date参数格式为:2006-01-02

返回值
如果出错返回403和出错信息
如果调用成功返回200和和失败任务个数
```

查看某一天执行成功的异步任务个数

```
http GET 127.0.0.1:9595/api/v1/task/result/success/:date
date参数格式为:2006-01-02
返回值
如果出错返回403和出错信息
如果调用成功返回200和和成功任务个数
```

### 3.3.3 调用异步任务例子

```
➜  ~  http POST 127.0.0.1:9595/api/v1/task bin_name="example" args="12 34"
HTTP/1.1 200 OK
Content-Length: 38
Content-Type: application/json; charset=utf-8
Date: Fri, 23 Oct 2015 01:10:22 GMT

"db3e0b22-a249-4ed2-9532-fc6318ccd321"

➜  ~  http GET 127.0.0.1:9595/api/v1/task/result/db3e0b22-a249-4ed2-9532-fc6318ccd321
HTTP/1.1 200 OK
Content-Length: 51
Content-Type: application/json; charset=utf-8
Date: Fri, 23 Oct 2015 01:11:44 GMT

{
    "is_result_exist": 1,
    "is_success": 1,
    "message": "46"
}

http POST 127.0.0.1:9595/api/v1/task/rpc method="POST" url="http://127.0.0.1:1323/sum" args='{"a":132,"b":75}'
HTTP/1.1 200 OK
Content-Length: 38
Content-Type: application/json; charset=utf-8
Date: Fri, 30 Oct 2015 02:34:56 GMT

"e6bfb95a-a619-48b7-acb6-396736486c7c"

➜  ~  http GET 127.0.0.1:9595/api/v1/task/result/e6bfb95a-a619-48b7-acb6-396736486c7c
HTTP/1.1 200 OK
Content-Length: 52
Content-Type: application/json; charset=utf-8
Date: Fri, 30 Oct 2015 02:36:05 GMT

{
    "is_result_exist": 1,
    "is_success": 1,
    "message": "207"
}

➜  ~  http GET 127.0.0.1:9595/api/v1/task/result/success/2015-10-30
HTTP/1.1 200 OK
Content-Length: 1
Content-Type: application/json; charset=utf-8
Date: Fri, 30 Oct 2015 03:08:10 GMT

3

➜  ~  http GET 127.0.0.1:9595/api/v1/task/result/failure/2015-10-30
HTTP/1.1 200 OK
Content-Length: 1
Content-Type: application/json; charset=utf-8
Date: Fri, 30 Oct 2015 03:08:34 GMT

2
```
