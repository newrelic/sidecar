package discovery

import (
	"bytes"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/NinesStack/sidecar/service"
	"github.com/relistan/go-director"
	log "github.com/sirupsen/logrus"
	. "github.com/smartystreets/goconvey/convey"
)

var credsPath string = "fixtures"

type mockK8sDiscoveryCommand struct {
	GetServicesShouldError      bool
	GetServicesShouldReturnJunk bool
	GetServicesWasCalled        bool

	GetNodesShouldError      bool
	GetNodesShouldReturnJunk bool
	GetNodesWasCalled        bool
}

func (m *mockK8sDiscoveryCommand) GetServices() ([]byte, error) {
	m.GetServicesWasCalled = true

	if m.GetServicesShouldError {
		return nil, errors.New("intentional test error")
	}

	if m.GetServicesShouldReturnJunk {
		return []byte(`asdfasdf`), nil
	}

	jsonStr := `
	{
	   "items" : [
	      {
	         "metadata" : {
	            "creationTimestamp" : "2022-11-07T13:18:03Z",
	            "labels" : {
	               "Environment" : "dev",
	               "ServiceName" : "chopper"
	            },
	            "name" : "chopper",
	            "uid" : "107b5bbf-9640-4fd0-b5de-1e898e8ae9f7"
	         },
	         "spec" : {
	            "ports" : [
	               {
	                  "port" : 10007,
	                  "protocol" : "TCP",
	                  "targetPort" : 8088,
					  "nodePort": 38088
	               },
	               {
	                  "port" : 10008,
	                  "protocol" : "TCP",
	                  "targetPort" : 8089
	               }
	            ]
	         }
	      }
	   ]
	}

	`
	return []byte(jsonStr), nil
}

func (m *mockK8sDiscoveryCommand) GetNodes() ([]byte, error) {
	m.GetNodesWasCalled = true

	if m.GetNodesShouldError {
		return nil, errors.New("intentional test error")
	}

	if m.GetNodesShouldReturnJunk {
		return []byte(`asdfasdf`), nil
	}

	jsonStr := `
		{
		   "items" : [
		      {
		         "status" : {
		            "addresses" : [
		               {
		                  "address" : "10.100.69.136",
		                  "type" : "InternalIP"
		               },
		               {
		                  "address" : "beowulf.example.com",
		                  "type" : "Hostname"
		               },
		               {
		                  "address" : "beowulf.example.com",
		                  "type" : "InternalDNS"
		               }
		            ]
		         }
		      }
		   ]
		}
	`
	return []byte(jsonStr), nil
}

func Test_NewK8sAPIDiscoverer(t *testing.T) {
	Convey("NewK8sAPIDiscoverer()", t, func() {
		Convey("returns a properly configured K8sAPIDiscoverer", func() {
			disco := NewK8sAPIDiscoverer("127.0.0.1", 443, "heorot", 3*time.Second, credsPath)

			So(disco.discoveredSvcs, ShouldNotBeNil)
			So(disco.Namespace, ShouldEqual, "heorot")
			So(disco.Command, ShouldNotBeNil)

			command := disco.Command.(*KubeAPIDiscoveryCommand)
			So(command.KubeHost, ShouldEqual, "127.0.0.1")
			So(command.KubePort, ShouldEqual, 443)
		})
	})
}

func Test_K8sHealthCheck(t *testing.T) {
	Convey("HealthCheck() always returns 'AlwaysSuccessful'", t, func() {
		disco := NewK8sAPIDiscoverer("127.0.0.1", 443, "heorot", 3*time.Second, credsPath)
		check, args := disco.HealthCheck(nil)
		So(check, ShouldEqual, "AlwaysSuccessful")
		So(args, ShouldBeEmpty)
	})
}

func Test_K8sListeners(t *testing.T) {
	Convey("Listeners() always returns and empty slice", t, func() {
		disco := NewK8sAPIDiscoverer("127.0.0.1", 443, "heorot", 3*time.Second, credsPath)
		listeners := disco.Listeners()
		So(listeners, ShouldBeEmpty)
	})
}

