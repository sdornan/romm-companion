package vdf

import (
	"bytes"
	"testing"
)

// sample is a hand-built shortcuts.vdf with one entry, laid out the way Steam
// writes it: root object "shortcuts", child object "0", typed fields, a tags
// object, and the trailing end marker.
func sample() []byte {
	var b bytes.Buffer
	w := func(parts ...any) {
		for _, p := range parts {
			switch v := p.(type) {
			case byte:
				b.WriteByte(v)
			case string:
				b.WriteString(v)
				b.WriteByte(0)
			case []byte:
				b.Write(v)
			}
		}
	}
	w(TypeObject, "shortcuts")
	w(TypeObject, "0")
	w(TypeInt32, "appid", []byte{0x39, 0x30, 0x00, 0x80}) // 0x80003039
	w(TypeString, "AppName", "Chrono Trigger")
	w(TypeString, "Exe", `"/usr/bin/romm-companion"`)
	w(TypeString, "StartDir", `"/usr/bin/"`)
	w(TypeString, "LaunchOptions", "launch --rom 123")
	w(TypeInt32, "IsHidden", []byte{0, 0, 0, 0})
	w(TypeObject, "tags")
	w(TypeString, "0", "RomM")
	w(TypeString, "1", "romm:123")
	w(TypeEnd) // tags
	w(TypeEnd) // 0
	w(TypeEnd) // shortcuts
	w(TypeEnd) // file terminator
	return b.Bytes()
}

func TestDecodeReadsTypedFields(t *testing.T) {
	root, err := Decode(bytes.NewReader(sample()))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if root.Key != "shortcuts" {
		t.Fatalf("root key = %q, want shortcuts", root.Key)
	}
	entry := root.Child("0")
	if entry == nil {
		t.Fatal("missing entry 0")
	}
	if got := entry.Child("appid").Int; uint32(got) != 0x80003039 {
		t.Errorf("appid = 0x%08x, want 0x80003039", uint32(got))
	}
	if got := entry.Child("appname").Str; got != "Chrono Trigger" {
		t.Errorf("AppName (case-insensitive lookup) = %q", got)
	}
	tags := entry.Child("tags")
	if tags == nil || len(tags.Children) != 2 || tags.Children[1].Str != "romm:123" {
		t.Errorf("tags not decoded: %+v", tags)
	}
}

func TestEncodeRoundTripsBytes(t *testing.T) {
	in := sample()
	root, err := Decode(bytes.NewReader(in))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	var out bytes.Buffer
	if err := Encode(&out, root); err != nil {
		t.Fatalf("encode: %v", err)
	}
	if !bytes.Equal(in, out.Bytes()) {
		t.Fatalf("round trip changed bytes\n in: % x\nout: % x", in, out.Bytes())
	}
}

func TestDecodeAcceptsAltEndMarker(t *testing.T) {
	in := sample()
	for i := range in {
		if in[i] == TypeEnd {
			in[i] = TypeEndAlt
		}
	}
	if _, err := Decode(bytes.NewReader(in)); err != nil {
		t.Fatalf("decode with 0x0B end markers: %v", err)
	}
}

func TestDecodeRejectsTruncatedFile(t *testing.T) {
	in := sample()
	if _, err := Decode(bytes.NewReader(in[:len(in)/2])); err == nil {
		t.Fatal("expected error for truncated input")
	}
}

func TestSettersReplaceInPlace(t *testing.T) {
	n := &Node{Key: "x", Type: TypeObject}
	n.SetString("AppName", "a")
	n.SetString("appname", "b")
	if len(n.Children) != 1 || n.Children[0].Str != "b" {
		t.Fatalf("SetString should replace case-insensitively: %+v", n.Children)
	}
	n.SetInt("IsHidden", 1)
	n.SetObject("tags").SetString("0", "RomM")
	if len(n.Children) != 3 {
		t.Fatalf("want 3 children, got %d", len(n.Children))
	}
}
