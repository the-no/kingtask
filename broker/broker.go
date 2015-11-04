//process
package broker

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/flike/golog"

	"github.com/kingsoft-wps/kingtask/config"
	"github.com/kingsoft-wps/kingtask/core/errors"
	"github.com/kingsoft-wps/kingtask/core/timer"
	"github.com/kingsoft-wps/kingtask/task"
	"github.com/labstack/echo"
	"github.com/tylerb/graceful"
	redis "gopkg.in/redis.v3"
)

type Broker struct {
	cfg         *config.BrokerConfig
	addr        string
	redisAddr   string
	redisDB     int
	running     bool
	web         *echo.Echo
	redisClient *redis.Client
	timer       *timer.Timer
}

func NewBroker(cfg *config.BrokerConfig) (*Broker, error) {
	var err error

	broker := new(Broker)
	broker.cfg = cfg
	broker.addr = cfg.Addr
	if len(broker.addr) == 0 {
		return nil, errors.ErrInvalidArgument
	}

	vec := strings.SplitN(cfg.RedisAddr, "/", 2)
	if len(vec) == 2 {
		broker.redisAddr = vec[0]
		broker.redisDB, err = strconv.Atoi(vec[1])
		if err != nil {
			return nil, err
		}
	} else {
		broker.redisAddr = vec[0]
		broker.redisDB = config.DefaultRedisDB
	}

	broker.web = echo.New()

	broker.timer = timer.New(time.Millisecond * 10)
	go broker.timer.Start()

	broker.redisClient = redis.NewClient(
		&redis.Options{
			Addr:     broker.redisAddr,
			Password: "", // no password set
			DB:       int64(broker.redisDB),
		},
	)
	_, err = broker.redisClient.Ping().Result()
	if err != nil {
		golog.Error("broker", "NewBroker", "ping redis fail", 0, "err", err.Error())
		return nil, err
	}

	return broker, nil
}

func (b *Broker) Run() {
	b.running = true
	b.RegisterMiddleware()
	b.RegisterURL()
	go b.HandleFailTask()
	graceful.ListenAndServe(b.web.Server(b.addr), 5*time.Second)
}

func (b *Broker) Close() {
	b.running = false
	b.redisClient.Close()
	b.timer.Stop()
}

func (b *Broker) HandleTaskResult(uuid string) (*task.Reply, error) {
	if len(uuid) == 0 {
		return nil, errors.ErrInvalidArgument
	}
	key := fmt.Sprintf("r_%s", uuid)
	result, err := b.redisClient.HMGet(key,
		"is_success",
		"result",
	).Result()
	if err != nil {
		golog.Error("Broker", "HandleTaskResult", err.Error(), 0, "req_key", key)
		return nil, err
	}

	//key不存在
	if result[0] == nil {
		return nil, errors.ErrResultNotExist
	}
	isSuccess, err := strconv.Atoi(result[0].(string))
	if err != nil {
		return nil, err
	}
	ret := result[1].(string)
	return &task.Reply{
		IsResultExist: 1,
		IsSuccess:     isSuccess,
		Result:        ret,
	}, nil
}

func (b *Broker) HandleRequest(request *task.TaskRequest) error {
	var err error
	now := time.Now().Unix()
	if request.StartTime == 0 {
		request.StartTime = now
	}

	if request.StartTime <= now {
		err = b.AddRequestToRedis(request)
		if err != nil {
			return err
		}
	} else {
		afterTime := time.Second * time.Duration(request.StartTime-now)
		b.timer.NewTimer(afterTime, b.AddRequestToRedis, request)
	}

	return nil
}

//处理失败的任务
func (b *Broker) HandleFailTask() error {
	var uuid string
	var err error
	for b.running {
		uuid, err = b.redisClient.SPop(config.FailResultUuidSet).Result()
		//没有结果，直接返回
		if err == redis.Nil {
			time.Sleep(time.Second)
			continue
		}
		if err != nil {
			golog.Error("Broker", "HandleFailTask", "spop error", 0, "error", err.Error())
			continue
		}

		key := fmt.Sprintf("r_%s", uuid)
		timeInterval, err := b.redisClient.HGet(key, "time_interval").Result()
		if err != nil {
			golog.Error("Broker", "HandleFailTask", err.Error(), 0, "key", key)
			continue
		}
		//没有超时重试机制
		if len(timeInterval) == 0 {
			b.SetFailTaskCount(fmt.Sprintf("t_%s", uuid))
			continue
		}
		//获取结果中所有值,改为逐个获取
		results, err := b.redisClient.HMGet(key,
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
			golog.Error("Broker", "HandleFailTask", err.Error(), 0, "key", key)
			continue
		}
		//key已经过期
		if results[0] == nil {
			golog.Error("Broker", "HandleFailTask", "result expired", 0, "key", key)
			continue
		}
		//删除结果
		_, err = b.redisClient.Del(key).Result()
		if err != nil {
			golog.Error("Broker", "HandleFailTask", "delete result failed", 0, "key", key)
		}
		err = b.resetTaskRequest(results)
		if err != nil {
			golog.Error("Broker", "HandleFailTask", err.Error(), 0, "key", key)
			b.SetFailTaskCount(fmt.Sprintf("t_%s", uuid))
		}
	}

	return nil
}

