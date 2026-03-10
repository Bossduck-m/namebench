// part of the history package, provides filtering capabilities.
package history

import (
	"math/rand"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/publicsuffix"
)

var (
	internalRe = regexp.MustCompile(`\.corp|\.sandbox\.|\.borg\.|\.hot\.|internal|dmz|\._[ut][dc]p\.|intra|\.\w$|\.\w{5,}$`)
)

func isWebURL(u *url.URL) bool {
	switch strings.ToLower(strings.TrimSpace(u.Scheme)) {
	case "http", "https":
		return true
	default:
		return false
	}
}

func isPossiblyInternal(addr string) bool {
	// note: this happens to reject IPs and anything with a port at the end.
	_, icann := publicsuffix.PublicSuffix(addr)
	if !icann {
		return true
	}
	if internalRe.MatchString(addr) {
		return true
	}
	return false
}

// Filter out external hostnames from history
func ExternalHostnames(entries []string) (hostnames []string) {
	for _, uString := range entries {
		u, err := url.ParseRequestURI(uString)
		if err != nil {
			continue
		}
		if !isWebURL(u) {
			continue
		}
		host := u.Hostname()
		if host == "" {
			continue
		}
		if !isPossiblyInternal(host) {
			hostnames = append(hostnames, host)
		}
	}
	return
}

// Filter input array for unique entries.
func Uniq(input []string) (output []string) {
	seen := make(map[string]struct{}, len(input))
	for _, i := range input {
		if _, exists := seen[i]; exists {
			continue
		}
		seen[i] = struct{}{}
		output = append(output, i)
	}
	return
}

// Randomly select X number of entries.
func Random(count int, input []string) (output []string) {
	if count <= 0 || len(input) == 0 {
		return nil
	}
	if count > len(input) {
		count = len(input)
	}

	selected := make(map[int]bool)

	for {
		if len(selected) >= count {
			return
		}
		index := rand.Intn(len(input))
		// If we have already picked this number, re-roll.
		if _, exists := selected[index]; exists == true {
			continue
		}
		output = append(output, input[index])
		selected[index] = true
	}
}
