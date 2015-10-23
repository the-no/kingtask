package broker

import (
	"net/http"

	"github.com/flike/golog"
	"github.com/flike/kingtask/core/errors"
	"github.com/flike/kingtask/task"
	"github.com/labstack/echo"
	mw "github.com/labstack/echo/middleware"
	"github.com/pborman/uuid"
)

func (b *Broker) RegisterMiddleware() {
	b.web.Use(mw.Logger())
	b.web.Use(mw.Recover())
}

func (b *Broker) RegisterURL() {
	b.web.Post("/api/v1/task", b.CreateTaskRequest)
	b.web.Get("/api/v1/task/result/:uuid", b.GetTaskResult)
}

func (b *Broker) CreateTaskRequest(c *echo.Context) error {
	args := struct {
		BinName      string `json:"bin_name"`
		Args         string `json:"args"` //空格分隔各个参数
		StartTime    int64  `json:"start_time,string"`
		TimeInterval string `json:"time_interval"` //空格分隔各个参数
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
	err = b.HandleRequest(taskRequest)
	if err != nil {
		return c.JSON(http.StatusForbidden, err.Error())
	}
	golog.Info("Broker", "CreateTaskRequest", "ok", 0,
		"uuid", taskRequest.Uuid,
		"bin_name", taskRequest.BinName,
		"args", taskRequest.Args,
		"start_time", taskRequest.StartTime,
		"time_interval", taskRequest.TimeInterval,
		"index", taskRequest.Index,
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
