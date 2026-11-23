// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"fmt"
	"regexp"
	"strings"
)

// noticeFormats are the library's notices the window translates: the English
// format each one is written with (by notice.Info/Warn, or fmt.Sprintf before
// it), and its catalog key. The key's English text is the format with each
// verb replaced by a {{name}}, in order (TestNoticeTemplatesFollowFormats), so
// a notice that changes wording in the code stops matching and shows in
// English, as before, until its entry here and in the catalogs is updated.
// Formats that are only an error (%v, "%s: %s") stay as they are.
var noticeFormats = []struct{ key, format string }{
	// browser
	{"notice.browser.closedUnexpectedly", "The browser closed unexpectedly (its window was closed, or it crashed); starting a new one."},
	{"notice.browser.startingHidden", "%s needs a real browser: starting it in a hidden window (this can take a few seconds)..."},
	{"notice.browser.startingVisible", "%s needs a real browser: starting it with a visible window (this can take a few seconds)..."},
	{"notice.browser.startingBackground", "%s needs a real browser: starting it in the background (this can take a few seconds)..."},
	{"notice.browser.headlessFailed", "%s didn't load in a headless browser (probably a Cloudflare check)."},
	{"notice.browser.openingVisible", "Opening a visible browser window to pass the check: solve it there if asked..."},
	{"notice.browser.closingAfterFailure", "Closing the browser after the failure, so the next try starts a fresh one."},
	{"notice.browser.httpNewBrowser", "%s answered HTTP %d: waiting %s, then trying again in a new browser"},
	{"notice.browser.httpRetry", "%s answered HTTP %d: waiting %s before trying again"},
	{"notice.browser.waitingWindow", "Waiting for %s to load in the browser window (%s); solve the check there if one is showing..."},
	{"notice.browser.waitingHidden", "Waiting for %s to load in the browser (%s); a Cloudflare check may be showing..."},
	{"notice.browser.popupClosed", "Closed a pop-up tab."},
	// downloads
	{"notice.download.duplicatesFailed", "looking for duplicates in %s: %s"},
	{"notice.download.mirrorTrying", "page %d: %s failed (%s), trying %s"},
	{"notice.download.mirrorGot", "page %d: got it from %s"},
	{"notice.download.mirrorFailed", "page %d: %s failed too (%s)"},
	{"notice.download.tooMany", "page %d: %s answered 429 (too many requests), waiting to try again"},
	{"notice.download.retry", "page %d: %s, trying again in %s (try %d of %d)"},
	// sites
	{"notice.site.groupsFetch", "could not fetch the series page to name the scanlation groups: %s"},
	{"notice.site.groupsRead", "could not read the series page to name the scanlation groups: %s"},
	{"notice.site.groupsFind", "could not find the scanlation groups in the series page: %s"},
	{"notice.site.groupsParse", "could not parse the scanlation groups from the series page: %s"},
	{"notice.site.noPageURL", "page %d of %s has no URL to fetch from 😕 (will be ignored)"},
	{"notice.site.genericPlainPage", "%s has no built-in support: the generic reader found %d chapters in the plain page. Its chapter numbers and pages are guesses, so check them."},
	{"notice.site.genericChaptersPage", "%s has no built-in support: the generic reader found %d chapters in the plain chapters page. Its chapter numbers and pages are guesses, so check them."},
	{"notice.site.genericBrowser", "%s has no built-in support: the generic reader found %d chapters in a hidden browser. Its chapter numbers and pages are guesses, so check them."},
	{"notice.site.listIncomplete", "found %d chapters but the site declares %d: the list may be incomplete"},
	{"notice.site.listEmbedded", "could not fetch the full chapter list from the api, using the list embedded in the page: %s"},
	{"notice.site.protectedPages", "could not decode the protected pages: %s"},
	{"notice.site.protectedPagesOf", "could not decode the protected pages of %s: %s"},
	{"notice.site.pageFetch", "error fetching page %d: %s"},
	{"notice.site.pageParse", "error parsing page %d: %s"},
	{"notice.site.imageNoURL", "page %d of %s has an image with no URL to fetch from"},
	{"notice.mangaplus.locked", "%d of %d chapters need a Manga Plus MAX subscription and can't be downloaded"},
	{"notice.mangaplus.fewUse", "This MangaPlus version lists only %d chapters (each language is a separate version: %s): use --language to pick another"},
	{"notice.mangaplus.fewPick", "This MangaPlus version lists only %d chapters (each language is a separate version: %s): pick another one with --language"},
	{"notice.mangaplus.noVersion", "MangaPlus has no %s version of %s (available: %s); downloading the %s version the link points at"},
	{"notice.mangaplus.noOthers", "MangaPlus listed no other versions of %s, so %s isn't available; downloading the %s version the link points at"},
	{"notice.mangaplus.switched", "The provided MangaPlus link is for the %s version of %s; --language selected %s, so that's the version being downloaded"},
	{"notice.mangaplus.mismatch", "MangaPlus's listing for the %s version of %s doesn't match the title it returns: downloading the %s version"},
	{"notice.mangaplus.secretRejected", "the cached MangaPlus device secret was rejected, registering a new device"},
	{"notice.mangaplus.secretNotStored", "registered a new MangaPlus device, but could not store its secret in %s (it will be registered again on the next run): %s"},
	{"notice.mangaplus.secretStored", "registered a new MangaPlus device, whose secret is stored in %s"},
	// packing
	{"notice.pack.epubPageLeftOut", "- warning: page %s: %s, leaving it out"},
	{"notice.pack.epubNoPages", "- warning: %s has no pages EPUB readers can show, leaving it out"},
	{"notice.pack.convertFailed", "- warning: page %03d: converting %s to jpeg: %s (keeping the original)"},
	// pacing (site-rate-limits.txt, site-parallel.txt, site-mirrors.txt, site-pace.txt)
	{"notice.pace.lineIgnored", "%s line %d: %s (ignored)"},
	{"notice.pace.duplicate", "%s line %d: %s is listed again; the first line is used"},
	{"notice.pace.saveFailed", "saving %s: %s"},
	{"notice.pace.calmRemoved", "%s: no 429 for %s, so its time is taken out of %s"},
	{"notice.pace.calmLowered", "%s: no 429 for %s, now one request every %s (saved in %s)"},
	{"notice.pace.tooMany", "%s answered 429 Too Many Requests: waiting %s, then one request every %s (saved in %s)"},
	{"notice.pace.struggling", "%s is struggling: down from %d to %d downloads at once (saved in %s)"},
	{"notice.pace.settled", "%s settled at %d downloads at once"},
	// the options file
	{"notice.options.unreadable", "ignoring the saved options: %v"},
	{"notice.options.packAs", "ignoring the saved pack-as %q, using %q"},
	{"notice.options.format", "ignoring the saved format %q, using %q"},
	{"notice.options.readingDirection", "ignoring the saved reading-direction %q, using %q"},
	{"notice.options.convertImages", "ignoring the saved convert-images %q, using %q: %v"},
	{"notice.options.jpegQuality", "ignoring the saved jpeg-quality %d, using %d"},
	{"notice.options.uiLanguage", "ignoring the saved ui-language %q, following the system"},
}

