// The export_test file gives the black-box tests of package inventory_test their test seams.
// It exposes private fields for fault injection. It does not expose private methods.

package inventory

// =============================================================================
// Test Seams
// =============================================================================

// SetShims sets the shims field of the BaseLister.
func (x *BaseLister) SetShims(v *Shims) {
	x.shims = v
}

// SetExtractors sets the extractors field of the BaseLister.
func (x *BaseLister) SetExtractors(v []Extractor) {
	x.extractors = v
}

// SetChartRenderer sets the chartRenderer field of the BaseLister.
func (x *BaseLister) SetChartRenderer(v ChartRenderer) {
	x.chartRenderer = v
}
