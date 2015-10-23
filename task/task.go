package task

type TaskRequest struct {
	Uuid         string `json:"uuid"`
	BinName      string `json:"bin_name"`
	Args         string `json:"args"` //空格分隔各个参数
	StartTime    int64  `json:"start_time,string"`
	TimeInterval string `json:"time_interval"` //空格分隔各个参数
	Index        int    `json:"index,string"`
}

type TaskResult struct {
	TaskRequest
	IsSuccess int64  `json:"is_success"`
	Result    string `json:"result"`
}

type Reply struct {
	IsResultExist int    `json:"is_result_exist"`
	IsSuccess     int    `json:"is_success"`
	Result        string `json:"message"`
}
