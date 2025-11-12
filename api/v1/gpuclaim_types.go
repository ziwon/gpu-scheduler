package v1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=gclaim
// +kubebuilder:printcolumn:name="Req",type=string,JSONPath=.spec.devices.count
// +kubebuilder:printcolumn:name="Policy",type=string,JSONPath=.spec.devices.policy
// +kubebuilder:printcolumn:name="Topology",type=string,JSONPath=.spec.topology.mode
// +kubebuilder:printcolumn:name="Allocated",type=string,JSONPath=.status.allocated

// GpuClaim defines a declarative GPU allocation request.
type GpuClaim struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GpuClaimSpec   `json:"spec,omitempty"`
	Status GpuClaimStatus `json:"status,omitempty"`
}

// GpuClaimSpec encodes the desired placement.
type GpuClaimSpec struct {
	Selector *NodeSelector   `json:"selector,omitempty"`
	Devices  DeviceRequest   `json:"devices"`
	Topology *TopologyPolicy `json:"topology,omitempty"`
	// Optional: link to an external PodGroup (Volcano/Kueue). Keep MVP simple.
	GangRef string `json:"gangRef,omitempty"`
}

// NodeSelector mirrors corev1 label selector semantics (simplified for MVP).
type NodeSelector struct {
	MatchLabels map[string]string `json:"matchLabels,omitempty"`
}

// DeviceRequest describes GPU needs.
type DeviceRequest struct {
	Count       int    `json:"count"`
	Policy      string `json:"policy,omitempty"`      // contiguous|spread|preferIds
	PreferIDs   []int  `json:"preferIds,omitempty"`   // optional pinned ids
	Exclusivity string `json:"exclusivity,omitempty"` // Exclusive|Shared|MIG
}

// TopologyPolicy encodes NVLink bandwidth preferences.
type TopologyPolicy struct {
	Mode             string `json:"mode,omitempty"` // Required|Preferred|Ignore
	MinBandwidthGBps int    `json:"minBandwidthGBps,omitempty"`
}

// GpuClaimStatus reflects scheduler progress.
type GpuClaimStatus struct {
	Phase     string `json:"phase,omitempty"` // Pending|Reserved|Bound|Failed
	NodeName  string `json:"nodeName,omitempty"`
	GPUIds    []int  `json:"gpuIds,omitempty"`
	Allocated string `json:"allocated,omitempty"` // e.g. node-a:0,1,2
	Message   string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true

// GpuClaimList lists GpuClaim objects.
type GpuClaimList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GpuClaim `json:"items"`
}
