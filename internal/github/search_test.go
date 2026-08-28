package github

import (
	"testing"

	"onekey/internal/models"
)

// TestSearchGames checks the免Key result ranking without network by seeding
// loadAppList's cache with a small fixed list.
func TestSearchGames(t *testing.T) {
	appList = []models.StoreSearchItem{
		{Name: "Resident Evil", ID: 304240},
		{Name: "RESIDENT EVIL 3", ID: 952060},
		{Name: "Vampire Survivors", ID: 1794680},
	}
	got := SearchGames("resident", "schinese", 20)
	// Prefix matches (Resident*) must come before substring matches and limit
	// must cap the total.
	if len(got) < 2 {
		t.Fatalf("expected >= 2 matches for 'resident', got %d", len(got))
	}
	if got[0].Name != "Resident Evil" {
		t.Fatalf("expected prefix 'Resident Evil' first, got %q", got[0].Name)
	}
	// Case-insensitive substring hit must be present.
	seen := false
	for _, it := range got {
		if it.ID == 952060 {
			seen = true
		}
	}
	if !seen {
		t.Fatalf("expected case-insensitive hit 'RESIDENT EVIL 3' in results")
	}

	if got = SearchGames("vampir", "schinese", 1); len(got) != 1 {
		t.Fatalf("limit=1 must cap results, got %d", len(got))
	}
	if got = SearchGames("  ", "schinese", 20); len(got) != 0 {
		t.Fatalf("blank term must return no results")
	}
}