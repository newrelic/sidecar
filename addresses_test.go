package main

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func Test_Addresses(t *testing.T) {
	Convey("setupIPBlocks()", t, func() {
		Convey("Sets up the right number of blocks", func() {
			setupIPBlocks()
			So(len(privateBlocks), ShouldEqual, 3)
		})
	})

	Convey("isPrivateIP()", t, func() {
		Convey("Can tell whether an address is private", func() {
			So(isPrivateIP("172.16.54.3"), ShouldBeTrue)
			So(isPrivateIP("12.1.1.1"), ShouldBeFalse)
		})
	})

	Convey("findPrivateAddresses()", t, func() {
		// Note that this actually looks at interfaces on the machine.
		// Almost all machines will pass this test. It's possible it might
		// fail on a machine that has only a public Internet IP.
		Convey("Finds at least one private address", func() {
			addresses, err := findPrivateAddresses()
			So(err, ShouldBeNil)
			So(len(addresses), ShouldBeGreaterThan, 0)
		})
	})

	Convey("getPublishedIP()", t, func() {
		ip := "10.10.10.10"

		Convey("Returns the advertised IP if supplied", func() {
			result, err := getPublishedIP([]string{}, ip)
			So(err, ShouldBeNil)
			So(result, ShouldEqual, ip)
		})

		// See caveat for findPrivateAddresses() above
		Convey("Returns an address", func() {
			addresses, _ := findPrivateAddresses()
			result, err := getPublishedIP([]string{}, "")

			So(err, ShouldBeNil)
			So(result, ShouldResemble, addresses[0].String())
		})
	})
}
