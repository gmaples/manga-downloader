// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package packer

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"unicode/utf8"

	"github.com/elboletaire/manga-downloader/grabber"
)

// FilenameTemplateParts represents the parts of a filename
type FilenameTemplateParts struct {
	// Series represents the series name (e.g. "One Piece")
	Series string
	// Number represents the chapter number (e.g. "1.0")
	Number string
	// Title is the chapter's label (e.g. "The Beginning", or "Chapter 12" when
	// the site gives no title), see grabber.ChapterLabel
	Title string
	// SiteTitle is the site's own chapter title, empty when it gives none or
	// for a volume or bundle
	SiteTitle string
	// Group is the scanlation group that made the release (e.g. "Alpha"). For a
	// volume or a bundle, the group its chapters share, empty when they differ
	// or name none. Several groups of one release are joined with ", "
	Group string
	// Volume is the volume number: the chapter's own (0 when the site doesn't
	// say) or, for a volume file, the volume's (e.g. 3). It's an int, so
	// {{printf "%02d" .Volume}} pads it.
	Volume int
	// Version placeholder appended to the title in case of duplicate filenames (e.g. "3")
	Version int
}

// FilenameTemplateDefault is the default filename template
const FilenameTemplateDefault = "{{.Series}} {{.Number}} - {{.Title}}{{if .Group}} [{{.Group}}]{{end}}{{if gt .Version 1}} v{{.Version}}{{end}}"

// filenameTemplateLegacyDefault is the default before {{.Group}} was added. It's
// still saved in gui.json files, so it's still treated as the default
const filenameTemplateLegacyDefault = "{{.Series}} {{.Number}} - {{.Title}}{{if gt .Version 1}} v{{.Version}}{{end}}"

// FilenameVolumeTemplateDefault names volume files (--volumes) while
// --filename-template is the chapter default, which would read oddly on a
// volume; the chapter range tells two groupings of one volume number apart
const FilenameVolumeTemplateDefault = `{{.Series}} Vol. {{printf "%02d" .Volume}} ({{.Number}}){{if .Group}} [{{.Group}}]{{end}}{{if gt .Version 1}} v{{.Version}}{{end}}`

// MigrateFilenameTemplate returns FilenameTemplateDefault in place of the legacy
// default a saved template may still hold, and templ otherwise
func MigrateFilenameTemplate(templ string) string {
	if templ == filenameTemplateLegacyDefault {
		return FilenameTemplateDefault
	}
	return templ
}

// isDefaultTemplate reports whether templ is a chapter default: blank, the
// current default or the legacy one
func isDefaultTemplate(templ string) bool {
	templ = strings.TrimSpace(templ)
	return templ == "" || templ == FilenameTemplateDefault || templ == filenameTemplateLegacyDefault
}

// templateNamesFile matches an action that reads a field telling files apart,
// however it is spaced, trimmed or piped
var templateNamesFile = regexp.MustCompile(`\{\{-?\s*[^}]*\.(Number|Title|SiteTitle|Volume)\b`)

// TemplateWarning returns a warning when a per-chapter templ names no number,
// title or volume, so files only differ by a version suffix; "" when it's
// fine or blank (the default)
func TemplateWarning(templ string) string {
	if strings.TrimSpace(templ) == "" || templateNamesFile.MatchString(templ) {
		return ""
	}
	return "This template has no {{.Number}}, {{.Title}}, {{.SiteTitle}} or {{.Volume}}, so files only differ by a version suffix."
}

// VolumeTemplate returns the template volume files are named with: the
// volume default when the filename template is a chapter default (or
// unset), the user's own template otherwise
func VolumeTemplate(templ string) string {
	if isDefaultTemplate(templ) {
		return FilenameVolumeTemplateDefault
	}
	return templ
}

