package broker

import (
	"net/http"

	"github.com/flike/golog"
	"github.com/kingsoft-wps/kingtask/core/errors"
	"github.com/kingsoft-wps/kingtask/task"
	"github.com/labstack/echo"
	mw "github.com/labstack/echo/middleware"
	"github.com/pborman/uuid"
)

func (b *Broker) RegisterMiddleware() {
	b.web.Use(mw.Logger())
	b.web.Use(mw.Recover())
}

func (b *Broker) RegisterURL() {
	b.web.Post("/api/v1/task/script", b.CreateScriptTaskRequest)
	b.web.Post("/api/v1/task/rpc", b.CreateRpcTaskRequest)
	b.web.Get("/api/v1/task/result/:uuid", b.GetTaskResult)
	b.web.Get("/api/v1/task/count/undo", b.UndoTaskCount)
	b.web.Get("/api/v1/task/result/failure/:date", b.FailTaskCount)
	b.web.Get("/api/v1/task/result/success/:date", b.SuccessTaskCount)
}

func (b *Broker) CreateScriptTaskRequest(c *echo.Context) error {
	args := struct {
		BinName      string `json:"bin_name"`
		Args         string `json:"args"` //空格分隔各个参数
		StartTime    int64  `json:"start_time,string"`
		TimeInterval string `json:"time_interval"` //空格分隔各个参数
		MaxRunTime   int64  `json:"max_run_time,string"`
	}{}

	err := c.Bind(&args)
	if err != nil {
		return c.JSON(http.StatusForbidden, err.Error())
	}
	taskRequest := new(task.TaskRequest)
	taskRequest.Uuid = uuid.New()
	if len(args.BinName) == 0 {
		return c.JSON(http.StatusForbidden, errors.ErrInvalidArgument.Error())
	}

	taskRequest.BinName = args.BinName
	taskRequest.Args = args.Args
	taskRequest.StartTime = args.StartTime
	taskRequest.TimeInterval = args.TimeInterval
	taskRequest.Index = 0
	taskRequest.MaxRunTime = args.MaxRunTime
	taskRequest.TaskType = task.ScriptTask

	err = b.HandleRequest(taskRequest)
	if err != nil {
		return c.JSON(http.StatusForbidden, err.Error())
	}
	golog.Info("Broker", "CreateScriptTaskRequest", "ok", 0,
		"uuid", taskRequest.Uuid,
		"bin_name", taskRequest.BinName,
		"args", taskRequest.Args,
		"start_time", taskRequest.StartTime,
		"time_interval", taskRequest.TimeInterval,
		"index", taskRequest.Index,
		"max_run_time", taskRequest.MaxRunTime,
		"task_type", taskRequest.TaskType,
	)
	return c.JSON(http.StatusOK, taskRequest.Uuid)
}

func (b *Broker) CreateRpcTaskRequest(c *echo.Context) error {
	args := struct {
		Method       string `json:"method"`
		URL          string `json:"url"`
		Args         string `json:"args"` //json Marshal后的字符串
		StartTime    int64  `json:"start_time,string"`
		TimeInterval string `json:"time_interval"` //空格分隔各个参数
		MaxRunTime   int64  `json:"max_run_time,string"`
	}{}

	err := c.Bind(&args)
	if err != nil {
		return c.JSON(http.StatusForbidden, err.Error())
	}

	taskRequest := new(task.TaskRequest)
	taskRequest.Uuid = uuid.New()
	if len(args.URL) == 0 {
		return c.JSON(http.StatusForbidden, errors.ErrInvalidArgument.Error())
	}

	taskRequest.BinName = args.URL
	taskRequest.Args = args.Args
	taskRequest.StartTime = args.StartTime
	taskRequest.TimeInterval = args.TimeInterval
	taskRequest.Index = 0
	taskRequest.MaxRunTime = args.MaxRunTime
	switch args.Method {
	case "GET":
		taskRequest.TaskType = task.RpcTaskGET
	case "POST":
		taskRequest.TaskType = task.RpcTaskPOST
	case "PUT":
		taskRequest.TaskType = task.RpcTaskPUT
	case "DELETE":
		taskRequest.TaskType = task.RpcTaskDELETE
	default:
		return c.JSON(http.StatusForbidden, errors.ErrInvalidArgument.Error())
	}

	err = b.HandleRequest(taskRequest)
	if err != nil {
		return c.JSON(http.StatusForbidden, err.Error())
	}
	golog.Info("Broker", "CreateRpcTaskRequest", "ok", 0,
		"uuid", taskRequest.Uuid,
		"bin_name", taskRequest.BinName,
		"args", taskRequest.Args,
		"start_time", taskRequest.StartTime,
		"time_interval", taskRequest.TimeInterval,
		"index", taskRequest.Index,
		"max_run_time", taskRequest.MaxRunTime,
		"task_type", taskRequest.TaskType,
	)
	return c.JSON(http.StatusOK, taskRequest.Uuid)
}

func (b *Broker) GetTaskResult(c *echo.Context) error {
	uuid := c.Param("uuid")
	if len(uuid) == 0 {
		return c.JSON(http.StatusForbidden, errors.ErrInvalidArgument.Error())
	}

	reply, err := b.HandleTaskResult(uuid)
	if err != nil {
		return c.JSON(http.StatusForbidden, err.Error())
	}
	return c.JSON(http.StatusOK, reply)
}

func (b *Broker) UndoTaskCount(c *echo.Context) error {
	count, err := b.GetUndoTaskCount()
	if err != nil {
		return c.JSON(http.StatusForbidden, err.Error())
	}
	return c.JSON(http.StatusOK, count)
}

func (b *Broker) FailTaskCount(c *echo.Context) error {
	date := c.Param("date")
	count, err := b.GetFailTaskCount(date)
	if err != nil {
		return c.JSON(http.StatusForbidden, err.Error())
	}
	return c.JSON(http.StatusOK, count)
}

func (b *Broker) SuccessTaskCount(c *echo.Context) error {
	date := c.Param("date")
	count, err := b.GetSuccessTaskCount(date)
	if err != nil {
		return c.JSON(http.StatusForbidden, err.Error())
	}
	return c.JSON(http.StatusOK, count)
}