// formatVerbRE matches one fmt verb, flags and width included
var formatVerbRE = regexp.MustCompile(`%[-+# 0]*[0-9]*(?:\.[0-9]+)?[a-zA-Z]`)

type noticeForm struct {
	key   string
	re    *regexp.Regexp
	names []string
}

// noticeForms are noticeFormats as patterns over the formatted text: each
// verb captures what it printed, named after the English template's
// placeholder in the same position
var noticeForms = compileNotices()

func compileNotices() []noticeForm {
	forms := make([]noticeForm, 0, len(noticeFormats))
	for _, f := range noticeFormats {
		template, ok := catalog[f.key]
		if !ok {
			panic(fmt.Sprintf("notice %s: not in the English catalog", f.key))
		}
		var names []string
		for _, m := range placeholderRE.FindAllStringSubmatch(template, -1) {
			names = append(names, m[1])
		}
		literals := formatVerbRE.Split(f.format, -1)
		if len(literals)-1 != len(names) {
			panic(fmt.Sprintf("notice %s: %d verbs but %d placeholders", f.key, len(literals)-1, len(names)))
		}
		var pattern strings.Builder
		pattern.WriteString(`(?s)^`)
		for i, literal := range literals {
			if i > 0 {
				pattern.WriteString(`(.*?)`)
			}
			pattern.WriteString(regexp.QuoteMeta(literal))
		}
		pattern.WriteString(`$`)
		forms = append(forms, noticeForm{key: f.key, re: regexp.MustCompile(pattern.String()), names: names})
	}

	return forms
}

// noticeMessage returns the library notice text as a Message the window
// translates, with what each verb printed as its args, or false when text is
// none of noticeFormats. Text stays the notice as written, for log.txt.
func noticeMessage(text string) (Message, bool) {
	for _, f := range noticeForms {
		m := f.re.FindStringSubmatch(text)
		if m == nil {
			continue
		}
		var args map[string]any
		if len(f.names) > 0 {
			args = make(map[string]any, len(f.names))
			for i, name := range f.names {
				args[name] = m[i+1]
			}
		}

		return Message{Key: f.key, Args: args, Text: text}, true
	}

	return Message{}, false
}
