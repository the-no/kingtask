package worker

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/flike/golog"

	"github.com/the-no/kingtask/config"
	"github.com/the-no/kingtask/core/errors"
	"github.com/the-no/kingtask/task"
	redis "gopkg.in/redis.v3"
)

type Worker struct {
	cfg         *config.WorkerConfig
	redisAddr   string
	redisDB     int
	running     bool
	redisClient *redis.Client
}

func NewWorker(cfg *config.WorkerConfig) (*Worker, error) {
	var err error
	w := new(Worker)
	w.cfg = cfg

	vec := strings.SplitN(cfg.RedisAddr, "/", 2)
	if len(vec) == 2 {
		w.redisAddr = vec[0]
		w.redisDB, err = strconv.Atoi(vec[1])
		if err != nil {
			return nil, err
		}
	} else {
		w.redisAddr = vec[0]
		w.redisDB = config.DefaultRedisDB
	}

	w.redisClient = redis.NewClient(
		&redis.Options{
			Addr:     w.redisAddr,
			Password: "", // no password set
			DB:       int64(w.redisDB),
		},
	)
	_, err = w.redisClient.Ping().Result()
	if err != nil {
		golog.Error("worker", "NewWorker", "ping redis fail", 0, "err", err.Error())
		return nil, err
	}

	return w, nil
}

func (w *Worker) Run() error {
	var taskResult *task.TaskResult
	w.running = true
	for w.running {
		uuid, err := w.redisClient.SPop(config.RequestUuidSet).Result()
		//没有请求
		if err == redis.Nil {
			time.Sleep(time.Second)
			continue
		}
		if err != nil {
			golog.Error("Worker", "run", "spop error", 0, "error", err.Error())
			continue
		}
		reqKey := fmt.Sprintf("t_%s", uuid)

		//获取请求中所有值
		request, err := w.redisClient.HMGet(reqKey,
			"uuid",
			"bin_name",
			"args",
			"start_time",
			"time_interval",
			"index",
			"max_run_time",
			"task_type",
		).Result()
		if err != nil {
			golog.Error("Worker", "run", err.Error(), 0, "req_key", reqKey)
			continue
		}
		//key不存在
		if request[0] == nil {
			golog.Error("Worker", "run", "Key is not exist", 0, "req_key", reqKey)
			continue
		}
		_, err = w.redisClient.Del(reqKey).Result()
		if err != nil {
			golog.Error("Worker", "run", "delete result failed", 0, "req_key", reqKey)
		}

		taskResult, err = w.DoTaskRequest(request)
		if err != nil {
			golog.Error("Worker", "run", "DoTaskRequest", 0, "err", err.Error(),
				"req_key", reqKey, "bin_name", request[1], "task_type", request[7])
		} else {
			w.SetSuccessTaskCount(reqKey)
		}

		if taskResult != nil {
			err = w.SetTaskResult(taskResult)
			if err != nil {
				golog.Error("Worker", "run", "DoScrpitTaskRequest", 0,
					"err", err.Error(), "req_key", reqKey)
			}
			golog.Info("worker", "run", "do task success", 0, "req_key", reqKey,
				"result", taskResult.Result)
		}

		if w.cfg.Peroid != 0 {
			time.Sleep(time.Second * time.Duration(w.cfg.Peroid))
		}
	}
	return nil
}

func (w *Worker) Close() {
	w.running = false
	w.redisClient.Close()
}

func (w *Worker) DoRpcTaskRequest(req *task.TaskRequest) (string, error) {
	var method string
	switch req.TaskType {
	case task.RpcTaskGET:
		method = "GET"
	case task.RpcTaskPOST:
		method = "POST"
	case task.RpcTaskPUT:
		method = "PUT"
	case task.RpcTaskDELETE:
		method = "DELETE"
	default:
		method = "GET"
	}
	url := req.BinName
	args := req.Args
	request, err := w.newHttpRequest(method, url, args)
	if err != nil {
		return "", err
	}
	result, err := w.callRpc(request, time.Second*time.Duration(req.MaxRunTime))
	return result, err
}

func (w *Worker) newHttpRequest(method string, url string, args string) (*http.Request, error) {
	var body io.Reader
	if len(args) != 0 {
		body = bytes.NewBuffer([]byte(args))
	}
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

func (w *Worker) callRpc(req *http.Request, maxRunTime time.Duration) (string, error) {
	var timeout time.Duration
	if w.cfg.TaskRunTime != 0 {
		timeout = time.Duration(w.cfg.TaskRunTime) * time.Second
	} else {
		timeout = maxRunTime
	}

	//new a http client with timeout setting
	client := &http.Client{
		Timeout: timeout,
	}

	r, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer r.Body.Close()
	buf, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return "", err
	}
	if r.StatusCode != http.StatusOK {
		return "", errors.NewError(string(buf))
	}

	return string(buf), nil
}

