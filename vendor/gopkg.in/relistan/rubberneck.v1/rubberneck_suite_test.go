package rubberneck_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestRubberneck(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Rubberneck Suite")
}
