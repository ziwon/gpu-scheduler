package gpuclaim

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
    utilruntime "k8s.io/apimachinery/pkg/util/runtime"
    clientset "k8s.io/client-go/kubernetes"
    coordclient "k8s.io/client-go/kubernetes/typed/coordination/v1"
    "k8s.io/klog/v2"
    framework "k8s.io/kubernetes/pkg/scheduler/framework"
    crclient "sigs.k8s.io/controller-runtime/pkg/client"

    apiv1 "github.com/ziwon/gpu-scheduler/api/v1"
    "github.com/ziwon/gpu-scheduler/internal/lease"
    "github.com/ziwon/gpu-scheduler/internal/util"
)

const (
	// Name exposes the plugin identifier to the framework.
	Name = "GpuClaimPlugin"

	defaultGPUCount = 1
	maxGPUID        = 16 // MVP assumption: at most 17 devices per host. Can be 64 with virtual GPUs on NVIDIA H200, B200
)

var (
	_ framework.PreFilterPlugin = &Plugin{}
	_ framework.FilterPlugin    = &Plugin{}
	_ framework.ScorePlugin     = &Plugin{}
	_ framework.ReservePlugin   = &Plugin{}
	_ framework.PreBindPlugin   = &Plugin{}
	_ framework.StateData       = &stateData{}
)

// stateData is stored in CycleState.
type stateData struct {
	claimName  string
	reqCount   int
	chosenIDs  []int
	chosenNode string
}

func (s *stateData) Clone() framework.StateData {
	if s == nil {
		return nil
	}
	out := *s
	out.chosenIDs = append([]int(nil), s.chosenIDs...)
	return &out
}

// Plugin implements scheduler hooks.
type Plugin struct {
	client clientset.Interface
	coord  coordclient.CoordinationV1Interface
	crcClient crclient.Client
}

// Name satisfies framework.Plugin interface.
func (p *Plugin) Name() string { return Name }

// New constructs a Plugin instance.
// func New(_ context.Context, _ runtime.Object, handle framework.Handle) (framework.Plugin, error) {
// 	return &Plugin{
// 		client: handle.ClientSet(),
// 		coord:  handle.ClientSet().CoordinationV1(),
// 	}, nil
// }

func New(_ context.Context, _ runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	cs := handle.ClientSet()

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(apiv1.AddToScheme(scheme))

	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("build kube config: %v", err)
	}

	c, err := crclient.New(cfg, crclient.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("build controller-runtime client: %v", err)
	}

	return &Plugin{
		client:    cs,
		coord:     cs.CoordinationV1(),
		crcClient: c,
	}, nil
}

// PreFilter reads annotations and seeds scheduler state.
func (p *Plugin) PreFilter(
    ctx context.Context,
    cycleState *framework.CycleState,
    pod *corev1.Pod,
) (*framework.PreFilterResult, *framework.Status) {
    claimName := pod.GetAnnotations()[util.AnnoClaim]
    if claimName == "" {
        return nil, framework.NewStatus(framework.Unschedulable, "gpu claim annotation missing")
    }

	// Fetch the GpuClaim referenced by the pod.
    claim := &apiv1.GpuClaim{}
    if err := p.crClient.Get(ctx, types.NamespacedName{
        Namespace: pod.Namespace,
        Name:      claimName,
    }, claim); err != nil {
		// Returning Error here would affect the entire scheduler; for now, return Unschedulable
        msg := fmt.Sprintf("failed to get GpuClaim %q: %v", claimName, err)
        return nil, framework.NewStatus(framework.Unschedulable, msg)
    }

	// Use devices.count, default to defaultGPUCount if not specified
    reqCount := claim.Spec.Devices.Count
    if reqCount <= 0 {
        reqCount = defaultGPUCount
    }

    state := &stateData{
        claimName: claimName,
        reqCount:  reqCount,
    }
    cycleState.Write(Name, state)
    return nil, nil
}

func (p *Plugin) PreFilterExtensions() framework.PreFilterExtensions { return nil }

// Filter enforces node selector requirements (noop for MVP).
func (p *Plugin) Filter(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	return nil
}

