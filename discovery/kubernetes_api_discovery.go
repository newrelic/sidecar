package discovery

import (
	"encoding/json"
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
	ClusterIP       string
	ClusterHostname string
	Namespace       string

	Command K8sDiscoveryCommand

	discovered *K8sServices
}

// NewK8sAPIDiscoverer returns a properly configured K8sAPIDiscoverer
func NewK8sAPIDiscoverer(clusterIP, clusterHostname, namespace, path string) *K8sAPIDiscoverer {
	return &K8sAPIDiscoverer{
		discovered:      &K8sServices{},
		ClusterIP:       clusterIP,
		ClusterHostname: clusterHostname,
		Namespace:       namespace,
		Command: &KubectlDiscoveryCommand{
			Path:      path,
			Namespace: namespace,
		},
	}
}

// Services implements part of the Discoverer interface and looks at the last
// cached data from the Command (`kubectl`) and returns services in a format
// that Sidecar can manage.
func (k *K8sAPIDiscoverer) Services() []service.Service {
	var services []service.Service
	for _, item := range k.discovered.Items {
		svc := service.Service{
			ID:        item.Metadata.UID,
			Name:      item.Metadata.Labels.ServiceName,
			Image:     item.Metadata.Labels.ServiceName+":kubernetes-hosted",
			Created:   item.Metadata.CreationTimestamp,
			Hostname:  k.ClusterHostname,
			ProxyMode: "http",
			Status:    service.ALIVE,
			Updated:   time.Now().UTC(),
		}
		for _, port := range item.Spec.Ports {
			svc.Ports = append(svc.Ports, service.Port{
				Type:        "tcp",
				Port:        int64(port.Port),
				ServicePort: int64(port.Port),
				IP:          k.ClusterIP,
			})
		}
		services = append(services, svc)
	}

	return services
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
		data, err := k.Command.Run()
		if err != nil {
			log.Errorf("Failed to invoke K8s API discovery: %s", err)
		}

		err = json.Unmarshal(data, &k.discovered)
		if err != nil {
			log.Errorf("Failed to unmarshal json: %s, %s", err, string(data))
		}
		return nil
	})
}
