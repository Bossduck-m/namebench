package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const resolverMetadataTimeout = 4 * time.Second

type resolverMetadata struct {
	IP           string
	ASN          string
	ASName       string
	Country      string
	Organization string
}

var (
	resolverMetadataClient = &http.Client{Timeout: resolverMetadataTimeout}
	resolverMetadataCache  sync.Map
)

func enrichServerMetadata(results []serverBenchmark) []string {
	if len(results) == 0 {
		return nil
	}

	warnings := []string{}
	seen := map[string]resolverMetadata{}

	for i := range results {
		ip := resolverServerIP(results[i].Server)
		if ip == "" {
			continue
		}
		metadata, ok := seen[ip]
		if !ok {
			resolved, err := lookupResolverMetadata(context.Background(), ip)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("Resolver metadata lookup failed for %s: %v", results[i].Server, err))
				continue
			}
			metadata = resolved
			seen[ip] = metadata
		}
		results[i].ResolverIP = metadata.IP
		results[i].ResolverASN = metadata.ASN
		results[i].ResolverASName = metadata.ASName
		results[i].ResolverCountry = metadata.Country
		results[i].ResolverOrganization = metadata.Organization
	}

	return warnings
}

func lookupResolverMetadata(ctx context.Context, ip string) (resolverMetadata, error) {
	if value, ok := resolverMetadataCache.Load(ip); ok {
		return value.(resolverMetadata), nil
	}

	ctx, cancel := context.WithTimeout(ctx, resolverMetadataTimeout)
	defer cancel()

	metadata := resolverMetadata{IP: ip}

	ripe, ripeErr := fetchRIPEMetadata(ctx, ip)
	if ripeErr == nil {
		metadata.ASN = ripe.ASN
		metadata.ASName = ripe.ASName
	}

	rdap, rdapErr := fetchRDAPMetadata(ctx, ip)
	if rdapErr == nil {
		if metadata.Organization == "" {
			metadata.Organization = rdap.Organization
		}
		metadata.Country = rdap.Country
		if metadata.ASName == "" {
			metadata.ASName = rdap.Organization
		}
	}

	if metadata.Organization == "" {
		metadata.Organization = metadata.ASName
	}
	if metadata.Country == "" {
		metadata.Country = "--"
	}

	if metadata.ASN == "" && metadata.Organization == "" && ripeErr != nil && rdapErr != nil {
		return resolverMetadata{}, fmt.Errorf("ripe=%v; rdap=%v", ripeErr, rdapErr)
	}

	resolverMetadataCache.Store(ip, metadata)
	return metadata, nil
}

type ripeNetworkInfoResponse struct {
	Data struct {
		ASNs []string `json:"asns"`
	} `json:"data"`
}

type ripePrefixOverviewResponse struct {
	Data struct {
		ASNs []struct {
			ASN    int    `json:"asn"`
			Holder string `json:"holder"`
		} `json:"asns"`
	} `json:"data"`
}

func fetchRIPEMetadata(ctx context.Context, ip string) (resolverMetadata, error) {
	networkInfo, err := fetchJSON[ripeNetworkInfoResponse](ctx, "https://stat.ripe.net/data/network-info/data.json?resource="+ip)
	if err != nil {
		return resolverMetadata{}, err
	}
	prefixOverview, err := fetchJSON[ripePrefixOverviewResponse](ctx, "https://stat.ripe.net/data/prefix-overview/data.json?resource="+ip)
	if err != nil {
		return resolverMetadata{}, err
	}

	metadata := resolverMetadata{IP: ip}
	if len(networkInfo.Data.ASNs) > 0 {
		metadata.ASN = strings.TrimSpace(networkInfo.Data.ASNs[0])
	}
	if len(prefixOverview.Data.ASNs) > 0 {
		if metadata.ASN == "" && prefixOverview.Data.ASNs[0].ASN > 0 {
			metadata.ASN = fmt.Sprintf("%d", prefixOverview.Data.ASNs[0].ASN)
		}
		metadata.ASName = strings.TrimSpace(prefixOverview.Data.ASNs[0].Holder)
	}
	return metadata, nil
}

type rdapResponse struct {
	Name     string `json:"name"`
	Country  string `json:"country"`
	Entities []struct {
		Roles      []string      `json:"roles"`
		VCardArray []interface{} `json:"vcardArray"`
	} `json:"entities"`
}

func fetchRDAPMetadata(ctx context.Context, ip string) (resolverMetadata, error) {
	response, err := fetchJSON[rdapResponse](ctx, "https://rdap-bootstrap.arin.net/bootstrap/ip/"+ip)
	if err != nil {
		return resolverMetadata{}, err
	}

	metadata := resolverMetadata{
		IP:           ip,
		Organization: strings.TrimSpace(response.Name),
		Country:      strings.TrimSpace(response.Country),
	}

	for _, entity := range response.Entities {
		if !roleMatch(entity.Roles, "registrant") && !roleMatch(entity.Roles, "administrative") {
			continue
		}
		org, country := parseVCardEntity(entity.VCardArray)
		if metadata.Organization == "" {
			metadata.Organization = org
		}
		if metadata.Country == "" {
			metadata.Country = country
		}
	}

	return metadata, nil
}

func roleMatch(roles []string, target string) bool {
	for _, role := range roles {
		if strings.EqualFold(strings.TrimSpace(role), target) {
			return true
		}
	}
	return false
}

func parseVCardEntity(raw []interface{}) (organization string, country string) {
	if len(raw) < 2 {
		return "", ""
	}
	entries, ok := raw[1].([]interface{})
	if !ok {
		return "", ""
	}

	for _, entry := range entries {
		fields, ok := entry.([]interface{})
		if !ok || len(fields) < 4 {
			continue
		}
		key, _ := fields[0].(string)
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "fn", "org":
			if value, ok := fields[3].(string); ok && organization == "" {
				organization = strings.TrimSpace(value)
			}
		case "adr":
			country = extractCountryFromAddressValue(fields[3])
		}
	}

	return organization, country
}

func extractCountryFromAddressValue(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return countryFromLabel(typed)
	case []interface{}:
		for i := len(typed) - 1; i >= 0; i-- {
			if text, ok := typed[i].(string); ok {
				text = strings.TrimSpace(text)
				if text != "" {
					return text
				}
			}
		}
	}
	return ""
}

func countryFromLabel(label string) string {
	lines := strings.Split(strings.ReplaceAll(label, "\r", ""), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return line
		}
	}
	return ""
}

func resolverServerIP(server string) string {
	host, _, err := net.SplitHostPort(server)
	if err != nil {
		return ""
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return ""
	}
	return ip.String()
}

func fetchJSON[T any](ctx context.Context, url string) (T, error) {
	var payload T

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return payload, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "namebench/0.2.5 resolver-metadata")

	resp, err := resolverMetadataClient.Do(req)
	if err != nil {
		return payload, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return payload, fmt.Errorf("http %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return payload, err
	}
	return payload, nil
}