func (w *Worker) DoTaskRequest(args []interface{}) (*task.TaskResult, error) {
	var err error
	var output string

	req := new(task.TaskRequest)
	ret := new(task.TaskResult)

	req.Uuid = args[0].(string)
	req.BinName = args[1].(string)
	req.Args = args[2].(string)
	req.StartTime, err = strconv.ParseInt(args[3].(string), 10, 64)
	if err != nil {
		return nil, err
	}
	req.TimeInterval = args[4].(string)
	req.Index, err = strconv.Atoi(args[5].(string))
	if err != nil {
		return nil, err
	}
	req.MaxRunTime, err = strconv.ParseInt(args[6].(string), 10, 64)
	if err != nil {
		return nil, err
	}
	req.TaskType, err = strconv.Atoi(args[7].(string))
	if err != nil {
		return nil, err
	}
	switch req.TaskType {
	case task.ScriptTask:
		output, err = w.DoScriptTaskRequest(req)
	case task.RpcTaskGET, task.RpcTaskPOST, task.RpcTaskPUT, task.RpcTaskDELETE:
		output, err = w.DoRpcTaskRequest(req)
	default:
		err = errors.ErrInvalidArgument
		golog.Error("Worker", "DoTaskRequest", "task type error", 0, "task_type", req.TaskType)
	}
	ret.TaskRequest = *req
	//执行任务失败，
	if err != nil {
		ret.IsSuccess = int64(0)
		ret.Result = err.Error()
		return ret, nil
	}
	ret.IsSuccess = int64(1)
	ret.Result = output

	return ret, nil
}

func (w *Worker) DoScriptTaskRequest(req *task.TaskRequest) (string, error) {
	var output string
	var err error
	var maxRunTime int64

	binPath := path.Clean(w.cfg.BinPath + "/" + req.BinName)
	_, err = os.Stat(binPath)
	if err != nil && os.IsNotExist(err) {
		golog.Error("worker", "DoScrpitTaskRequest", "File not exist", 0,
			"key", fmt.Sprintf("t_%s", req.Uuid),
			"bin_path", binPath,
		)
		return "", errors.ErrFileNotExist
	}
	if req.MaxRunTime == 0 {
		maxRunTime = w.cfg.TaskRunTime
	} else {
		maxRunTime = req.MaxRunTime
	}
	if len(req.Args) == 0 {
		output, err = w.ExecBin(binPath, nil, maxRunTime)
	} else {
		argsVec := strings.Split(req.Args, " ")
		output, err = w.ExecBin(binPath, argsVec, maxRunTime)
	}
	return output, err
}

func (w *Worker) ExecBin(binPath string, args []string, maxRunTime int64) (string, error) {
	var cmd *exec.Cmd
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var err error

	if len(args) == 0 {
		cmd = exec.Command(binPath)
	} else {
		cmd = exec.Command(binPath, args...)
	}

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Start() // attention!

	err, _ = w.CmdRunWithTimeout(cmd,
		time.Duration(maxRunTime)*time.Second,
	)
	if err != nil {
		return "", err
	}
	if len(stderr.String()) != 0 {
		errMsg := strings.TrimRight(stderr.String(), "\n")
		return "", errors.NewError(errMsg)
	}

	return strings.TrimRight(stdout.String(), "\n"), nil
}

func (w *Worker) CmdRunWithTimeout(cmd *exec.Cmd, timeout time.Duration) (error, bool) {
	done := make(chan error)
	go func() {
		done <- cmd.Wait()
	}()

	var err error
	select {
	case <-time.After(timeout):
		// timeout
		if err = cmd.Process.Kill(); err != nil {
			golog.Error("worker", "CmdRunTimeout", "kill error", 0,
				"path", cmd.Path,
				"error", err.Error(),
			)
		}
		golog.Info("worker", "CmdRunWithTimeout", "kill process", 0,
			"path", cmd.Path,
			"error", errors.ErrExecTimeout.Error(),
		)
		go func() {
			<-done // allow goroutine to exit
		}()
		return errors.ErrExecTimeout, true
	case err = <-done:
		return err, false
	}
}

func (w *Worker) SetTaskResult(result *task.TaskResult) error {
	key := fmt.Sprintf("r_%s", result.Uuid)
	setCmd := w.redisClient.HMSet(key,
		"uuid", result.Uuid,
		"bin_name", result.BinName,
		"args", result.Args,
		"start_time", strconv.FormatInt(result.StartTime, 10),
		"time_interval", result.TimeInterval,
		"index", strconv.Itoa(result.Index),
		"max_run_time", strconv.FormatInt(result.MaxRunTime, 10),
		"task_type", strconv.Itoa(result.TaskType),
		"is_success", strconv.Itoa(int(result.IsSuccess)),
		"result", result.Result,
	)
	err := setCmd.Err()
	if err != nil {
		return err
	}
	if result.IsSuccess == int64(0) {
		saddCmd := w.redisClient.SAdd(config.FailResultUuidSet, result.Uuid)
		err = saddCmd.Err()
		if err != nil {
			return err
		}
	}
	_, err = w.redisClient.Expire(key, time.Second*time.Duration(w.cfg.ResultKeepTime)).Result()
	if err != nil {
		return err
	}
	return nil
}

func (w *Worker) SetSuccessTaskCount(reqKey string) error {
	successTaskKey := fmt.Sprintf(config.SuccessTaskKey,
		time.Now().Format(config.TimeFormat))
	count, err := w.redisClient.Incr(successTaskKey).Result()
	if err != nil {
		golog.Error("Worker", "SetSuccessTaskCount", "Incr", 0, "err", err.Error(),
			"req_key", reqKey)
		return err
	}
	//第一次设置该key
	if count == 1 {
		//保存一个月
		expireTime := time.Second * time.Duration(60*60*24*30)
		_, err = w.redisClient.Expire(successTaskKey, expireTime).Result()
		if err != nil {
			golog.Error("Worker", "SetSuccessTaskCount", "Expire", 0, "err", err.Error(),
				"req_key", reqKey)
			return err
		}
	}
	return nil
}
