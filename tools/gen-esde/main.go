// Command gen-esde translates ES-DE's system definitions and emulator find
// rules into the per-OS tables embedded by internal/emulator/esde.
//
// It is a maintainer step, not part of the build: run it when bumping the
// pinned ES-DE release, then commit the regenerated JSON.
//
//	go run ./tools/gen-esde -aliases tools/gen-esde/romm_slugs.json
//
// ES-DE is MIT licensed; the derived tables carry its copyright notice in
// internal/emulator/esde/LICENSE.ES-DE.
package main

import (
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/sdornan/romm-companion/internal/emulator/esde"
)

// esOSes maps ES-DE's resource directory to the GOOS the table is embedded for.
var esOSes = map[string]string{"linux": "linux", "macos": "darwin", "windows": "windows"}

// directives are the leading tokens ES-DE uses to tune how it starts a
// process. Everything but %STARTDIR% is about ES-DE's own window handling and
// is dropped.
var directives = map[string]bool{
	"%RUNINBACKGROUND%": true,
	"%HIDEWINDOW%":      true,
	"%ENABLESHORTCUTS%": true,
	"%ESCAPESPECIALS%":  true,
}

// skipEmulators are entries the companion never launches: RetroArch, because
// RomM's own core map decides which core runs a platform, and the shell, whose
// "Shortcut or script" alternative just executes the file itself.
var skipEmulators = map[string]bool{
	"RETROARCH": true,
	"OS-SHELL":  true,
}

// argPlaceholders are the substitutions the launcher can make. An alternative
// using anything else is dropped rather than launched with a literal %TOKEN%.
var argPlaceholders = map[string]bool{
	"%ROM%":      true,
	"%BASENAME%": true,
	"%GAMEDIR%":  true,
	"%EMUDIR%":   true,
	"%ROMPATH%":  true,
}

type esSystems struct {
	Systems []struct {
		Name     string `xml:"name"`
		Commands []struct {
			Label string `xml:"label,attr"`
			Text  string `xml:",chardata"`
		} `xml:"command"`
	} `xml:"system"`
}

type esFindRules struct {
	Emulators []struct {
		Name  string `xml:"name,attr"`
		Rules []struct {
			Type    string   `xml:"type,attr"`
			Entries []string `xml:"entry"`
		} `xml:"rule"`
	} `xml:"emulator"`
}

// aliasFile is the slug data exported from RomM; see the README for the
// one-liner that produces it.
type aliasFile struct {
	Aliases map[string]string `json:"aliases"`
	Slugs   []string          `json:"slugs"`
}

func main() {
	ref := flag.String("ref", "v3.4.1", "ES-DE git tag to translate")
	aliases := flag.String("aliases", "tools/gen-esde/romm_slugs.json", "RomM slug export")
	out := flag.String("out", "internal/emulator/esde", "output directory")
	flag.Parse()

	if err := run(*ref, *aliases, *out); err != nil {
		fmt.Fprintln(os.Stderr, "gen-esde:", err)
		os.Exit(1)
	}
}

func run(ref, aliasPath, outDir string) error {
	slugs, err := loadAliases(aliasPath)
	if err != nil {
		return err
	}
	for esOS, goos := range esOSes {
		data, skipped, err := build(ref, esOS, slugs)
		if err != nil {
			return fmt.Errorf("%s: %w", esOS, err)
		}
		path := filepath.Join(outDir, "data_"+goos+".json")
		if err := writeJSON(path, data); err != nil {
			return err
		}
		fmt.Printf("%-8s %3d platforms, %3d emulators (%d systems had no usable alternative)\n",
			goos, len(data.Systems), len(data.Emulators), skipped)
	}
	return nil
}

