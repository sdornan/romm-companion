// Package vdf reads and writes Valve's binary KeyValues format, the encoding
// Steam uses for userdata/<id>/config/shortcuts.vdf.
//
// The tree is kept generic and ordered so a file can be decoded, edited in
// one place, and re-encoded without disturbing entries this program did not
// create.
package vdf

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Type tags as they appear on disk.
const (
	TypeObject byte = 0x00
	TypeString byte = 0x01
	TypeInt32  byte = 0x02
	TypeEnd    byte = 0x08
	// TypeEndAlt is written by newer Steam clients in place of TypeEnd.
	TypeEndAlt byte = 0x0B
)

// Node is one key in a binary KeyValues tree. Exactly one of Str, Int or
// Children is meaningful, selected by Type.
type Node struct {
	Key      string
	Type     byte
	Str      string
	Int      int32
	Children []*Node
}

// Child returns the first direct child with the given key, matching
// case-insensitively because Steam has changed key casing between versions.
func (n *Node) Child(key string) *Node {
	for _, c := range n.Children {
		if equalFold(c.Key, key) {
			return c
		}
	}
	return nil
}

// SetString sets or appends a string child.
func (n *Node) SetString(key, value string) {
	if c := n.Child(key); c != nil {
		c.Type, c.Str, c.Int, c.Children = TypeString, value, 0, nil
		return
	}
	n.Children = append(n.Children, &Node{Key: key, Type: TypeString, Str: value})
}

// SetInt sets or appends an int32 child.
func (n *Node) SetInt(key string, value int32) {
	if c := n.Child(key); c != nil {
		c.Type, c.Str, c.Int, c.Children = TypeInt32, "", value, nil
		return
	}
	n.Children = append(n.Children, &Node{Key: key, Type: TypeInt32, Int: value})
}

// SetObject sets or appends an object child and returns it.
func (n *Node) SetObject(key string) *Node {
	if c := n.Child(key); c != nil {
		c.Type, c.Str, c.Int = TypeObject, "", 0
		return c
	}
	c := &Node{Key: key, Type: TypeObject}
	n.Children = append(n.Children, c)
	return c
}

// Decode parses a complete binary KeyValues document. The result is the
// single root object (for shortcuts.vdf its key is "shortcuts").
func Decode(r io.Reader) (*Node, error) {
	br := bufio.NewReader(r)
	t, err := br.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("vdf: read root type: %w", err)
	}
	if t != TypeObject {
		return nil, fmt.Errorf("vdf: root must be an object, got type 0x%02x", t)
	}
	key, err := readCString(br)
	if err != nil {
		return nil, err
	}
	root := &Node{Key: key, Type: TypeObject}
	if err := decodeChildren(br, root, 0); err != nil {
		return nil, err
	}
	return root, nil
}

// maxDepth guards against a corrupt file recursing forever.
const maxDepth = 64

func decodeChildren(br *bufio.Reader, parent *Node, depth int) error {
	if depth > maxDepth {
		return errors.New("vdf: nesting too deep")
	}
	for {
		t, err := br.ReadByte()
		if err != nil {
			return fmt.Errorf("vdf: unexpected end of data in %q: %w", parent.Key, err)
		}
		if t == TypeEnd || t == TypeEndAlt {
			return nil
		}
		key, err := readCString(br)
		if err != nil {
			return err
		}
		n := &Node{Key: key, Type: t}
		switch t {
		case TypeObject:
			if err := decodeChildren(br, n, depth+1); err != nil {
				return err
			}
		case TypeString:
			if n.Str, err = readCString(br); err != nil {
				return err
			}
		case TypeInt32:
			var buf [4]byte
			if _, err := io.ReadFull(br, buf[:]); err != nil {
				return fmt.Errorf("vdf: read int32 %q: %w", key, err)
			}
			n.Int = int32(binary.LittleEndian.Uint32(buf[:]))
		default:
			return fmt.Errorf("vdf: unsupported type 0x%02x for key %q", t, key)
		}
		parent.Children = append(parent.Children, n)
	}
}

// Encode writes root as a complete binary KeyValues document.
func Encode(w io.Writer, root *Node) error {
	var buf bytes.Buffer
	if err := encodeNode(&buf, root); err != nil {
		return err
	}
	// Steam terminates the file with a second end marker after the root.
	buf.WriteByte(TypeEnd)
	_, err := w.Write(buf.Bytes())
	return err
}

func encodeNode(buf *bytes.Buffer, n *Node) error {
	buf.WriteByte(n.Type)
	writeCString(buf, n.Key)
	switch n.Type {
	case TypeObject:
		for _, c := range n.Children {
			if err := encodeNode(buf, c); err != nil {
				return err
			}
		}
		buf.WriteByte(TypeEnd)
	case TypeString:
		writeCString(buf, n.Str)
	case TypeInt32:
		var b [4]byte
		binary.LittleEndian.PutUint32(b[:], uint32(n.Int))
		buf.Write(b[:])
	default:
		return fmt.Errorf("vdf: cannot encode type 0x%02x for key %q", n.Type, n.Key)
	}
	return nil
}

func readCString(br *bufio.Reader) (string, error) {
	s, err := br.ReadString(0)
	if err != nil {
		return "", fmt.Errorf("vdf: read string: %w", err)
	}
	return s[:len(s)-1], nil
}

func writeCString(buf *bytes.Buffer, s string) {
	buf.WriteString(s)
	buf.WriteByte(0)
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
