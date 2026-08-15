package update

import "testing"

// TestCompareVersions_ForkReleaseTagPrefix guards the fork's release tag shape.
//
// This fork tags releases `loom-v<version>` because a fork inherits the whole
// upstream tag namespace, where `v*` is both taken and unsafe. compareVersions
// parses a leading integer out of each dotted segment, so an un-normalized
// `loom-v1.1.0` yields 0 for its first segment and sorts BELOW every real
// version. The update check would then answer "nothing newer" forever - a
// silent failure, which is worse than a wrong answer, because nothing in the
// logs or the UI would ever contradict it.
func TestCompareVersions_ForkReleaseTagPrefix(t *testing.T) {
	cases := []struct {
		name    string
		latest  string
		current string
		want    int
	}{
		{"newer minor is detected through the prefix", "loom-v1.1.0", "1.0.0", 1},
		{"newer patch is detected through the prefix", "loom-v1.0.1", "1.0.0", 1},
		{"newer major is detected through the prefix", "loom-v2.0.0", "1.9.9", 1},
		{"same version is equal despite the prefix", "loom-v1.0.0", "1.0.0", 0},
		{"older version stays older", "loom-v1.0.0", "1.1.0", -1},
		{"plain upstream-style tag still works", "v1.1.0", "1.0.0", 1},
		{"bare version still works", "1.1.0", "1.0.0", 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := compareVersions(c.latest, c.current); got != c.want {
				t.Fatalf("compareVersions(%q, %q) = %d, want %d", c.latest, c.current, got, c.want)
			}
		})
	}
}