func build(ref, esOS string, slugs *aliasFile) (*esde.Data, int, error) {
	var systems esSystems
	if err := fetchXML(ref, esOS, "es_systems.xml", &systems); err != nil {
		return nil, 0, err
	}
	var rules esFindRules
	if err := fetchXML(ref, esOS, "es_find_rules.xml", &rules); err != nil {
		return nil, 0, err
	}

	known := make(map[string]bool, len(slugs.Slugs))
	for _, s := range slugs.Slugs {
		known[s] = true
	}

	data := &esde.Data{
		Source:    "ES-DE " + ref,
		Systems:   map[string][]esde.Alternative{},
		Emulators: map[string]esde.FindRule{},
	}
	used := map[string]bool{}
	skipped := 0

	for _, sys := range systems.Systems {
		slug := slugs.Aliases[sys.Name]
		if slug == "" {
			slug = sys.Name
		}
		if !known[slug] {
			continue
		}
		var alts []esde.Alternative
		for _, cmd := range sys.Commands {
			alt, ok := parseCommand(cmd.Label, cmd.Text)
			if !ok {
				continue
			}
			alts = append(alts, alt)
			used[alt.Emulator] = true
		}
		if len(alts) == 0 {
			skipped++
			continue
		}
		// Several ES-DE systems fold into one RomM slug (regional variants,
		// for instance), so append rather than replace.
		data.Systems[slug] = append(data.Systems[slug], alts...)
	}

	for _, em := range rules.Emulators {
		if !used[em.Name] {
			continue
		}
		var rule esde.FindRule
		for _, r := range em.Rules {
			switch r.Type {
			case "systempath":
				rule.SystemPath = append(rule.SystemPath, trimAll(r.Entries)...)
			case "staticpath":
				rule.StaticPath = append(rule.StaticPath, trimAll(r.Entries)...)
			}
		}
		if len(rule.SystemPath) > 0 || len(rule.StaticPath) > 0 {
			data.Emulators[em.Name] = rule
		}
	}

	// An alternative whose emulator has no find rule can never resolve.
	for slug, alts := range data.Systems {
		kept := alts[:0]
		for _, a := range alts {
			if _, ok := data.Emulators[a.Emulator]; ok {
				kept = append(kept, a)
			}
		}
		if len(kept) == 0 {
			delete(data.Systems, slug)
			skipped++
			continue
		}
		data.Systems[slug] = kept
	}
	return data, skipped, nil
}

// parseCommand turns one ES-DE command into an alternative, reporting false
// for anything the launcher cannot honour: a RetroArch command (RomM's own
// core map covers those) or an unsupported placeholder.
func parseCommand(label, text string) (esde.Alternative, bool) {
	tokens := splitCommand(text)
	alt := esde.Alternative{Label: strings.TrimSpace(label)}
	i := 0
	for ; i < len(tokens); i++ {
		tok := tokens[i]
		if directives[tok] {
			continue
		}
		if dir, ok := strings.CutPrefix(tok, "%STARTDIR%="); ok {
			alt.StartDir = dir
			continue
		}
		if name, ok := emulatorName(tok); ok {
			if skipEmulators[name] {
				return alt, false
			}
			alt.Emulator = name
			i++
			break
		}
		// A leading token that is neither a directive nor the emulator means
		// something we do not model, such as %PRECOMMAND_WINE% or %INJECT%.
		return alt, false
	}
	if alt.Emulator == "" {
		return alt, false
	}
	for _, tok := range tokens[i:] {
		for _, ph := range placeholders(tok) {
			if !argPlaceholders[ph] {
				return alt, false
			}
		}
		alt.Args = append(alt.Args, tok)
	}
	return alt, true
}

func emulatorName(tok string) (string, bool) {
	name, ok := strings.CutPrefix(tok, "%EMULATOR_")
	if !ok {
		return "", false
	}
	name, ok = strings.CutSuffix(name, "%")
	return name, ok
}

// placeholders returns the %TOKEN% substrings in s.
func placeholders(s string) []string {
	var out []string
	for {
		start := strings.Index(s, "%")
		if start < 0 {
			return out
		}
		rest := s[start+1:]
		end := strings.Index(rest, "%")
		if end < 0 {
			return out
		}
		out = append(out, s[start:start+end+2])
		s = rest[end+1:]
	}
}

// splitCommand splits on unescaped whitespace, honouring double quotes and
// backslash escapes the way ES-DE does.
func splitCommand(s string) []string {
	var out []string
	var cur strings.Builder
	quoted, started := false, false
	flush := func() {
		if started {
			out = append(out, cur.String())
			cur.Reset()
			started = false
		}
	}
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '\\' && i+1 < len(s):
			i++
			cur.WriteByte(s[i])
			started = true
		case c == '"':
			quoted = !quoted
			started = true
		case (c == ' ' || c == '\t' || c == '\n' || c == '\r') && !quoted:
			flush()
		default:
			cur.WriteByte(c)
			started = true
		}
	}
	flush()
	return out
}

func trimAll(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func loadAliases(path string) (*aliasFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var a aliasFile
	if err := json.Unmarshal(data, &a); err != nil {
		return nil, err
	}
	if len(a.Slugs) == 0 {
		return nil, fmt.Errorf("%s: no slugs", path)
	}
	return &a, nil
}

func fetchXML(ref, esOS, name string, out any) error {
	url := fmt.Sprintf(
		"https://gitlab.com/es-de/emulationstation-de/-/raw/%s/resources/systems/%s/%s",
		ref, esOS, name,
	)
	resp, err := http.Get(url) //nolint:gosec // the URL is built from a pinned tag
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return xml.Unmarshal(body, out)
}

func writeJSON(path string, data *esde.Data) error {
	// encoding/json sorts map keys, so the file stays diffable across releases.
	buf, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(buf, '\n'), 0o644)
}
