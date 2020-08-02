package qpack

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("StaticTable", func() {

	It("verifies that encoderMap and staticTableEntries are coherent", func() {
		for idx, hf := range staticTableEntries {
			if len(hf.Value) == 0 {
				Expect(encoderMap[hf.Name].idx).To(Equal(uint8(idx)))
			} else {
				Expect(encoderMap[hf.Name].values[hf.Value]).To(Equal(uint8(idx)))
			}
		}
	})

})
