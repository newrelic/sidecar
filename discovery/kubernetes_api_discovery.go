package discovery

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/NinesStack/sidecar/service"
	"github.com/relistan/go-director"
	log "github.com/sirupsen/logrus"
)

// A K8sAPIDiscoverer is a discovery mechanism that assumes that a K8s cluster
// with be fronted by a load balancer and that all the ports exposed will match
// up on both the load balancer and the backing pods. It relies on an underlying
// command to run the discovery. This is normally `kubectl`.
type K8sAPIDiscoverer struct {
	Namespace string

	Command K8sDiscoveryAdapter

	discoveredSvcs  *K8sServices
	discoveredNodes *K8sNodes
	lock            sync.RWMutex
}

// NewK8sAPIDiscoverer returns a properly configured K8sAPIDiscoverer
func NewK8sAPIDiscoverer(kubeHost string, kubePort int, namespace string, timeout time.Duration, credsPath string) *K8sAPIDiscoverer {

	cmd := NewKubeAPIDiscoveryCommand(kubeHost, kubePort, namespace, timeout, credsPath)

	return &K8sAPIDiscoverer{
		discoveredSvcs:  &K8sServices{},
		discoveredNodes: &K8sNodes{},
		Namespace:       namespace,
		Command:         cmd,
	}
}

// Services implements part of the Discoverer interface and looks at the last
// cached data from the Command (`kubectl`) and returns services in a format
// that Sidecar can manage.
func (k *K8sAPIDiscoverer) Services() []service.Service {
	k.lock.RLock()
	defer k.lock.RUnlock()

	// Enumerate all the K8s nodes we discovered, and for each one, emit all the
	// services that we separately discovered. This means we will attempt to hit
	// the NodePort for each of the nodes when looking for this service.
	var services []service.Service
	for _, node := range k.discoveredNodes.Items {
		hostname, ip := getIPHostForNode(&node)

		for _, item := range k.discoveredSvcs.Items {
			// We require an annotation called 'ServiceName' to make sure this is
			// a service we want to announce.
			if item.Metadata.Labels.ServiceName == "" {
				continue
			}

			svc := service.Service{
				ID:        item.Metadata.UID,
				Name:      item.Metadata.Labels.ServiceName,
				Image:     item.Metadata.Labels.ServiceName + ":kubernetes-hosted",
				Created:   item.Metadata.CreationTimestamp,
				Hostname:  hostname,
				ProxyMode: "http",
				Status:    service.ALIVE,
				Updated:   time.Now().UTC(),
			}

			for _, port := range item.Spec.Ports {
				// We only support entries with NodePort defined
				if port.NodePort < 1 {
					continue
				}
				svc.Ports = append(svc.Ports, service.Port{
					Type:        "tcp",
					Port:        int64(port.NodePort),
					ServicePort: int64(port.Port),
					IP:          ip,
				})
			}
			services = append(services, svc)
		}
	}

	return services
}

func getIPHostForNode(node *K8sNode) (hostname string, ip string) {
	for _, address := range node.Status.Addresses {
		if address.Type == "InternalIP" {
			ip = address.Address
		}

		if address.Type == "Hostname" {
			hostname = address.Address
		}
	}

	return hostname, ip
}

// HealthCheck implements part of the Discoverer interface and returns the
// built-in AlwaysSuccessful check, on the assumption that the underlying load
// balancer we are pointing to will have already health checked the service.
func (k *K8sAPIDiscoverer) HealthCheck(svc *service.Service) (string, string) {
	return "AlwaysSuccessful", ""
}

// Listeners implements part of the Discoverer interface and always returns
// an empty list because it doesn't make sense in this context.
func (k *K8sAPIDiscoverer) Listeners() []ChangeListener {
	return []ChangeListener{}
}

// Run is part of the Discoverer interface and calls the Command in a loop,
// which is injected as a Looper.
func (k *K8sAPIDiscoverer) Run(looper director.Looper) {
	looper.Loop(func() error {
		data, err := k.getServices()
		if err != nil {
			log.Errorf("Failed to unmarshal services json: %s, %s", err, string(data))
		}

		data, err = k.getNodes()
		if err != nil {
			log.Errorf("Failed to unmarshal nodes json: %s, %s", err, string(data))
		}

		return nil
	})
}

func (k *K8sAPIDiscoverer) getServices() ([]byte, error) {
	data, err := k.Command.GetServices()
	if err != nil {
		log.Errorf("Failed to invoke K8s API discovery: %s", err)
	}

	k.lock.Lock()
	err = json.Unmarshal(data, &k.discoveredSvcs)
	k.lock.Unlock()
	return data, err
}

func (k *K8sAPIDiscoverer) getNodes() ([]byte, error) {
	data, err := k.Command.GetNodes()
	if err != nil {
		log.Errorf("Failed to invoke K8s API discovery: %s", err)
	}

	k.lock.Lock()
	err = json.Unmarshal(data, &k.discoveredNodes)
	k.lock.Unlock()
	return data, err
}
