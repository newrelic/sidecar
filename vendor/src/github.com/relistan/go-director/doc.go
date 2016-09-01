// Director contains a few common loop flow control patterns that can be
// dependency-injected, generally to simplify running goroutines forever in the
// backround. It has a single interface and a few implementations of the
// interface. The externalization of the loop flow control makes it easy to
// test the internal functions of background goroutines by, for instance,
// only running the loop once while under test.
//
// The design is that any errors which need to be returned from the loop
// will be passed back on a DoneChan whose implementation is left up to
// the individual Looper.
package director
