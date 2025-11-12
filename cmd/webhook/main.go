package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"strings"

	admv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/aaronlab/gpu-scheduler/internal/util"
)

var (
	tlsCert = flag.String("tls-cert-file", "/certs/tls.crt", "Path to TLS certificate")
	tlsKey  = flag.String("tls-private-key-file", "/certs/tls.key", "Path to TLS private key")
	addr    = flag.String("addr", ":8443", "Webhook listen address")
)

func main() {
	flag.Parse()
	http.HandleFunc("/mutate", mutate)
	if err := http.ListenAndServeTLS(*addr, *tlsCert, *tlsKey, nil); err != nil {
		panic(err)
	}
}

func mutate(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var review admv1.AdmissionReview
	if err := json.NewDecoder(r.Body).Decode(&review); err != nil {
		writeResponse(w, admissionError(review, err))
		return
	}
	if review.Request == nil {
		writeResponse(w, admissionError(review, fmt.Errorf("empty request")))
		return
	}

	pod := &corev1.Pod{}
	if err := json.Unmarshal(review.Request.Object.Raw, pod); err != nil {
		writeResponse(w, admissionError(review, err))
		return
	}

	allocated := pod.GetAnnotations()[util.AnnoAllocated]
	response := &admv1.AdmissionResponse{
		UID:     review.Request.UID,
		Allowed: true,
	}
	if allocated == "" || len(pod.Spec.Containers) == 0 {
		review.Response = response
		writeResponse(w, review)
		return
	}

	devices := deviceIDs(allocated)
	patch := buildPatch(pod, devices)
	patchBytes, err := json.Marshal(patch)
	if err != nil {
		writeResponse(w, admissionError(review, err))
		return
	}

	pt := admv1.PatchTypeJSONPatch
	response.PatchType = &pt
	response.Patch = patchBytes
	review.Response = response
	writeResponse(w, review)
}

func buildPatch(pod *corev1.Pod, devices string) []map[string]interface{} {
	var ops []map[string]interface{}
	for i, c := range pod.Spec.Containers {
		envPath := fmt.Sprintf("/spec/containers/%d/env", i)
		value := map[string]string{
			"name":  "CUDA_VISIBLE_DEVICES",
			"value": devices,
		}
		if len(c.Env) == 0 {
			ops = append(ops, map[string]interface{}{
				"op":   "add",
				"path": envPath,
				"value": []map[string]string{
					value,
				},
			})
		} else {
			ops = append(ops, map[string]interface{}{
				"op":    "add",
				"path":  envPath + "/-",
				"value": value,
			})
		}
	}
	return ops
}

func deviceIDs(allocated string) string {
	parts := strings.SplitN(allocated, ":", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return allocated
}

func admissionError(review admv1.AdmissionReview, err error) admv1.AdmissionReview {
	review.Response = &admv1.AdmissionResponse{
		Result: &metav1.Status{
			Message: err.Error(),
		},
	}
	return review
}

func writeResponse(w http.ResponseWriter, review admv1.AdmissionReview) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(review)
}
