package catalog_test

import (
	"testing"

	"github.com/Sebas1705/WinForge/internal/catalog"
)

func TestIsOpenSource(t *testing.T) {
	yes := []string{"MIT", "Apache-2.0", "GPL-3.0", "GPL-2.0 with Classpath Exception", "MIT OR Apache-2.0",
		"BSD-3-Clause", "MPL-2.0", "LGPL-2.1", "GNU General Public License v3.0", "PostgreSQL", "PSF", "Boost Software License 1.0", "Unlicense"}
	no := []string{"", "Proprietary", "Proprietary (MIT source)", "BUSL-1.1", "Freeware", "Trial", "Proprietary (free for individuals)",
		"Limited", "Permitted use only", "Elastic License 2.0", "SSPL-1.0", "Custom"}
	for _, s := range yes {
		if !catalog.IsOpenSource(s) {
			t.Errorf("%q should be open source", s)
		}
	}
	for _, s := range no {
		if catalog.IsOpenSource(s) {
			t.Errorf("%q must not be open source", s)
		}
	}
}