// NewFilenameFromTemplate returns a new filename from a series title, a chapter and a template.
// A blank template is the default.
func NewFilenameFromTemplate(templ string, parts FilenameTemplateParts) (string, error) {
	if strings.TrimSpace(templ) == "" {
		templ = FilenameTemplateDefault
	}
	tmpl, err := template.New("filename").Funcs(template.FuncMap{"pad": padNumber}).Parse(templ)
	if err != nil {
		return "", err
	}

	buffer := new(bytes.Buffer)
	err = tmpl.Execute(buffer, parts)

	return buffer.String(), err
}

// padNumber is the template's pad: number with its whole part zero-padded to
// width digits and its decimals kept ({{pad .Number 4}} is "0010.5" for 10.5)
func padNumber(number any, width int) string {
	s := fmt.Sprint(number)
	digits := len(s) - len(strings.TrimLeft(s, "0123456789"))
	if digits == 0 || digits >= width {
		return s
	}
	return strings.Repeat("0", width-digits) + s
}

// NewChapterFileTemplateParts returns a new FilenameTemplateParts from a title and a chapter
func NewChapterFileTemplateParts(title string, chapter *grabber.Chapter) FilenameTemplateParts {
	return FilenameTemplateParts{
		Series:    SanitizeFilename(title),
		Number:    NumberText(chapter.GetNumber()),
		Title:     SanitizeFilename(grabber.ChapterLabel(chapter)),
		SiteTitle: SanitizeFilename(chapter.GetTitle()),
		Group:     SanitizeFilename(chapter.Group),
		Volume:    chapter.Volume,
	}
}

// NewVolumeFileTemplateParts returns the FilenameTemplateParts of a volume file:
// Number is rng ("1-10,13-28") or, when empty, the first and last chapter ("1-7"),
// and Title "Vol. 01" for templates written with chapters in mind.
func NewVolumeFileTemplateParts(title string, volume int, rng string, chapters []*grabber.Chapter) FilenameTemplateParts {
	number := rng
	if number == "" && len(chapters) > 0 {
		first, last := chapters[0].GetNumber(), chapters[len(chapters)-1].GetNumber()
		number = formatChapterNumber(first)
		if last != first {
			number += "-" + formatChapterNumber(last)
		}
	}
	return FilenameTemplateParts{
		Series: SanitizeFilename(title),
		Number: number,
		Title:  fmt.Sprintf("Vol. %02d", volume),
		Group:  sharedGroup(chapters),
		Volume: volume,
	}
}

// sharedGroup returns the scanlation group every chapter was made by, sanitized
// for a filename, and "" when they differ or none names one
func sharedGroup(chapters []*grabber.Chapter) string {
	if len(chapters) == 0 {
		return ""
	}
	group := chapters[0].Group
	for _, c := range chapters[1:] {
		if c.Group != group {
			return ""
		}
	}

	return SanitizeFilename(group)
}

// formatChapterNumber formats a chapter number as filenames show it: 1, 10.5
func formatChapterNumber(n float64) string {
	return strconv.FormatFloat(n, 'f', -1, 64)
}

// paddedChapterNumber formats a chapter number zero-padded to (at least) 4
// integer digits so lexicographic ordering matches numeric ordering (e.g.
// 1 -> "0001", 10.5 -> "0010.5", 1186 -> "1186"). Reuses the same
// "%.1f, strip trailing .0" idea as NewChapterFileTemplateParts.
func paddedChapterNumber(number float64) string {
	numStr := strings.Replace(fmt.Sprintf("%.1f", number), ".0", "", 1)

	intPart, fracPart := numStr, ""
	if i := strings.IndexByte(numStr, '.'); i >= 0 {
		intPart, fracPart = numStr[:i], numStr[i:]
	}

	intVal, err := strconv.Atoi(intPart)
	if err != nil {
		// Fall back to the unpadded representation if parsing ever fails.
		return numStr
	}

	return fmt.Sprintf("%04d%s", intVal, fracPart)
}

