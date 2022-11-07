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

type mockK8sDiscoveryCommand struct {
	RunShouldError      bool
	RunShouldReturnJunk bool
	WasCalled           bool
}

func (m *mockK8sDiscoveryCommand) Run() ([]byte, error) {
	m.WasCalled = true

	if m.RunShouldError {
		return nil, errors.New("intentional test error")
	}

	if m.RunShouldReturnJunk {
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
	                  "targetPort" : 8088
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
			disco := NewK8sAPIDiscoverer("127.0.0.1", "beowulf.example.com", "heorot", "/usr/local/somewhere")

			So(disco.discovered, ShouldNotBeNil)
			So(disco.ClusterIP, ShouldEqual, "127.0.0.1")
			So(disco.ClusterHostname, ShouldEqual, "beowulf.example.com")
			So(disco.Namespace, ShouldEqual, "heorot")
			So(disco.Command, ShouldResemble, &KubectlDiscoveryCommand{
				Path: "/usr/local/somewhere", Namespace: "heorot",
			})
		})
	})
}

func Test_K8sHealthCheck(t *testing.T) {
	Convey("HealthCheck() always returns 'AlwaysSuccessful'", t, func() {
		disco := NewK8sAPIDiscoverer("127.0.0.1", "beowulf.example.com", "heorot", "/usr/local/somewhere")
		check, args := disco.HealthCheck(nil)
		So(check, ShouldEqual, "AlwaysSuccessful")
		So(args, ShouldBeEmpty)
	})
}

func Test_K8sListeners(t *testing.T) {
	Convey("Listeners() always returns and empty slice", t, func() {
		disco := NewK8sAPIDiscoverer("127.0.0.1", "beowulf.example.com", "heorot", "/usr/local/somewhere")
		listeners := disco.Listeners()
		So(listeners, ShouldBeEmpty)
	})
}

func Test_K8sRun(t *testing.T) {
	Convey("Run()", t, func() {
		disco := NewK8sAPIDiscoverer("127.0.0.1", "beowulf.example.com", "heorot", "/usr/local/somewhere")
		mock := &mockK8sDiscoveryCommand{}
		disco.Command = mock

		capture := &bytes.Buffer{}

		Convey("calls the command and unmarshals the result", func() {
			log.SetOutput(capture)
			disco.Run(director.NewFreeLooper(director.ONCE, nil))
			log.SetOutput(os.Stdout)

			So(mock.WasCalled, ShouldBeTrue)
			So(capture.String(), ShouldNotContainSubstring, "error")
			So(disco.discovered, ShouldNotBeNil)
			So(disco.discovered, ShouldNotEqual, &K8sServices{})
			So(len(disco.discovered.Items), ShouldEqual, 1)
			So(len(disco.discovered.Items[0].Spec.Ports), ShouldEqual, 1)
		})

		Convey("call the command and logs errors", func() {
			mock.RunShouldError = true
			log.SetOutput(capture)
			disco.Run(director.NewFreeLooper(director.ONCE, nil))
			log.SetOutput(os.Stdout)

			So(mock.WasCalled, ShouldBeTrue)
			So(capture.String(), ShouldContainSubstring, "Failed to invoke")
		})

		Convey("call the command and logs errors from the JSON output", func() {
			mock.RunShouldReturnJunk = true
			log.SetOutput(capture)
			disco.Run(director.NewFreeLooper(director.ONCE, nil))
			log.SetOutput(os.Stdout)

			So(mock.WasCalled, ShouldBeTrue)
			So(capture.String(), ShouldContainSubstring, "Failed to unmarshal json")
		})
	})
}

func Test_K8sServices(t *testing.T) {
	Convey("Services()", t, func() {
		disco := NewK8sAPIDiscoverer("127.0.0.1", "beowulf.example.com", "heorot", "/usr/local/somewhere")
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
		})
	})
}
