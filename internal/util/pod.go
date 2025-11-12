package util

import (
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
)

const (
	// AnnoClaim stores the claim name a pod references.
	AnnoClaim = "gpu.scheduling/claim"
	// AnnoAllocated stores the resolved `node:ids` payload for webhook consumption.
	AnnoAllocated = "gpu.scheduling/allocated"
)

// SetAllocated annotates the pod with the resolved node and GPU ids.
func SetAllocated(p *corev1.Pod, node string, ids []int) {
	m := p.GetAnnotations()
	if m == nil {
		m = map[string]string{}
	}
	b, _ := json.Marshal(ids)
	m[AnnoAllocated] = node + ":" + trimList(b)
	p.Annotations = m
}

func trimList(b []byte) string {
	if len(b) >= 2 && b[0] == '[' && b[len(b)-1] == ']' {
		return string(b[1 : len(b)-1])
	}
	return string(b)
}
