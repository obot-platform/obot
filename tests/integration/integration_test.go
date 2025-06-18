package integration_test

import (
	"testing"

	//revive:disable
	. "github.com/onsi/ginkgo/v2"
	//revive:disable
	. "github.com/onsi/gomega"
)

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Suite")
}