func (b *Broker) SetFailTaskCount(reqKey string) error {
	failTaskKey := fmt.Sprintf(config.FailTaskKey,
		time.Now().Format(config.TimeFormat))
	count, err := b.redisClient.Incr(failTaskKey).Result()
	if err != nil {
		golog.Error("Worker", "SetFailTaskCount", "Incr", 0, "err", err.Error(),
			"req_key", reqKey)
		return err
	}
	//第一次设置该key
	if count == 1 {
		//保存一个月
		expireTime := time.Second * time.Duration(60*60*24*30)
		_, err = b.redisClient.Expire(failTaskKey, expireTime).Result()
		if err != nil {
			golog.Error("Worker", "SetFailTaskCount", "Expire", 0, "err", err.Error(),
				"req_key", reqKey)
			return err
		}
	}
	return nil
}

func (b *Broker) resetTaskRequest(args []interface{}) error {
	var err error
	if len(args) == 0 || len(args) != config.TaskRequestItemCount {
		return errors.ErrInvalidArgument
	}
	request := new(task.TaskRequest)
	request.Uuid = args[0].(string)
	request.BinName = args[1].(string)
	request.Args = args[2].(string)
	request.StartTime, err = strconv.ParseInt(args[3].(string), 10, 64)
	if err != nil {
		return err
	}
	request.TimeInterval = args[4].(string)
	request.Index, err = strconv.Atoi(args[5].(string))
	if err != nil {
		return err
	}
	request.MaxRunTime, err = strconv.ParseInt(args[6].(string), 10, 64)
	if err != nil {
		return err
	}
	request.TaskType, err = strconv.Atoi(args[7].(string))
	if err != nil {
		return err
	}
	vec := strings.Split(request.TimeInterval, " ")
	request.Index++
	if request.Index < len(vec) {
		timeLater, err := strconv.Atoi(vec[request.Index])
		if err != nil {
			return err
		}
		afterTime := time.Second * time.Duration(timeLater)
		b.timer.NewTimer(afterTime, b.AddRequestToRedis, request)
	} else {
		golog.Error("Broker", "HandleFailTask", "retry max time", 0,
			"key", fmt.Sprintf("t_%s", request.Uuid))
		return errors.ErrTryMaxTimes
	}
	return nil
}

func (b *Broker) AddRequestToRedis(tr interface{}) error {
	r, ok := tr.(*task.TaskRequest)
	if !ok {
		return errors.ErrInvalidArgument
	}
	key := fmt.Sprintf("t_%s", r.Uuid)
	setCmd := b.redisClient.HMSet(key,
		"uuid", r.Uuid,
		"bin_name", r.BinName,
		"args", r.Args,
		"start_time", strconv.FormatInt(r.StartTime, 10),
		"time_interval", r.TimeInterval,
		"index", strconv.Itoa(r.Index),
		"max_run_time", strconv.FormatInt(r.MaxRunTime, 10),
		"task_type", strconv.Itoa(r.TaskType),
	)
	err := setCmd.Err()
	if err != nil {
		golog.Error("Broker", "AddRequestToRedis", "HMSET error", 0,
			"set", config.RequestUuidSet,
			"uuid", r.Uuid,
			"err", err.Error(),
		)
		return err
	}
	saddCmd := b.redisClient.SAdd(config.RequestUuidSet, r.Uuid)
	err = saddCmd.Err()
	if err != nil {
		golog.Error("Broker", "AddRequestToRedis", "SADD error", 0,
			"set", config.RequestUuidSet,
			"uuid", r.Uuid,
			"err", err.Error(),
		)
		return err
	}

	return nil
}

func (b *Broker) GetUndoTaskCount() (int64, error) {
	count, err := b.redisClient.SCard(config.RequestUuidSet).Result()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (b *Broker) GetFailTaskCount(date string) (int64, error) {
	if len(date) == 0 {
		return 0, errors.ErrInvalidArgument
	}
	failTaskKey := fmt.Sprintf(config.FailTaskKey, date)
	str, err := b.redisClient.Get(failTaskKey).Result()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	count, err := strconv.ParseInt(str, 10, 64)
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (b *Broker) GetSuccessTaskCount(date string) (int64, error) {
	if len(date) == 0 {
		return 0, errors.ErrInvalidArgument
	}
	successTaskKey := fmt.Sprintf(config.SuccessTaskKey, date)
	str, err := b.redisClient.Get(successTaskKey).Result()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	count, err := strconv.ParseInt(str, 10, 64)
	if err != nil {
		return 0, err
	}
	return count, nil
}
