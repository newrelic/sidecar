package discovery

import (
	"os/exec"
	"time"
)

type K8sServices struct {
	APIVersion string `json:"apiVersion"`
	Items      []struct {
		APIVersion string `json:"apiVersion"`
		Kind       string `json:"kind"`
		Metadata   struct {
			Annotations struct {
				KubectlKubernetesIoLastAppliedConfiguration string `json:"kubectl.kubernetes.io/last-applied-configuration"`
			} `json:"annotations"`
			CreationTimestamp time.Time `json:"creationTimestamp"`
			Labels            struct {
				Environment string `json:"Environment"`
				ServiceName string `json:"ServiceName"`
			} `json:"labels"`
			Name            string `json:"name"`
			Namespace       string `json:"namespace"`
			ResourceVersion string `json:"resourceVersion"`
			UID             string `json:"uid"`
		} `json:"metadata"`
		Spec struct {
			ClusterIP             string   `json:"clusterIP"`
			ClusterIPs            []string `json:"clusterIPs"`
			InternalTrafficPolicy string   `json:"internalTrafficPolicy"`
			IPFamilies            []string `json:"ipFamilies"`
			IPFamilyPolicy        string   `json:"ipFamilyPolicy"`
			Ports                 []struct {
				Port       int    `json:"port"`
				Protocol   string `json:"protocol"`
				TargetPort int    `json:"targetPort"`
			} `json:"ports"`
			Selector struct {
				Environment string `json:"Environment"`
				ServiceName string `json:"ServiceName"`
			} `json:"selector"`
			SessionAffinity string `json:"sessionAffinity"`
			Type            string `json:"type"`
		} `json:"spec"`
		Status struct {
			LoadBalancer struct {
			} `json:"loadBalancer"`
		} `json:"status"`
	} `json:"items"`
	Kind     string `json:"kind"`
	Metadata struct {
		ResourceVersion string `json:"resourceVersion"`
	} `json:"metadata"`
}

// A K8sDiscoveryCommand wraps a call to an external command that can be used
// to discover services running on a Kubernetes cluster. This is normally
// `kubectl` but for tests, this allows mocking out the underlying call.
type K8sDiscoveryCommand interface {
	Run() ([]byte, error)
}

// KubectlDiscoveryCommand is the main implementation for K8sDiscoveryCommand
type KubectlDiscoveryCommand struct {
	Path      string
	Namespace string
}

func (d *KubectlDiscoveryCommand) Run() ([]byte, error) {
	// Run `kubectl` from the specific path, and namespace, and return data as
	// JSON, to be parsed by the Discoverer
	return exec.Command(d.Path, "-n", d.Namespace, "get", "services", "-o", "json").CombinedOutput()
}
