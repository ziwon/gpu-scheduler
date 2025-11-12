package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apiv1 "github.com/aaronlab/gpu-scheduler/api/v1"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg, err := rest.InClusterConfig()
	if err != nil {
		klog.Fatalf("build kube config: %v", err)
	}

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(apiv1.AddToScheme(scheme))

	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		klog.Fatalf("build client: %v", err)
	}

	nodeName := os.Getenv("NODE_NAME")
	if nodeName == "" {
		nodeName = os.Getenv("HOSTNAME")
	}
	if nodeName == "" {
		klog.Fatalf("NODE_NAME env missing")
	}

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			devices := discoverDevices()
			if err := publishStatus(ctx, c, nodeName, devices); err != nil {
				klog.ErrorS(err, "failed to publish GPU status")
			}
		}
	}
}

func discoverDevices() []apiv1.Device {
	// TODO: wire to NVML. For now emit a placeholder entry so flows can be tested.
	return []apiv1.Device{
		{
			ID:        0,
			Health:    "Unknown",
			Bandwidth: 0,
			Island:    "default",
		},
	}
}

func publishStatus(ctx context.Context, c client.Client, nodeName string, devices []apiv1.Device) error {
	obj := &apiv1.GpuNodeStatus{
		TypeMeta:   metav1.TypeMeta{APIVersion: "gpu.scheduling/v1", Kind: "GpuNodeStatus"},
		ObjectMeta: metav1.ObjectMeta{Name: nodeName},
		Spec: apiv1.GpuNodeStatusSpec{
			NodeName: nodeName,
		},
	}
	if err := c.Patch(ctx, obj, client.Apply, client.ForceOwnership(true), client.FieldOwner("gpu-agent")); err != nil {
		return err
	}

	status := &apiv1.GpuNodeStatus{
		TypeMeta:   obj.TypeMeta,
		ObjectMeta: obj.ObjectMeta,
		Status: apiv1.GpuNodeStatusStatus{
			Devices: devices,
			Total:   len(devices),
		},
	}
	return c.Status().Patch(ctx, status, client.Apply, client.ForceOwnership(true), client.FieldOwner("gpu-agent-status"))
}
