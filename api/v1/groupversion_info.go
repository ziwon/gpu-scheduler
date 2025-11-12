package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	// GroupVersion identifies the API group/versions for CRDs.
	GroupVersion = schema.GroupVersion{Group: "gpu.scheduling", Version: "v1"}

	SchemeBuilder = runtime.NewSchemeBuilder(func(scheme *runtime.Scheme) error {
		scheme.AddKnownTypes(GroupVersion,
			&GpuClaim{},
			&GpuClaimList{},
			&GpuNodeStatus{},
			&GpuNodeStatusList{},
		)
		metav1.AddToGroupVersion(scheme, GroupVersion)
		return nil
	})

	// AddToScheme registers the API types with a Scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)
