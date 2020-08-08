package qpack

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("StaticTable", func() {
	It("verifies that encoderMap has a value for every staticTableEntries entry", func() {
		for idx, hf := range staticTableEntries {
			if len(hf.Value) == 0 {
				Expect(encoderMap[hf.Name].idx).To(Equal(uint8(idx)))
			} else {
				Expect(encoderMap[hf.Name].values[hf.Value]).To(Equal(uint8(idx)))
			}
		}
	})

	It("verifies that staticTableEntries has a value for every encoderMap entry", func() {
		for name, indexAndVal := range encoderMap {
			if len(indexAndVal.values) == 0 {
				id := indexAndVal.idx
				Expect(staticTableEntries[id].Name).To(Equal(name))
				Expect(staticTableEntries[id].Value).To(BeEmpty())
			} else {
				for value, id := range indexAndVal.values {
					Expect(staticTableEntries[id].Name).To(Equal(name))
					Expect(staticTableEntries[id].Value).To(Equal(value))
				}
			}
		}
	})
})
