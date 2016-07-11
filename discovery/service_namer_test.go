package discovery

import (
	"testing"

	"github.com/fsouza/go-dockerclient"
	. "github.com/smartystreets/goconvey/convey"
)

func Test_RegexpNamer(t *testing.T) {
	Convey("RegexpNamer", t, func() {
		container := &docker.APIContainers{
			ID: "deadbeef001",
			Image: "gonitro/awesome-svc:0.1.34",
			Names: []string { "/awesome-svc-1231b1b12323" },
			Labels: map[string]string{},
		}

		var namer ServiceNamer

		Convey("Extracts a ServiceName", func() {
			namer = &RegexpNamer{ ServiceNameMatch: "^/(.+)(-[0-9a-z]{7,14})$" }
			So(namer.ServiceName(container), ShouldEqual, "awesome-svc")
		})

		Convey("Returns the image when the expression doesn't match", func() {
			namer = &RegexpNamer{ ServiceNameMatch: "ASDF" }
			So(namer.ServiceName(container), ShouldEqual, "gonitro/awesome-svc:0.1.34")
		})

		Convey("Handles error when passed a nil service", func() {
			namer = &RegexpNamer{}
			So(namer.ServiceName(nil), ShouldEqual, "")
		})
	})
}

func Test_DockerLabelNamer(t *testing.T) {
	Convey("DockerLabelNamer", t, func() {
		container := &docker.APIContainers{
			ID: "deadbeef001",
			Image: "gonitro/awesome-svc:0.1.34",
			Names: []string { "/awesome-svc-1231b1b12323" },
			Labels: map[string]string{ "ServiceName": "awesome-svc-1" },
		}

		var namer ServiceNamer

		Convey("Extracts a ServiceName", func() {
			namer = &DockerLabelNamer{ Label: "ServiceName" }
			So(namer.ServiceName(container), ShouldEqual, "awesome-svc-1")
		})

		Convey("Returns the image when the expression doesn't match", func() {
			namer = &DockerLabelNamer{ Label: "ASDF" }
			So(namer.ServiceName(container), ShouldEqual, "gonitro/awesome-svc:0.1.34")
		})

		Convey("Handles error when passed a nil service", func() {
			namer = &DockerLabelNamer{}
			So(namer.ServiceName(nil), ShouldEqual, "")
		})
	})
}