// Score favors nodes with contiguous GPUs. MVP stub returns static score.
func (p *Plugin) Score(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeInfo *framework.NodeInfo) (int64, *framework.Status) {
	return 1, nil
}

func (p *Plugin) ScoreExtensions() framework.ScoreExtensions { return nil }

// Reserve acquires GPU leases on the chosen node.
func (p *Plugin) Reserve(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeName string) *framework.Status {
	data, err := readState(cycleState)
	if err != nil {
		return framework.NewStatus(framework.Error, err.Error())
	}
	data.chosenNode = nodeName

	// Fetch the GpuNodeStatus to see available devices.
	gns, err := p.getGpuNodeStatus(ctx, nodeName)
	if err != nil {
		return framework.NewStatus(framework.Error, fmt.Sprintf("get GpuNodeStatus: %v", err))
	}

	// Check if the node has any GPU devices.
	if len(gns.Status.Devices)  == 0 {
		return framework.NewStatus(framework.Unschedulable, "node has no GPU devices")
	}

	// Try to acquire leases for the requested GPU count.
	var allocated []int
	for _, dev := range gns.Status.Devices {
		if len(allocated) >= data.reqCount {
			break
		}

		id := dev.ID
		ok, err := lease.TryAcquire(ctx, p.coord, pod.Namespace, nodeName, string(pod.UID), id)
        if err != nil {
            klog.V(4).InfoS("lease acquisition failed", "node", nodeName, "gpuID", id, "err", err)
            continue
        }
        if ok {
            allocated = append(allocated, id)
        }
	}

	// Check if we acquired enough GPUs.
	if len(allocated) < data.reqCount {
		total := len(gns.Status.Devices)
		free := 0
		for _, dev := range gns.Status.Devices {
			_ = dev
		}
		klog.V(4).InfoS("not enough GPUs available", "node", nodeName, "requested", data.reqCount, "allocated", len(allocated), "free", free, "total", total)
		// Release any partial allocations.
		for _, id := range allocated {
			_ = lease.Release(ctx, p.coord, pod.Namespace, nodeName, id)
		}
		msg := fmt.Sprintf("not enough GPUs available on node %s (requested=%d, total=%d)", nodeName, data.reqCount, total)
		return framework.NewStatus(framework.Unschedulable, msg)
	}

	data.chosenIDs = allocated
	cycleState.Write(Name, data)
	return nil
}

// Unreserve releases leases when scheduling fails.
func (p *Plugin) Unreserve(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeName string) {
	data, err := readState(cycleState)
	if err != nil {
		return
	}
	for _, id := range data.chosenIDs {
		_ = lease.Release(ctx, p.coord, pod.Namespace, nodeName, id)
	}
}

// PreBind persists allocation annotations so the webhook can inject env vars.
func (p *Plugin) PreBind(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeName string) *framework.Status {
	data, err := readState(cycleState)
	if err != nil {
		return framework.NewStatus(framework.Error, err.Error())
	}

	util.SetAllocated(pod, nodeName, data.chosenIDs)
	payload := map[string]interface{}{
		"metadata": map[string]interface{}{
			"annotations": map[string]string{
				util.AnnoAllocated: pod.Annotations[util.AnnoAllocated],
			},
		},
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return framework.NewStatus(framework.Error, err.Error())
	}

	if _, err := p.client.CoreV1().Pods(pod.Namespace).Patch(ctx, pod.Name, types.MergePatchType, b, metav1.PatchOptions{}); err != nil {
		return framework.NewStatus(framework.Error, fmt.Sprintf("patch pod annotations: %v", err))
	}
	return nil
}

func (p *Plugin) getGpuNodeStatus(ctx context.Context, nodeName string) (*apiv1.GpuNodeStatus, error) {
	gns := &apiv1.GpuNodeStatus{}
	if err := p.crcClient.Get(ctx, types.NamespacedName{Name: nodeName}, gns); err != nil {
		return nil, err
	}
	return gns, nil
}

func readState(cycleState *framework.CycleState) (*stateData, error) {
	raw, err := cycleState.Read(Name)
	if err != nil {
		return nil, err
	}
	state, ok := raw.(*stateData)
	if !ok {
		return nil, fmt.Errorf("unexpected state type %T", raw)
	}
	return state, nil
}
