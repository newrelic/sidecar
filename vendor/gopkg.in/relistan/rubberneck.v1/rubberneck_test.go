package rubberneck_test

import (
	"fmt"

	. "github.com/relistan/rubberneck"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Rubberneck", func() {
	Describe("NewDefaultPrinter()", func() {
		It("returns a properly configured Printer", func() {
			printer := NewDefaultPrinter()
			Expect(printer.Show).NotTo(BeNil())
		})
	})

	Describe("NewPrinter()", func() {
		var didRun bool
		var didAddLineFeed bool

		var printFunc func(format string, v ...interface{})
		var printable struct{ Content string }
		var receivedFormat string

		BeforeEach(func() {
			didRun = false
			didAddLineFeed = false

			printFunc = func(format string, v ...interface{}) {
				didRun = true
				if format[len(format)-1] == '\n' {
					didAddLineFeed = true
				}
				receivedFormat = format
			}

			printable = struct{ Content string }{"grendel"}
		})

		It("returns a properly configured Printer without line feed", func() {
			printer := NewPrinter(printFunc, NoAddLineFeed)
			Expect(printer.Show).NotTo(BeNil())

			printer.Print(printable)
			Expect(didRun).To(BeTrue())
			Expect(didAddLineFeed).To(BeFalse())
		})

		It("returns a properly configured Printer with line feed", func() {
			printer := NewPrinter(printFunc, AddLineFeed)
			Expect(printer.Show).NotTo(BeNil())

			printer.Print(printable)
			Expect(didRun).To(BeTrue())
			Expect(didAddLineFeed).To(BeTrue())
		})
	})

	Describe("when printing with", func() {
		var printFunc func(format string, v ...interface{})
		var printable struct {
			Content []string
			Another struct{ Included string }
			private bool
		}
		var output string
		var printer *Printer

		BeforeEach(func() {
			output = ""

			printFunc = func(format string, v ...interface{}) {
				output += fmt.Sprintf(format, v...)
			}

			printable = struct {
				Content []string
				Another struct{ Included string }
				private bool
			}{
				[]string{"njal", "groenlendinga"},
				struct{ Included string }{"leif"},
				true,
			}

			printer = NewPrinter(printFunc, AddLineFeed)
		})

		Describe("PrintWithLabel()", func() {
			It("generates correct output", func() {
				printer.PrintWithLabel("saga", printable)
				Expect(output).To(ContainSubstring("saga ----"))
				Expect(output).To(ContainSubstring("Content: [njal groenlendinga]"))
				Expect(output).To(MatchRegexp("\\* Another:\n\\s+\\* Included: leif"))
			})

			It("generates correct output when passed a pointer", func() {
				printer.PrintWithLabel("saga", &printable)
				Expect(output).To(ContainSubstring("saga ----"))
				Expect(output).To(ContainSubstring("Content: [njal groenlendinga]"))
				Expect(output).To(MatchRegexp("\\* Another:\n\\s+\\* Included: leif"))
			})

			It("excludes private struct members", func() {
				printer.PrintWithLabel("saga", &printable)
				Expect(output).NotTo(ContainSubstring("private"))
			})
		})

		Describe("Print()", func() {
			It("complains when passed a string", func() {
				printer.Print("saga", printable)
				Expect(output).To(ContainSubstring("Expected to print a struct"))
				Expect(output).NotTo(ContainSubstring("Content: [njal groenlendinga]"))
				Expect(output).NotTo(MatchRegexp("\\* Another:\n\\s+\\* Included: leif"))
			})
		})
	})

	Describe("handling values", func() {
		var printFunc func(format string, v ...interface{})
		var printable struct {
			Bad *int
		}
		var output string
		var printer *Printer

		BeforeEach(func() {
			output = ""

			printFunc = func(format string, v ...interface{}) {
				output += fmt.Sprintf(format, v...)
			}

			printable = struct {
				Bad *int
			}{}

			printer = NewPrinter(printFunc, AddLineFeed)
		})

		It("handles nil pointers", func() {
			test := func() { printer.Print(printable) }

			Expect(test).NotTo(Panic())
		})
	})
})
