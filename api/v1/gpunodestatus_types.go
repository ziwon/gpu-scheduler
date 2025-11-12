package v1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=gns
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=.spec.nodeName
// +kubebuilder:printcolumn:name="Devices",type=integer,JSONPath=.status.total

// GpuNodeStatus is posted by the DaemonSet agent.
type GpuNodeStatus struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GpuNodeStatusSpec   `json:"spec,omitempty"`
	Status GpuNodeStatusStatus `json:"status,omitempty"`
}

// GpuNodeStatusSpec links the CR to a node.
type GpuNodeStatusSpec struct {
	NodeName string `json:"nodeName"`
}

// Device carries per GPU metadata.
type Device struct {
	ID        int      `json:"id"`
	InUseBy   []string `json:"inUseBy,omitempty"` // pod UIDs
	Health    string   `json:"health,omitempty"`  // Healthy|Unhealthy|Other
	Bandwidth int      `json:"bandwidthGBps,omitempty"`
	Island    string   `json:"island,omitempty"` // NVLink island identifier
}

// GpuNodeStatusStatus holds aggregated telemetry.
type GpuNodeStatusStatus struct {
	Devices []Device `json:"devices,omitempty"`
	Total   int      `json:"total,omitempty"`
}

// +kubebuilder:object:root=true

// GpuNodeStatusList lists node status objects.
type GpuNodeStatusList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GpuNodeStatus `json:"items"`
}
