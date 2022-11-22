package discovery

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	cleanhttp "github.com/hashicorp/go-cleanhttp"
	log "github.com/sirupsen/logrus"
)

// K8sServices represents the payload that is returned by `kubectl get services -o json`
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
				NodePort   int    `json:"nodePort"`
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

// K8sServices represents a cut-down version of the payload that is returned by
// `kubectl get services -o json`
type K8sNodes struct {
	APIVersion string    `json:"apiVersion"`
	Items      []K8sNode `json:"items"`
	Kind       string    `json:"kind"`
	Metadata   struct {
		ResourceVersion string `json:"resourceVersion"`
	} `json:"metadata"`
}

// A K8sNode represents a single node from the K8sNodes wrapper structure
type K8sNode struct {
	Status struct {
		Addresses []K8sNodeAddress `json:"addresses"`
	} `json:"status"`
}

type K8sNodeAddress struct {
	Address string `json:"address"`
	Type    string `json:"type"`
}

// A K8sDiscoveryAdapter wraps a call to an external command that can be used
// to discover services running on a Kubernetes cluster. This is normally
// `kubectl` but for tests, this allows mocking out the underlying call.
type K8sDiscoveryAdapter interface {
	GetServices() ([]byte, error)
	GetNodes() ([]byte, error)
}

// KubeAPIDiscoveryCommand is the main implementation for K8sDiscoveryCommand
type KubeAPIDiscoveryCommand struct {
	Namespace string
	Timeout   time.Duration

	KubeHost string
	KubePort int

	token  string
	client *http.Client
}

// NewKubeAPIDiscoveryCommand returns a properly configured KubeAPIDiscoveryCommand
func NewKubeAPIDiscoveryCommand(kubeHost string, kubePort int, namespace string, timeout time.Duration, credsPath string) *KubeAPIDiscoveryCommand {
	d := &KubeAPIDiscoveryCommand{
		Namespace: namespace,
		Timeout:   timeout,
		KubeHost:  kubeHost,
		KubePort:  kubePort,
	}
	// Cache the secret from the file
	data, err := ioutil.ReadFile(credsPath + "/token")
	if err != nil {
		log.Errorf("Failed to read serviceaccount token: %s", err)
		return nil
	}

	// New line is illegal in tokens
	d.token = strings.Replace(string(data), "\n", "", -1)

	// Set up the timeout on a clean HTTP client
	d.client = cleanhttp.DefaultClient()
	d.client.Timeout = d.Timeout

	// Get the SystemCertPool — on error we have empty pool
	rootCAs, _ := x509.SystemCertPool()
	if rootCAs == nil {
		rootCAs = x509.NewCertPool()
	}

	certs, err := ioutil.ReadFile(credsPath + "/ca.crt")
	if err != nil {
		log.Warnf("Failed to load CA cert file: %s", err)
	}

	if ok := rootCAs.AppendCertsFromPEM(certs); !ok {
		log.Warn("No certs appended! Using system certs only")
	}

	// Add the pool to the TLS config we'll use in the client.
	config := &tls.Config{
		RootCAs: rootCAs,
	}

	d.client.Transport = &http.Transport{TLSClientConfig: config}

	return d
}

func (d *KubeAPIDiscoveryCommand) makeRequest(path string) ([]byte, error) {
	var scheme = "http"
	if d.KubePort == 443 {
		scheme = "https"
	}

	apiURL := url.URL{
		Scheme: scheme,
		Host:   fmt.Sprintf("%s:%d", d.KubeHost, d.KubePort),
		Path:   path,
	}

	req, err := http.NewRequest("GET", apiURL.String(), nil)
	if err != nil {
		return []byte{}, err
	}

	req.Header.Set("Authorization", "Bearer "+d.token)

	resp, err := d.client.Do(req)
	if err != nil {
		return []byte{}, fmt.Errorf("failed to fetch from K8s API '%s': %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode > 299 || resp.StatusCode < 200 {
		return []byte{}, fmt.Errorf("got unexpected response code from %s: %d", path, resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return []byte{}, fmt.Errorf("failed to read from K8s API '%s' response body: %w", path, err)
	}

	return body, nil
}

func (d *KubeAPIDiscoveryCommand) GetServices() ([]byte, error) {
	return d.makeRequest("/api/v1/services/")
}

func (d *KubeAPIDiscoveryCommand) GetNodes() ([]byte, error) {
	return d.makeRequest("/api/v1/nodes/")
}
