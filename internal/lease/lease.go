package lease

import (
	"context"
	"fmt"

	coordv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	coordclient "k8s.io/client-go/kubernetes/typed/coordination/v1"
)

// LeaseName deterministically maps a node and GPU id to the lease resource identifier.
func LeaseName(node string, id int) string {
	return fmt.Sprintf("gpu-%s-%d", node, id)
}

// TryAcquire attempts to create a lease per GPU id. Success indicates this pod owns the GPU.
func TryAcquire(
	ctx context.Context,
	cli coordclient.CoordinationV1Interface,
	ns, node, holder string,
	id int,
) (bool, error) {
	name := LeaseName(node, id)
	lease := &coordv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: coordv1.LeaseSpec{
			HolderIdentity: strPtr(holder),
		},
	}
	if _, err := cli.Leases(ns).Create(ctx, lease, metav1.CreateOptions{}); err != nil {
		return false, err
	}
	return true, nil
}

// Release drops the lease so other pods may use the GPU.
func Release(ctx context.Context, cli coordclient.CoordinationV1Interface, ns, node string, id int) error {
	return cli.Leases(ns).Delete(ctx, LeaseName(node, id), metav1.DeleteOptions{})
}

func strPtr(s string) *string { return &s }
