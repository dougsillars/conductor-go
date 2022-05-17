package workflow

import (
	workflow_status "github.com/conductor-sdk/conductor-go/pkg/model/enum"
)

type TerminateTask struct {
	Task
}

func Terminate(taskRefName string, status workflow_status.WorkflowStatus, terminationReason string) *TerminateTask {
	return &TerminateTask{
		Task{
			name:              taskRefName,
			taskReferenceName: taskRefName,
			description:       "",
			taskType:          TERMINATE,
			optional:          false,
			inputParameters: map[string]interface{}{
				"terminationStatus": status,
				"terminationReason": terminationReason,
			},
		},
	}
}