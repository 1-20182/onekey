package github

import "testing"

func TestParseManifestName(t *testing.T) {
	cases := []struct {
		in       string
		depot    string
		manifest string
		ok       bool
	}{
		{"1144201_6091457157396211072.manifest", "1144201", "6091457157396211072", true},
		{"480_1738311098.manifest", "480", "1738311098", true},
		{"foo.manifest", "", "", false},
		{"readme.md", "", "", false},
		{"1_2_3.manifest", "", "", false}, // >2 parts is not a depot manifest
	}
	for _, c := range cases {
		d, m, ok := parseManifestName(c.in)
		if d != c.depot || m != c.manifest || ok != c.ok {
			t.Errorf("parseManifestName(%q) = (%q,%q,%v), want (%q,%q,%v)",
				c.in, d, m, ok, c.depot, c.manifest, c.ok)
		}
	}
}

func TestParseKeyVDF(t *testing.T) {
	content := []byte("\"depots\"\n{\n\t\"1144201\"\n\t{\n\t\t\"DecryptionKey\" \"b9236894ba071d12dd117ce16220fefeb20574d7c6f7d2170e3113346867f3d6\"\n\t}\n}\n")
	keys := parseKeyVDF(content)
	if keys["1144201"] != "b9236894ba071d12dd117ce16220fefeb20574d7c6f7d2170e3113346867f3d6" {
		t.Fatalf("unexpected keys: %v", keys)
	}
}