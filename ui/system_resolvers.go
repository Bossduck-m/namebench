package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

var resolverTokenPattern = regexp.MustCompile(`(?i)\b(?:\d{1,3}(?:\.\d{1,3}){3}|(?:[0-9a-f]{0,4}:){2,}[0-9a-f:]{0,4})\b`)

func discoverSystemResolvers() ([]string, string) {
	switch runtime.GOOS {
	case "windows":
		return discoverWindowsResolvers()
	default:
		return discoverResolvConf("/etc/resolv.conf")
	}
}

func discoverWindowsResolvers() ([]string, string) {
	command := `$ErrorActionPreference='Stop'; $servers = Get-DnsClientServerAddress -AddressFamily IPv4,IPv6 | Where-Object { $_.ServerAddresses } | ForEach-Object { $_.ServerAddresses } | Select-Object -Unique; $servers | ConvertTo-Json -Compress`
	output, err := exec.Command(windowsSystemCommand("powershell.exe"), "-NoProfile", "-NonInteractive", "-Command", command).CombinedOutput()
	if err == nil {
		servers, parseErr := parseWindowsResolverJSON(output)
		if parseErr == nil && len(servers) > 0 {
			return servers, ""
		}
	}

	servers := parseResolverText(runBestEffort(windowsSystemCommand("netsh.exe"), "interface", "ip", "show", "dnsservers"))
	servers = append(servers, parseResolverText(runBestEffort(windowsSystemCommand("netsh.exe"), "interface", "ipv6", "show", "dnsservers"))...)
	servers = normalizeResolverList(servers)
	if len(servers) > 0 {
		return servers, ""
	}

	if err != nil {
		return nil, fmt.Sprintf("System DNS discovery failed: %v", err)
	}
	return nil, "System DNS discovery returned no public resolvers."
}

func discoverResolvConf(path string) ([]string, string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Sprintf("System DNS discovery failed: %v", err)
	}
	servers := parseResolvConfNameservers(raw)
	if len(servers) == 0 {
		return nil, "System DNS discovery returned no public resolvers."
	}
	return servers, ""
}

func parseWindowsResolverJSON(raw []byte) ([]string, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}

	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		return normalizeResolverList(list), nil
	}

	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		return normalizeResolverList([]string{single}), nil
	}

	return nil, fmt.Errorf("unable to parse resolver json")
}

func parseResolvConfNameservers(raw []byte) []string {
	lines := strings.Split(string(raw), "\n")
	candidates := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(strings.SplitN(line, "#", 2)[0])
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.EqualFold(fields[0], "nameserver") {
			candidates = append(candidates, fields[1])
		}
	}
	return normalizeResolverList(candidates)
}

func parseResolverText(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return normalizeResolverList(resolverTokenPattern.FindAllString(raw, -1))
}

func normalizeResolverList(candidates []string) []string {
	if len(candidates) == 0 {
		return nil
	}

	normalized := make([]string, 0, len(candidates))
	seen := map[string]bool{}
	for _, candidate := range candidates {
		server, ok := normalizeNameServer(candidate)
		if !ok || seen[server] {
			continue
		}
		seen[server] = true
		normalized = append(normalized, server)
	}
	return normalized
}

func runBestEffort(name string, args ...string) string {
	output, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return ""
	}
	return string(output)
}

func windowsSystemCommand(name string) string {
	root := strings.TrimSpace(os.Getenv("SystemRoot"))
	if root == "" {
		root = `C:\Windows`
	}
	if strings.EqualFold(name, "powershell.exe") {
		return filepath.Join(root, "System32", "WindowsPowerShell", "v1.0", name)
	}
	return filepath.Join(root, "System32", name)
}
