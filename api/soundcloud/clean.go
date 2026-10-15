package soundcloud

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	linkCandidate = regexp.MustCompile(`(?i)(?:(?:https?|ftp)://|www\.)[^\s()\[\]{}<>"«»|]+|` +
		`(?:[\p{L}\p{N}](?:[\p{L}\p{N}-]*[\p{L}\p{N}])?\.)+\p{L}{2,24}(?:/[^\s()\[\]{}<>"«»|]*)?`)

	linkLabel = regexp.MustCompile(`(?i)(?:free\s*(?:dl|download)|download|dl|скачать|links?|ссылк[аи]|listen|слушать|stream|` +
		`tg|telegram|телеграм|телег[аи]|telega|тгк?|канал|channel|inst|insta|instagram|инст|vk|вк|yt|youtube|ютуб|` +
		`tiktok|тикток|buy|купить|site|website|сайт|e-?mail|mail|почта|booking|` +
		`(?:(?:leak|prod\.?|produced|mixed|uploaded)\s+)?by|via|on|at|от|на|в|` +
		`(?:подпишись|подписывайтесь|подпишитесь)(?:\s+на)?|follow(?:\s+(?:me|us))?(?:\s+on)?|subscribe(?:\s+on)?)` +
		`\s*[:\-–—=>]*\s*$`)
	emptyBrackets = regexp.MustCompile(`[(\[{（【「]\s*[-–—|:;,.·•/\\]*\s*[)\]}）】」]`)
	openSpace     = regexp.MustCompile(`([(\[{（【「])\s+`)
	closeSpace    = regexp.MustCompile(`\s+([)\]}）】」])`)
)

var bareTLDs = map[string]bool{"com": true, "net": true, "org": true, "ru": true, "su": true, "ua": true, "by": true,
	"kz": true, "рф": true, "info": true, "biz": true, "xyz": true, "site": true, "online": true, "store": true,
	"shop": true, "club": true, "live": true, "music": true, "app": true, "dev": true, "page": true, "link": true,
	"top": true, "space": true, "website": true}

func cleanText(s string) string {
	plain := strings.Join(strings.Fields(s), " ")
	if !strings.ContainsRune(s, '.') {
		return plain
	}
	var out strings.Builder
	last, removed := 0, false
	for _, m := range linkCandidate.FindAllStringIndex(s, -1) {
		start, end, ok := linkAt(s, m[0], m[1])
		if !ok || start < last {
			continue
		}

		before := strings.TrimRightFunc(s[last:start], unicode.IsSpace)
		if label := linkLabel.FindStringIndex(before); label != nil && boundaryBefore(before, label[0]) {
			start = last + label[0]
		}
		out.WriteString(s[last:start])
		out.WriteByte(' ')
		last, removed = end, true
	}
	if !removed {
		return plain
	}
	out.WriteString(s[last:])
	return tidy(out.String())
}

func linkAt(s string, i, j int) (int, int, bool) {
	prev, _ := utf8.DecodeLastRuneInString(s[:i])
	if i > 0 && prev == '@' {
		start := i - 1
		for start > 0 {
			r, size := utf8.DecodeLastRuneInString(s[:start])
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune("._%+-", r) {
				break
			}
			start -= size
		}
		return start, j, start < i-1
	}
	if i > 0 && (unicode.IsLetter(prev) || unicode.IsDigit(prev) || strings.ContainsRune("._-", prev)) {
		return 0, 0, false
	}
	match := strings.ToLower(s[i:j])
	for _, prefix := range []string{"http://", "https://", "ftp://", "www."} {
		if strings.HasPrefix(match, prefix) {
			return i, j, true
		}
	}
	host, _, hasPath := strings.Cut(match, "/")
	tld := host[strings.LastIndexByte(host, '.')+1:]
	if hasPath {
		return i, j, utf8.RuneCountInString(tld) <= 6
	}
	next, _ := utf8.DecodeRuneInString(s[j:])
	if j < len(s) && (unicode.IsLetter(next) || unicode.IsDigit(next) || next == '-') {
		return 0, 0, false
	}
	return i, j, bareTLDs[tld]
}

func boundaryBefore(s string, i int) bool {
	r, _ := utf8.DecodeLastRuneInString(s[:i])
	return i == 0 || !(unicode.IsLetter(r) || unicode.IsDigit(r))
}

func tidy(s string) string {
	for {
		next := emptyBrackets.ReplaceAllString(s, " ")
		if next == s {
			break
		}
		s = next
	}
	s = closeSpace.ReplaceAllString(openSpace.ReplaceAllString(s, "$1"), "$1")
	const separators = "-–—|:;,./\\•·~+@>="
	junk := func(field string) bool { return strings.Trim(field, separators) == "" }
	var fields []string
	for _, field := range strings.Fields(s) {
		if junk(field) && (len(fields) == 0 || junk(fields[len(fields)-1])) {
			continue
		}
		fields = append(fields, field)
	}
	for len(fields) > 0 && junk(fields[len(fields)-1]) {
		fields = fields[:len(fields)-1]
	}
	if len(fields) == 0 {
		return ""
	}
	const glued = "-–—|:;,/\\•·>="
	fields[0] = strings.TrimLeft(fields[0], glued)
	fields[len(fields)-1] = strings.TrimRight(fields[len(fields)-1], glued)
	return strings.Join(fields, " ")
}

func cleanTitle(s string) string {
	if cleaned := cleanText(s); cleaned != "" {
		return cleaned
	}
	if strings.TrimSpace(s) == "" {
		return ""
	}
	return "Без названия"
}
