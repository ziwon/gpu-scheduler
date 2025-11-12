package main

import (
	"os"

	"k8s.io/component-base/logs"
	"k8s.io/klog/v2"
	kubescheduler "k8s.io/kube-scheduler/cmd/kube-scheduler/app"

	"github.com/aaronlab/gpu-scheduler/internal/plugin/gpuclaim"
)

func main() {
	logs.InitLogs()
	defer logs.FlushLogs()

	command := kubescheduler.NewSchedulerCommand(
		kubescheduler.WithPlugin(gpuclaim.Name, gpuclaim.New),
	)
	if err := command.Execute(); err != nil {
		klog.ErrorS(err, "scheduler exited with error")
		os.Exit(1)
	}
}
