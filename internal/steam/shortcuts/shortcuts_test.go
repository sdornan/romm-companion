package shortcuts

import (
	"bytes"
	"testing"

	"github.com/sdornan/romm-companion/internal/steam/vdf"
)

func TestAppIDMatchesSteamDerivation(t *testing.T) {
	// Steam hashes the quoted exe string followed by the name, then sets the
	// high bit. The expected value was produced independently with Python:
	// zlib.crc32(b'"/usr/bin/romm-companion"Chrono Trigger') | 0x80000000.
	got := AppID("/usr/bin/romm-companion", "Chrono Trigger")
	if got&0x80000000 == 0 {
		t.Fatalf("high bit not set: %08x", got)
	}
	if got != expectedAppID {
		t.Fatalf("AppID = %08x, want %08x", got, expectedAppID)
	}
	if LegacyAppID("/usr/bin/romm-companion", "Chrono Trigger") != uint64(got)<<32|0x02000000 {
		t.Fatal("LegacyAppID does not wrap AppID")
	}
}

func TestUpsertRemovePreservesForeignEntries(t *testing.T) {
	f := New()
	// A shortcut added by the user, not by us: no romm tag.
	foreign := &vdf.Node{Key: "0", Type: vdf.TypeObject}
	foreign.SetInt("appid", -5)
	foreign.SetString("AppName", "Handmade")
	foreign.SetString("Exe", `"/bin/true"`)
	f.root.Children = append(f.root.Children, foreign)

	id := f.Upsert(Shortcut{RomID: 42, AppName: "Metroid", Exe: "/opt/romm", StartDir: "/opt", LaunchOptions: "launch --rom 42"})
	if id != AppID("/opt/romm", "Metroid") {
		t.Fatal("Upsert returned wrong app id")
	}
	if f.Len() != 2 || len(f.Owned()) != 1 {
		t.Fatalf("len=%d owned=%d", f.Len(), len(f.Owned()))
	}

	// Upsert again with a new name replaces in place rather than appending.
	f.Upsert(Shortcut{RomID: 42, AppName: "Super Metroid", Exe: "/opt/romm", StartDir: "/opt"})
	if f.Len() != 2 || f.Owned()[0].AppName != "Super Metroid" {
		t.Fatalf("second upsert should replace: %+v", f.Owned())
	}

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	back, err := Read(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if back.Len() != 2 || back.Owned()[0].RomID != 42 || back.Owned()[0].Exe != "/opt/romm" {
		t.Fatalf("round trip lost data: %+v", back.Owned())
	}
	if back.root.Children[0].Child("AppName").Str != "Handmade" {
		t.Fatal("foreign entry was modified")
	}

	if !back.Remove(42) || back.Len() != 1 || back.Remove(42) {
		t.Fatal("Remove should delete exactly the owned entry once")
	}
}

func TestReadEmptyFileIsEmpty(t *testing.T) {
	f, err := Read(bytes.NewReader(nil))
	if err != nil || f.Len() != 0 {
		t.Fatalf("empty read: f=%v err=%v", f, err)
	}
}
