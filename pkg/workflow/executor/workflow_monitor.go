package executor

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/conductor-sdk/conductor-go/pkg/concurrency"
	"github.com/conductor-sdk/conductor-go/pkg/conductor_client/conductor_http_client"
	"github.com/conductor-sdk/conductor-go/pkg/http_model"
	"github.com/sirupsen/logrus"
)

type WorkflowExecutionChannel chan *http_model.Workflow

type WorkflowMonitor struct {
	mutex                        sync.Mutex
	refreshInterval              time.Duration
	executionChannelByWorkflowId map[string]WorkflowExecutionChannel
	workflowClient               *conductor_http_client.WorkflowResourceApiService
}

const (
	defaultMonitorRunningWorkflowsRefreshInterval = 100 * time.Millisecond
)

func NewWorkflowMonitor(workflowClient *conductor_http_client.WorkflowResourceApiService) *WorkflowMonitor {
	workflowMonitor := &WorkflowMonitor{
		refreshInterval:              defaultMonitorRunningWorkflowsRefreshInterval,
		executionChannelByWorkflowId: make(map[string]WorkflowExecutionChannel),
		workflowClient:               workflowClient,
	}
	go workflowMonitor.monitorRunningWorkflowsDaemon()
	return workflowMonitor
}

func (w *WorkflowMonitor) GenerateWorkflowExecutionChannel(workflowId string) (WorkflowExecutionChannel, error) {
	channel := make(WorkflowExecutionChannel, 1)
	w.addWorkflowExecutionChannel(workflowId, channel)
	return channel, nil
}

func (w *WorkflowMonitor) monitorRunningWorkflowsDaemon() {
	defer concurrency.HandlePanicError("monitor_running_workflows")
	for {
		err := w.monitorRunningWorkflows()
		if err != nil {
			logrus.Warning(
				"Failed to monitor running workflows",
				", error: ", err.Error(),
			)
		}
		time.Sleep(w.refreshInterval)
	}
}

func (w *WorkflowMonitor) monitorRunningWorkflows() error {
	workflowsInTerminalState, err := w.getWorkflowsInTerminalState()
	if err != nil {
		return err
	}
	for _, workflow := range workflowsInTerminalState {
		err = w.notifyFinishedWorkflow(workflow.WorkflowId, workflow)
		if err != nil {
			return err
		}
	}
	return nil
}

func (w *WorkflowMonitor) getWorkflowsInTerminalState() ([]*http_model.Workflow, error) {
	runningWorkflowIdList, err := w.getRunningWorkflowIdList()
	if err != nil {
		return nil, err
	}
	workflowsInTerminalState := make([]*http_model.Workflow, 0)
	for _, workflowId := range runningWorkflowIdList {
		workflow, response, err := w.workflowClient.GetExecutionStatus(
			context.Background(),
			workflowId,
			nil,
		)
		if err != nil {
			logrus.Debug(
				"Failed to get workflow execution status",
				", reason: ", err.Error(),
				", workflowId: ", workflowId,
				", response: ", response,
			)
			return nil, err
		}
		if IsWorkflowInTerminalState(&workflow) {
			workflowsInTerminalState = append(workflowsInTerminalState, &workflow)
		}
	}
	return workflowsInTerminalState, nil
}

func (w *WorkflowMonitor) addWorkflowExecutionChannel(workflowId string, executionChannel WorkflowExecutionChannel) error {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	w.executionChannelByWorkflowId[workflowId] = executionChannel
	logrus.Debug(
		fmt.Sprint(
			"Added workflow execution channel",
			", workflowId: ", workflowId,
		),
	)
	return nil
}

func (w *WorkflowMonitor) getRunningWorkflowIdList() ([]string, error) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	i := 0
	runningWorkflowIdList := make([]string, len(w.executionChannelByWorkflowId))
	for workflowId := range w.executionChannelByWorkflowId {
		runningWorkflowIdList[i] = workflowId
		i += 1
	}
	return runningWorkflowIdList, nil
}

func (w *WorkflowMonitor) notifyFinishedWorkflow(workflowId string, workflow *http_model.Workflow) error {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	logrus.Debug(fmt.Sprintf("Notifying finished workflow: %+v", *workflow))
	executionChannel, ok := w.executionChannelByWorkflowId[workflowId]
	if !ok {
		return fmt.Errorf("execution channel not found for workflowId: %s", workflowId)
	}
	executionChannel <- workflow
	logrus.Debug("Sent finished workflow through channel")
	close(executionChannel)
	logrus.Debug("Closed client workflow execution channel")
	delete(w.executionChannelByWorkflowId, workflowId)
	logrus.Debug("Deleted workflow execution channel")
	return nil
}