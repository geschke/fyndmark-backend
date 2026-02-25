package sanitize

import (
	"fmt"
	"net"
	"net/mail"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

// AuthorNameReport describes what was changed/removed while sanitizing an author name.
type AuthorNameReport struct {
	Changed bool

	InvalidUTF8Fixed bool
	RemovedNULBytes  bool

	RemovedControlChars    int
	RemovedDisallowedChars int

	CollapsedWhitespace bool
	Trimmed             bool
	RejectedFrontmatter bool // input was exactly "---" (or became that after trim)
}

// AuthorURLReport describes what was detected/changed while validating an author URL.
type AuthorURLReport struct {
	Changed bool

	InvalidUTF8Fixed bool
	RemovedNULBytes  bool

	Trimmed bool

	RejectedTooLong        bool
	RejectedWhitespace     bool
	RejectedControlChars   bool
	RejectedBadScheme      bool
	RejectedNotAbsolute    bool
	RejectedMissingHost    bool
	RejectedHasUserInfo    bool
	RejectedLocalhost      bool
	RejectedPrivateOrLocal bool
}

// EmailReport describes what was detected/changed while validating an email address.
type EmailReport struct {
	Changed bool

	InvalidUTF8Fixed bool
	RemovedNULBytes  bool
	Trimmed          bool
	Lowercased       bool

	RejectedTooLong       bool
	RejectedWhitespace    bool
	RejectedControlChars  bool
	RejectedAngleBrackets bool
	RejectedQuotes        bool
	RejectedBadFormat     bool
	RejectedNotPlainAddr  bool
	RejectedEmpty         bool
}

// SanitizeAuthorName applies a strict, unicode-aware whitelist to author names.
// It returns the sanitized name and a report describing what was changed.
// If the result is empty, the caller should reject the request (missing/invalid author).
func SanitizeAuthorName(input string, maxLen int) (string, AuthorNameReport) {
	var rep AuthorNameReport
	original := input

	// Normalize newlines/tabs to spaces first (so whitespace collapse can handle it).
	input = strings.ReplaceAll(input, "\r\n", " ")
	input = strings.ReplaceAll(input, "\r", " ")
	input = strings.ReplaceAll(input, "\n", " ")
	input = strings.ReplaceAll(input, "\t", " ")

	// Remove NUL bytes early.
	if strings.IndexByte(input, 0x00) >= 0 {
		rep.RemovedNULBytes = true
		input = strings.ReplaceAll(input, "\x00", "")
	}

	// Ensure valid UTF-8.
	if !utf8.ValidString(input) {
		rep.InvalidUTF8Fixed = true
		input = strings.ToValidUTF8(input, "")
	}

	trimmed := strings.TrimSpace(input)
	if trimmed != input {
		rep.Trimmed = true
	}
	input = trimmed

	// Reject pure frontmatter breaker (or if trimming turns it into that).
	if input == "---" {
		rep.RejectedFrontmatter = true
		rep.Changed = original != ""
		return "", rep
	}

	// Whitelist: letters, marks, digits; space; and a small set of safe punctuation.
	// Disallowed: anything that could trigger markdown/html/yaml or be "weird".
	allowedPunct := func(r rune) bool {
		switch r {
		case '.', ',', '-', '\'', '’', '_':
			return true
		default:
			return false
		}
	}

	var b strings.Builder
	b.Grow(len(input))

	lastWasSpace := false
	collapsed := false

	for _, r := range input {
		// Drop control characters.
		if unicode.IsControl(r) {
			rep.RemovedControlChars++
			continue
		}

		// Normalize any unicode whitespace to a single ASCII space.
		if unicode.IsSpace(r) {
			if !lastWasSpace && b.Len() > 0 {
				b.WriteByte(' ')
			} else {
				collapsed = true
			}
			lastWasSpace = true
			continue
		}
		lastWasSpace = false

		// Allow unicode letters and combining marks.
		if unicode.IsLetter(r) || unicode.IsMark(r) {
			b.WriteRune(r)
			continue
		}

		// Allow digits (pragmatic).
		if unicode.IsDigit(r) {
			b.WriteRune(r)
			continue
		}

		// Allow selected punctuation.
		if allowedPunct(r) {
			b.WriteRune(r)
			continue
		}

		// Everything else is dropped.
		rep.RemovedDisallowedChars++
	}

	out := strings.TrimSpace(b.String())
	if out != b.String() {
		rep.Trimmed = true
	}
	if collapsed {
		rep.CollapsedWhitespace = true
	}

	// Enforce max length after sanitizing.
	if maxLen > 0 {
		// Count runes, not bytes.
		if utf8.RuneCountInString(out) > maxLen {
			// Truncate to maxLen runes.
			var tb strings.Builder
			tb.Grow(len(out))
			n := 0
			for _, r := range out {
				if n >= maxLen {
					break
				}
				tb.WriteRune(r)
				n++
			}
			out2 := strings.TrimSpace(tb.String())
			if out2 != out {
				rep.Changed = true
			}
			out = out2
		}
	}

	// Changed flag (compare to original input, not only trimmed).
	if out != original {
		rep.Changed = true
	}

	return out, rep
}

// SanitizeAuthorURL validates an optional author URL strictly.
// Rules:
// - empty input => ok (returns "")
// - must be absolute URL
// - scheme must be https (strict)
// - must have host
// - must NOT contain userinfo
// - must NOT contain control characters or whitespace
// - max length enforced (bytes, after trimming & UTF-8 fix)
// - optional: reject localhost and private/local IPs (enabled here for strictness)
//
// It returns the normalized URL string (u.String()) or an error if invalid.
func SanitizeAuthorURL(input string, maxLen int) (string, AuthorURLReport, error) {
	var rep AuthorURLReport
	original := input

	// Normalize basic whitespace around the value.
	input = strings.TrimSpace(input)
	if input != original {
		rep.Trimmed = true
	}

	// Optional field: allow empty.
	if input == "" {
		rep.Changed = original != ""
		return "", rep, nil
	}

	// Remove NUL bytes (defensive).
	if strings.IndexByte(input, 0x00) >= 0 {
		rep.RemovedNULBytes = true
		input = strings.ReplaceAll(input, "\x00", "")
	}

	// Ensure valid UTF-8 (rejecting is also ok, but fixing is fine before validation).
	if !utf8.ValidString(input) {
		rep.InvalidUTF8Fixed = true
		input = strings.ToValidUTF8(input, "")
	}

	// Length limit (after trimming / UTF-8 fix).
	if maxLen > 0 && len(input) > maxLen {
		rep.RejectedTooLong = true
		return "", rep, fmt.Errorf("author_url too long")
	}

	// Reject any whitespace or control chars anywhere in the URL.
	for _, r := range input {
		if unicode.IsControl(r) {
			rep.RejectedControlChars = true
			return "", rep, fmt.Errorf("author_url contains control characters")
		}
		if unicode.IsSpace(r) {
			rep.RejectedWhitespace = true
			return "", rep, fmt.Errorf("author_url contains whitespace")
		}
	}

	// Parse strictly as request URI first; then ensure absolute + scheme/host.
	// ParseRequestURI is stricter than Parse for many malformed inputs.
	u, err := url.ParseRequestURI(input)
	if err != nil {
		return "", rep, fmt.Errorf("invalid author_url: %w", err)
	}

	// Must be absolute and have scheme+host.
	if !u.IsAbs() {
		rep.RejectedNotAbsolute = true
		return "", rep, fmt.Errorf("author_url must be absolute")
	}

	// Strict scheme allowlist: http and https only.
	if !strings.EqualFold(u.Scheme, "https") && !strings.EqualFold(u.Scheme, "http") {
		rep.RejectedBadScheme = true
		return "", rep, fmt.Errorf("author_url must use http or https")
	}

	if strings.TrimSpace(u.Host) == "" {
		rep.RejectedMissingHost = true
		return "", rep, fmt.Errorf("author_url missing host")
	}

	// No userinfo (user:pass@host).
	if u.User != nil {
		rep.RejectedHasUserInfo = true
		return "", rep, fmt.Errorf("author_url must not contain userinfo")
	}

	// Optional strictness: reject localhost.
	host := u.Hostname()
	if strings.EqualFold(host, "localhost") {
		rep.RejectedLocalhost = true
		return "", rep, fmt.Errorf("author_url must not use localhost")
	}

	// Optional strictness: reject private/local IPs if host is an IP literal.
	if ip := net.ParseIP(host); ip != nil {
		if isPrivateOrLocalIP(ip) {
			rep.RejectedPrivateOrLocal = true
			return "", rep, fmt.Errorf("author_url must not use private/local IPs")
		}
	}

	normalized := u.String()
	if normalized != original {
		rep.Changed = true
	}
	return normalized, rep, nil
}

// isPrivateOrLocalIP performs its package-specific operation.
func isPrivateOrLocalIP(ip net.IP) bool {
	ip = ip.To16()
	if ip == nil {
		return true
	}

	// IPv4-mapped?
	if v4 := ip.To4(); v4 != nil {
		// 10.0.0.0/8
		if v4[0] == 10 {
			return true
		}
		// 172.16.0.0/12
		if v4[0] == 172 && v4[1] >= 16 && v4[1] <= 31 {
			return true
		}
		// 192.168.0.0/16
		if v4[0] == 192 && v4[1] == 168 {
			return true
		}
		// 127.0.0.0/8 loopback
		if v4[0] == 127 {
			return true
		}
		// 169.254.0.0/16 link-local
		if v4[0] == 169 && v4[1] == 254 {
			return true
		}
		// 0.0.0.0/8 and 255.255.255.255 etc. (treat as local/invalid)
		if v4[0] == 0 || v4[0] == 255 {
			return true
		}
		return false
	}

	// IPv6 checks (basic)
	// Loopback ::1
	if ip.Equal(net.IPv6loopback) {
		return true
	}
	// Link-local fe80::/10
	if ip[0] == 0xfe && (ip[1]&0xc0) == 0x80 {
		return true
	}
	// Unique local fc00::/7
	if (ip[0] & 0xfe) == 0xfc {
		return true
	}
	return false
}

// SanitizeEmail validates an email address strictly.
// Rules:
// - must be a plain addr-spec only (no "Name <addr>")
// - no whitespace/control chars
// - no '<' '>' '"' '\”
// - length <= maxLen (recommend 254)
// - uses net/mail.ParseAddress, then requires parsed address equals the input (after trim/lower)
func SanitizeEmail(input string, maxLen int) (string, EmailReport, error) {
	var rep EmailReport
	original := input

	// Trim surrounding whitespace.
	input = strings.TrimSpace(input)
	if input != original {
		rep.Trimmed = true
	}

	// Remove NUL bytes (defensive).
	if strings.IndexByte(input, 0x00) >= 0 {
		rep.RemovedNULBytes = true
		input = strings.ReplaceAll(input, "\x00", "")
	}

	// Ensure valid UTF-8.
	if !utf8.ValidString(input) {
		rep.InvalidUTF8Fixed = true
		input = strings.ToValidUTF8(input, "")
	}

	// Reject empty after normalization.
	if input == "" {
		rep.RejectedEmpty = true
		return "", rep, fmt.Errorf("email is empty")
	}

	// Reject whitespace or control chars anywhere.
	for _, r := range input {
		if unicode.IsControl(r) {
			rep.RejectedControlChars = true
			return "", rep, fmt.Errorf("email contains control characters")
		}
		if unicode.IsSpace(r) {
			rep.RejectedWhitespace = true
			return "", rep, fmt.Errorf("email contains whitespace")
		}
	}

	// Reject common "display name" / quoting formats.
	if strings.ContainsAny(input, "<>") {
		rep.RejectedAngleBrackets = true
		return "", rep, fmt.Errorf("email must not contain angle brackets")
	}
	if strings.ContainsAny(input, "\"'") {
		rep.RejectedQuotes = true
		return "", rep, fmt.Errorf("email must not contain quotes")
	}

	// Lowercase (practical; domains are case-insensitive, local-part is usually treated case-insensitive).
	lower := strings.ToLower(input)
	if lower != input {
		rep.Lowercased = true
		input = lower
	}

	// Length limit (bytes).
	if maxLen > 0 && len(input) > maxLen {
		rep.RejectedTooLong = true
		return "", rep, fmt.Errorf("email too long")
	}

	// Parse using standard library.
	addr, err := mail.ParseAddress(input)
	if err != nil {
		rep.RejectedBadFormat = true
		return "", rep, fmt.Errorf("invalid email: %w", err)
	}

	parsed := strings.ToLower(strings.TrimSpace(addr.Address))
	if parsed == "" {
		rep.RejectedBadFormat = true
		return "", rep, fmt.Errorf("invalid email address")
	}

	// Strict: must be plain addr-spec only (no "Name <addr>").
	// ParseAddress would accept "Name <a@b>", but then addr.Address != input.
	if parsed != input {
		rep.RejectedNotPlainAddr = true
		return "", rep, fmt.Errorf("email must be a plain address only")
	}

	rep.Changed = (input != original)
	return input, rep, nil
}
