package model

import "fmt"

// 状态定义
type TaskStatus int

const (
	Unclaimed TaskStatus = iota
	Claimed
	Done
)

func (s TaskStatus) String() string {
	switch s {
	case Unclaimed:
		return "unclaimed"
	case Claimed:
		return "claimed"
	case Done:
		return "done"
	default:
		return "unknown"
	}
}

// 解析 TaskStatus 的函数
func ParseTaskStatus(s string) (TaskStatus, error) {
	switch s {
	case "unclaimed":
		return Unclaimed, nil
	case "claimed":
		return Claimed, nil
	case "done":
		return Done, nil
	default:
		return -1, fmt.Errorf("invalid status: %s", s)
	}
}

type Task struct {
	CmdKey     string
	Command    string
	ResultChan chan TaskResult // 每个任务自己的结果通道
}

type TaskResult struct {
	CmdKey   string
	Status   string
	StdOut   string
	StdErr   string
	Err      error
	ExitCode int
}

// Task 的字符串表示
func (t Task) String() string {
	return fmt.Sprintf("{CmdKey: %s, Command: %s}", t.CmdKey, t.Command)
}

func (t TaskResult) String() string {
	return fmt.Sprintf("{CmdKey: %s, Status: %s, StdOut: %s, StdErr: %s, Err: %v, ExitCode: %d}", t.CmdKey, t.Status, t.StdOut, t.StdErr, t.Err, t.ExitCode)
}
