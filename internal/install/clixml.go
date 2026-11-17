package install

import (
	"html"
	"regexp"
	"strings"
)

var (
	clixmlStream = regexp.MustCompile(`<S S="(error|warning|verbose)">(.*?)</S>`)
	clixmlTag    = regexp.MustCompile(`<[^>]+>`)
)

// CleanCLIXML filters powershell's stderr. With -EncodedCommand powershell
// wraps warnings and errors in CLIXML ("#< CLIXML" followed by one XML line),
// which would otherwise reach the log as raw markup. The header is dropped, the
// text of warning and error streams is kept, and everything else in the XML
// (progress records, module preparation) is discarded. Other lines pass through.
func CleanCLIXML(next func(string)) func(string) {
	return func(line string) {
		t := strings.TrimSpace(line)
		if t == "#< CLIXML" {
			return
		}
		if strings.HasPrefix(t, "<Objs ") {
			for _, m := range clixmlStream.FindAllStringSubmatch(t, -1) {
				msg := html.UnescapeString(clixmlTag.ReplaceAllString(m[2], ""))
				msg = strings.TrimSpace(strings.ReplaceAll(msg, "_x000D__x000A_", " "))
				if msg != "" {
					next(msg)
				}
			}
			return
		}
		next(line)
	}
}
