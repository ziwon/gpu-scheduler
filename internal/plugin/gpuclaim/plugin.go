package gpuclaim

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientset "k8s.io/client-go/kubernetes"
	coordclient "k8s.io/client-go/kubernetes/typed/coordination/v1"
	"k8s.io/klog/v2"
	framework "k8s.io/kubernetes/pkg/scheduler/framework"

	"github.com/aaronlab/gpu-scheduler/internal/lease"
	"github.com/aaronlab/gpu-scheduler/internal/util"
)

const (
	// Name exposes the plugin identifier to the framework.
	Name = "GpuClaimPlugin"

	defaultGPUCount = 1
	maxGPUID        = 16 // MVP assumption: at most 16 devices per host.
)

var (
	_ framework.PreFilterPlugin = &Plugin{}
	_ framework.FilterPlugin    = &Plugin{}
	_ framework.ScorePlugin     = &Plugin{}
	_ framework.ReservePlugin   = &Plugin{}
	_ framework.UnreservePlugin = &Plugin{}
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
}

// Name satisfies framework.Plugin interface.
func (p *Plugin) Name() string { return Name }

// New constructs a Plugin instance.
func New(_ runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	return &Plugin{
		client: handle.ClientSet(),
		coord:  handle.ClientSet().CoordinationV1(),
	}, nil
}

// PreFilter reads annotations and seeds scheduler state.
func (p *Plugin) PreFilter(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod) (*framework.PreFilterResult, *framework.Status) {
	claimName := pod.GetAnnotations()[util.AnnoClaim]
	if claimName == "" {
		return nil, framework.NewStatus(framework.Unschedulable, "gpu claim annotation missing")
	}
	state := &stateData{
		claimName: claimName,
		reqCount:  defaultGPUCount,
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
func (p *Plugin) Score(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeName string) (int64, *framework.Status) {
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

	var allocated []int
	for id := 0; id < maxGPUID && len(allocated) < data.reqCount; id++ {
		ok, err := lease.TryAcquire(ctx, p.coord, pod.Namespace, nodeName, string(pod.UID), id)
		if err != nil {
			klog.V(4).InfoS("lease acquisition failed", "node", nodeName, "gpuID", id, "err", err)
		}
		if ok {
			allocated = append(allocated, id)
		}
	}
	if len(allocated) != data.reqCount {
		for _, id := range allocated {
			_ = lease.Release(ctx, p.coord, pod.Namespace, nodeName, id)
		}
		return framework.NewStatus(framework.Unschedulable, "not enough free GPUs")
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