// SanitizeFilename sanitizes a filename: it replaces the characters Windows
// refuses in a name (and the path separators) with ones it accepts, and the
// control characters (0x00-0x1F, 0x7F) with "_"
func SanitizeFilename(filename string) string {
	sanitized := strings.Replace(filename, "/", "_", -1)
	sanitized = strings.Replace(sanitized, "\\", "_", -1)
	sanitized = strings.Replace(sanitized, ":", ";", -1)
	sanitized = strings.Replace(sanitized, "?", "¿", -1)
	sanitized = strings.Replace(sanitized, `"`, "'", -1)
	sanitized = strings.Replace(sanitized, "<", "_", -1)
	sanitized = strings.Replace(sanitized, ">", "_", -1)
	sanitized = strings.Replace(sanitized, "|", "_", -1)
	sanitized = strings.Replace(sanitized, "*", "_", -1)
	sanitized = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return '_'
		}
		return r
	}, sanitized)

	return sanitized
}

// windowsReserved are the device names Windows refuses as a file or folder
// name, with or without an extension
var windowsReserved = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// DirnameUntitled is the folder name SanitizeDirname gives a name with nothing usable left
const DirnameUntitled = "Untitled"

// MaxNameBytes is the longest file or folder name, in UTF-8 bytes, that every
// target takes: ext4 and f2fs (Linux, Android) cap a name at 255 bytes, and
// NTFS, APFS and exFAT at 255 UTF-16 units, which 255 bytes never exceed
const MaxNameBytes = 255

// nameTail is how many bytes of a name's end a shortened name keeps, so the
// " v2" suffix and the end of the title survive
const nameTail = 40

// portableName makes name, already through SanitizeFilename, valid on Windows,
// macOS, Linux and Android (whose shared storage follows FAT's rules):
//
//   - a leading dot becomes "_", as Linux, macOS and Android hide dot names
//   - with trimEnd, trailing spaces and dots go, as Windows and FAT drop them
//     (only a name without an extension ends there)
//   - a reserved Windows device name (CON, NUL, COM1, …) gets "_" after its stem
//   - over maxBytes, the middle is cut and marked "…", keeping the start and
//     the last nameTail bytes, on rune boundaries
//
// It's deterministic, so a rerun gives the same name and skip-existing finds it.
func portableName(name string, maxBytes int, trimEnd bool) string {
	if trimEnd {
		name = strings.TrimRight(name, " .")
	}
	if strings.HasPrefix(name, ".") {
		name = "_" + name[1:]
	}
	stem := name
	if i := strings.IndexByte(stem, '.'); i >= 0 {
		stem = stem[:i]
	}
	if windowsReserved[strings.ToUpper(strings.TrimRight(stem, " "))] {
		// after the stem, as Windows reads "CON.x_" as CON too
		name = stem + "_" + name[len(stem):]
	}
	if len(name) > maxBytes {
		const mark = "…"
		tail := name[len(name)-nameTail:]
		for !utf8.ValidString(tail) {
			tail = tail[1:]
		}
		head := name[:maxBytes-len(mark)-len(tail)]
		for !utf8.ValidString(head) {
			head = head[:len(head)-1]
		}
		name = head + mark + tail
	}

	return name
}

// SanitizeDirname turns name into one folder name that is valid on Windows,
// macOS, Linux and Android: SanitizeFilename's replacements, then
// portableName's rules with trailing spaces and dots trimmed (so "." and ".."
// can't remain), at most maxBytes long, and DirnameUntitled when nothing is
// left. The result never holds a path separator, so it can't be absolute or
// climb out of the folder it's joined to. Unicode text is kept.
func SanitizeDirname(name string, maxBytes int) string {
	sanitized := strings.TrimRight(SanitizeFilename(name), " .")
	if sanitized == "" {
		return DirnameUntitled
	}

	return portableName(sanitized, maxBytes, true)
}