func Test_K8sGetServices(t *testing.T) {
	Convey("GetServices()", t, func() {
		disco := NewK8sAPIDiscoverer("127.0.0.1", 443, "heorot", 3*time.Second, credsPath)
		mock := &mockK8sDiscoveryCommand{}
		disco.Command = mock

		capture := &bytes.Buffer{}

		Convey("calls the command and unmarshals the result", func() {
			log.SetOutput(capture)
			disco.Run(director.NewFreeLooper(director.ONCE, nil))
			log.SetOutput(os.Stdout)

			So(mock.GetServicesWasCalled, ShouldBeTrue)
			So(capture.String(), ShouldNotContainSubstring, "error")
			So(disco.discoveredSvcs, ShouldNotBeNil)
			So(disco.discoveredSvcs, ShouldNotEqual, &K8sServices{})
			So(len(disco.discoveredSvcs.Items), ShouldEqual, 1)
			So(len(disco.discoveredSvcs.Items[0].Spec.Ports), ShouldEqual, 2)
		})

		Convey("call the command and logs errors", func() {
			mock.GetServicesShouldError = true
			log.SetOutput(capture)
			disco.Run(director.NewFreeLooper(director.ONCE, nil))
			log.SetOutput(os.Stdout)

			So(mock.GetServicesWasCalled, ShouldBeTrue)
			So(capture.String(), ShouldContainSubstring, "Failed to invoke")
		})

		Convey("call the command and logs errors from the JSON output", func() {
			mock.GetServicesShouldReturnJunk = true
			log.SetOutput(capture)
			disco.Run(director.NewFreeLooper(director.ONCE, nil))
			log.SetOutput(os.Stdout)

			So(mock.GetServicesWasCalled, ShouldBeTrue)
			So(capture.String(), ShouldContainSubstring, "Failed to unmarshal services json")
		})
	})
}

func Test_K8sGetNodes(t *testing.T) {
	Convey("GetNodes()", t, func() {
		disco := NewK8sAPIDiscoverer("127.0.0.1", 443, "heorot", 3*time.Second, credsPath)
		mock := &mockK8sDiscoveryCommand{}
		disco.Command = mock

		capture := &bytes.Buffer{}

		Convey("calls the command and unmarshals the result", func() {
			log.SetOutput(capture)
			disco.Run(director.NewFreeLooper(director.ONCE, nil))
			log.SetOutput(os.Stdout)

			So(mock.GetNodesWasCalled, ShouldBeTrue)
			So(capture.String(), ShouldNotContainSubstring, "error")
			So(disco.discoveredNodes, ShouldNotBeNil)
			So(disco.discoveredNodes, ShouldNotEqual, &K8sNodes{})
			So(len(disco.discoveredNodes.Items), ShouldEqual, 1)
			So(len(disco.discoveredNodes.Items[0].Status.Addresses), ShouldEqual, 3)
		})

		Convey("call the command and logs errors", func() {
			mock.GetNodesShouldError = true
			log.SetOutput(capture)
			disco.Run(director.NewFreeLooper(director.ONCE, nil))
			log.SetOutput(os.Stdout)

			So(mock.GetNodesWasCalled, ShouldBeTrue)
			So(capture.String(), ShouldContainSubstring, "Failed to invoke")
		})

		Convey("call the command and logs errors from the JSON output", func() {
			mock.GetNodesShouldReturnJunk = true
			log.SetOutput(capture)
			disco.Run(director.NewFreeLooper(director.ONCE, nil))
			log.SetOutput(os.Stdout)

			So(mock.GetNodesWasCalled, ShouldBeTrue)
			So(capture.String(), ShouldContainSubstring, "Failed to unmarshal nodes json")
		})
	})
}

func Test_K8sServices(t *testing.T) {
	Convey("Services()", t, func() {
		disco := NewK8sAPIDiscoverer("127.0.0.1", 443, "heorot", 3*time.Second, credsPath)
		mock := &mockK8sDiscoveryCommand{}
		disco.Command = mock

		Convey("works on a newly-created Discoverer", func() {
			services := disco.Services()
			So(len(services), ShouldEqual, 0)
		})

		Convey("returns the list of cached services", func() {
			disco.Run(director.NewFreeLooper(director.ONCE, nil))
			services := disco.Services()

			So(len(services), ShouldEqual, 1)
			svc := services[0]
			So(svc.ID, ShouldEqual, "107b5bbf-9640-4fd0-b5de-1e898e8ae9f7")
			So(svc.Name, ShouldEqual, "chopper")
			So(svc.Image, ShouldEqual, "chopper:kubernetes-hosted")
			So(svc.Created.String(), ShouldEqual, "2022-11-07 13:18:03 +0000 UTC")
			So(svc.Hostname, ShouldEqual, "beowulf.example.com")
			So(svc.ProxyMode, ShouldEqual, "http")
			So(svc.Status, ShouldEqual, service.ALIVE)
			So(svc.Updated.Unix(), ShouldBeGreaterThan, time.Now().UTC().Add(-2*time.Second).Unix())
			So(len(svc.Ports), ShouldEqual, 1)
			So(svc.Ports[0].IP, ShouldEqual, "10.100.69.136")
		})
	})
}
