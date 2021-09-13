package adapter

import (
	"testing"

	"github.com/NinesStack/sidecar/service"
	. "github.com/smartystreets/goconvey/convey"
)

func Test_isPortCollision(t *testing.T) {
	Convey("isPortCollision()", t, func() {
		portsMap := map[int64]string{
			int64(10001): "beowulf",
			int64(10002): "grendel",
		}

		Convey("returns true when the port is a different service", func() {
			svc := &service.Service{Name: "hrothgar"}
			port := service.Port{ServicePort: int64(10001)}

			result := isPortCollision(portsMap, svc, port)

			So(result, ShouldBeTrue)
			So(portsMap[int64(10001)], ShouldEqual, "beowulf")
		})

		Convey("returns false when the port is the same service", func() {
			svc := &service.Service{Name: "beowulf"}
			port := service.Port{ServicePort: int64(10001)}

			result := isPortCollision(portsMap, svc, port)

			So(result, ShouldBeFalse)
		})

		Convey("returns false and assigns it when the port is not assigned", func() {
			svc := &service.Service{Name: "hrothgar"}
			port := service.Port{ServicePort: int64(10003)}

			result := isPortCollision(portsMap, svc, port)

			So(result, ShouldBeFalse)
			So(portsMap[int64(10003)], ShouldEqual, "hrothgar")
		})
	})
}
