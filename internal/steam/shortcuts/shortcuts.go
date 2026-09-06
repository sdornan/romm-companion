// Package shortcuts maps RomM games onto entries in Steam's shortcuts.vdf.
//
// Entries owned by this program carry a "romm:<rom_id>" tag. Everything else
// in the file is preserved untouched, including entries added by hand or by
// other tools.
package shortcuts

import (
	"fmt"
	"hash/crc32"
	"io"
	"strconv"
	"strings"

	"github.com/sdornan/romm-companion/internal/steam/vdf"
)

// OwnerTag marks a shortcut as created by RomM Companion. Steam shows tags as
// collections, so every RomM game also lands in a "RomM" collection.
const OwnerTag = "RomM"

const romTagPrefix = "romm:"

// Shortcut is the subset of a shortcuts.vdf entry this program manages.
type Shortcut struct {
	RomID         int
	AppName       string
	Exe           string // absolute path, unquoted; quoted on write
	StartDir      string // absolute directory, unquoted; quoted on write
	LaunchOptions string
	Icon          string
	// AppID is derived from Exe and AppName. It is filled on read and on Upsert.
	AppID uint32
}

// AppID computes the 32-bit non-Steam app id Steam derives for a shortcut:
// CRC32 of the quoted exe followed by the app name, with the high bit set.
// Artwork in userdata/<id>/config/grid/ is named by this value.
func AppID(exe, appName string) uint32 {
	return crc32.ChecksumIEEE([]byte(quote(exe)+appName)) | 0x80000000
}

// LegacyAppID is the 64-bit form older Steam builds used in steam://rungameid/
// URLs and some artwork names.
func LegacyAppID(exe, appName string) uint64 {
	return uint64(AppID(exe, appName))<<32 | 0x02000000
}

// File is a decoded shortcuts.vdf.
type File struct {
	root *vdf.Node
}

// Read decodes a shortcuts.vdf. An empty reader yields an empty file, which is
// what a fresh Steam install has.
func Read(r io.Reader) (*File, error) {
	root, err := vdf.Decode(r)
	if err != nil {
		if err == io.EOF || strings.Contains(err.Error(), "EOF") {
			return New(), nil
		}
		return nil, err
	}
	if !strings.EqualFold(root.Key, "shortcuts") {
		return nil, fmt.Errorf("shortcuts: root key is %q, want shortcuts", root.Key)
	}
	return &File{root: root}, nil
}

// New returns an empty shortcuts file.
func New() *File {
	return &File{root: &vdf.Node{Key: "shortcuts", Type: vdf.TypeObject}}
}

// Write encodes the file.
func (f *File) Write(w io.Writer) error {
	f.renumber()
	return vdf.Encode(w, f.root)
}

// Owned returns every entry created by RomM Companion, in file order.
func (f *File) Owned() []Shortcut {
	var out []Shortcut
	for _, e := range f.root.Children {
		if s, ok := fromNode(e); ok {
			out = append(out, s)
		}
	}
	return out
}

// Len is the total number of entries, owned or not.
func (f *File) Len() int { return len(f.root.Children) }

// Upsert adds or replaces the entry for s.RomID and returns its app id.
func (f *File) Upsert(s Shortcut) uint32 {
	s.AppID = AppID(s.Exe, s.AppName)
	if e := f.find(s.RomID); e != nil {
		toNode(e, s)
		return s.AppID
	}
	e := &vdf.Node{Key: strconv.Itoa(len(f.root.Children)), Type: vdf.TypeObject}
	toNode(e, s)
	f.root.Children = append(f.root.Children, e)
	return s.AppID
}

// Remove deletes the entry for romID. It reports whether one existed.
func (f *File) Remove(romID int) bool {
	for i, e := range f.root.Children {
		if id, ok := romIDOf(e); ok && id == romID {
			f.root.Children = append(f.root.Children[:i], f.root.Children[i+1:]...)
			return true
		}
	}
	return false
}

func (f *File) find(romID int) *vdf.Node {
	for _, e := range f.root.Children {
		if id, ok := romIDOf(e); ok && id == romID {
			return e
		}
	}
	return nil
}

// renumber keeps entry keys as a dense "0".."n-1" sequence, which Steam
// expects after removals.
func (f *File) renumber() {
	for i, e := range f.root.Children {
		e.Key = strconv.Itoa(i)
	}
}

func romIDOf(e *vdf.Node) (int, bool) {
	tags := e.Child("tags")
	if tags == nil {
		return 0, false
	}
	for _, t := range tags.Children {
		if strings.HasPrefix(t.Str, romTagPrefix) {
			id, err := strconv.Atoi(strings.TrimPrefix(t.Str, romTagPrefix))
			if err == nil {
				return id, true
			}
		}
	}
	return 0, false
}

func fromNode(e *vdf.Node) (Shortcut, bool) {
	id, ok := romIDOf(e)
	if !ok {
		return Shortcut{}, false
	}
	s := Shortcut{
		RomID:         id,
		AppName:       str(e, "AppName"),
		Exe:           unquote(str(e, "Exe")),
		StartDir:      unquote(str(e, "StartDir")),
		LaunchOptions: str(e, "LaunchOptions"),
		Icon:          str(e, "icon"),
	}
	if n := e.Child("appid"); n != nil {
		s.AppID = uint32(n.Int)
	}
	return s, true
}

func toNode(e *vdf.Node, s Shortcut) {
	e.SetInt("appid", int32(s.AppID))
	e.SetString("AppName", s.AppName)
	e.SetString("Exe", quote(s.Exe))
	e.SetString("StartDir", quote(s.StartDir))
	e.SetString("icon", s.Icon)
	e.SetString("ShortcutPath", "")
	e.SetString("LaunchOptions", s.LaunchOptions)
	e.SetInt("IsHidden", 0)
	e.SetInt("AllowDesktopConfig", 1)
	e.SetInt("AllowOverlay", 1)
	e.SetInt("OpenVR", 0)
	e.SetInt("Devkit", 0)
	e.SetString("DevkitGameID", "")
	e.SetInt("DevkitOverrideAppID", 0)
	if e.Child("LastPlayTime") == nil {
		e.SetInt("LastPlayTime", 0)
	}
	e.SetString("FlatpakAppID", "")
	tags := e.SetObject("tags")
	tags.Children = nil
	tags.SetString("0", OwnerTag)
	tags.SetString("1", romTagPrefix+strconv.Itoa(s.RomID))
}

func str(e *vdf.Node, key string) string {
	if n := e.Child(key); n != nil {
		return n.Str
	}
	return ""
}

func quote(s string) string {
	if s == "" {
		return ""
	}
	return `"` + s + `"`
}

func unquote(s string) string {
	return strings.TrimSuffix(strings.TrimPrefix(s, `"`), `"`)
}
