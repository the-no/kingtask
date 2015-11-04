[![Build Status](https://travis-ci.org/kingsoft-wps/kingtask.svg?branch=master)](https://travis-ci.org/kingsoft-wps/kingtask)
# Kingtask
Kingtask is an async task system developed in golang.

# Features

- Async task with timer
- Retry when task failed(When and how many times to retry are in config file)
- Result of task can be queried
- Task can be executable files or Web APIs
- No need to regist to `Kingtask` before execute async task
- `broker` and `worker` are decoupled by redis
- `Kingtask` can become `HA(High Available)` through redis cluster(master-slave)

# Architecture
## Kingtask architecture diagram

![architecture diagram](./kingtask_arch.png)

## Implementation

1. The `broker` will wrap the `async task(send from client, each async task got an uuid)` to a struct and store it into redis, meanwhile, if the `async task` is a timer task, the `broker` will wrap and store it when the timer stop.
2. The `worker` will fetch `async task` from redis, then execute it and store the result to redis.
3. If the `async task` was failed and it was configured to retry, the `broker` will restore the `async task` to redis so that the `worker` will execute it again.

# Quick start

## Setup broker

```
#broker
addr : 0.0.0.0:9595
#redis
redis : 127.0.0.1:6379
#log path(option)
#log_path: /Users/flike/src
#log level
log_level: debug
```

## Setup worker

```
#redis
redis : 127.0.0.1:6379
#Path of executable task file
bin_path : /Users/flike/src
#log path(option)
#log_path : /Users/flike/src
#log level
log_level: debug

#Timer task (s)
period : 1
#Expired time of task result
result_keep_time : 1000
#Task timeout
task_run_time: 30
```

## Run broker and worker

```
#Copy executable task file to bin_path
cp example /Users/flike/src
#Turn to the directory of Kingtask
cd kingtask
#Run broker
./bin/broker -config=etc/broker.yaml
#Run worker
./bin/worker -config=etc/worker.yaml
```

### Source code of example

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

## Using Kingtask

You can use `Kingtask` via HTTP APIs as follows:


### For calling executable task file

**Request api**

```
POST /api/v1/task/script
```
**Request params**

name|type|required|description
:----|:----|:--------|:-----------
bin_name| string| true| The name of executable task file
args| string| false| Arguments of executable file, split by ` `
start_time| int| false| The time to execute the `async task`, execute immediately if got null
time_interval| string| false| The retry time format
max_run_time| int| true| The timeout of the `async task`

**Response**

`Kingtask` will response 403 and error message when calling it failed.

`Kingtask` will response 200 and the uuid of the `async task`, the uuid can be used to query the result of the `async task`

**Examples**

Do the http request via a tool named httpie:
```
http POST 127.0.0.1:9595/api/v1/task bin_name="mytask" args="12 hello" start_time=1445562622 time_interval="60 600 3600"
```

Then kingtask will do:

- Find the executable file `mytask` in /Users/flike/src with args of `12` and `"hello"`
- Execute the executable file at `1445562622`(timestamp)
- If the async task failed, kingtask will retry it and the time interval is `60s,600s,3600s`


### For calling rpc api

**Request api**

```
POST /api/v1/task/rpc
```

**Request params**

name|type|required|description
:----|:----|:--------|:-----------
method| string| true| Request method(GET,PUT,POST,DELETE)
url| string| true| The request url for Rpc task
args| string| true| Json string argumens for Rpc task
start_time| int| false| The time to execute the `async task`, execute immediately if got null
time_interval| string| false| The retry time format
max_run_time| int| true| The timeout of the `async task`

**Reponse**

`Kingtask` will response 403 and error message when calling it failed.

`Kingtask` will response 200 and the uuid of the `async task`, the uuid can be used to query the result of the `async task`

**Examples**

Do the http request via a tool named httpie:
```
http POST 127.0.0.1:9595/api/v1/task/rpc method="POST" url="http://127.0.0.1:1323/sum" args='{"a":132,"b":75}'
```
Then kingtask will do ：
```
POST http://127.0.0.1:1323/sum with args (args)
```

### For query the result of async task

**Request api**

```
GET /api/v1/task/result/:uuid
```

**Request params**

name|type|required|description
:----|:----|:--------|:-----------
uuid| string| true| Uuid of the async task

**Reponse**

`Kingtask` will response 403 and error message when calling it failed.

`Kingtask` will response 200 and result of the `async task`

**Example**

```
http GET 127.0.0.1:9595/api/v1/task/result/db3e0b22-a249-4ed2-9532-fc6318ccd321
```

### For looking up the report of async tasks

To look up the count of all left async tasks

```
http GET 127.0.0.1:9595/api/v1/task/count/undo
```

**Reponse**

`Kingtask` will response 403 and error message when calling it failed.

`Kingtask` will response 200 and the count of all left async tasks


To look up the count of failed async tasks in specified date

```
http GET 127.0.0.1:9595/api/v1/task/result/failure/:date
date format :2006-01-02
```

**Reponse**

`Kingtask` will response 403 and error message when calling it failed.

`Kingtask` will response 200 and the count of failed async tasks


To look up the count of executed async tasks in specified date

```
http GET 127.0.0.1:9595/api/v1/task/result/success/:date
date format :2006-01-02
```
**Reponse**

`Kingtask` will response 403 and error message when calling it failed.

`Kingtask` will response 200 and the count of executed async tasks


### Practice

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
