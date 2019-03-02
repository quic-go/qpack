package qpack_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestQpack(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "QPACK Suite")
}
