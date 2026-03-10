// the history package is a collection of functions for reading history files from browsers.
package history

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// unlockDatabase is a bad hack for opening potentially locked SQLite databases.
func unlockDatabase(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	t, err := os.CreateTemp("", "namebench-history-*")
	if err != nil {
		return "", err
	}
	defer t.Close()

	written, err := io.Copy(t, f)
	if err != nil {
		return "", err
	}
	log.Printf("%d bytes written to %s", written, t.Name())
	return t.Name(), nil
}

// Chrome returns an array of URLs found in Chrome's history within X days
func Chrome(days int) (urls []string, err error) {
	return ChromiumFamily(days)
}

// ChromiumFamily returns URLs from Chrome-family browser profiles within X days.
func ChromiumFamily(days int) (urls []string, err error) {
	paths := chromiumHistoryPaths()
	query := historyQuery(days)

	seen := make(map[string]struct{}, 1024)
	for _, path := range paths {
		records, err := queryHistoryURLs(path, query)
		if err != nil {
			return nil, err
		}
		for _, record := range records {
			if _, exists := seen[record]; exists {
				continue
			}
			seen[record] = struct{}{}
			urls = append(urls, record)
		}
	}
	return urls, nil
}

func historyQuery(days int) string {
	return fmt.Sprintf(
		`SELECT urls.url FROM visits
		 LEFT JOIN urls ON visits.url = urls.id
		 WHERE (visit_time - 11644473600000000 >
			    strftime('%%s', date('now', '-%d day')) * 1000000)
		 ORDER BY visit_time DESC`, days)
}

func queryHistoryURLs(path string, query string) (urls []string, err error) {
	log.Printf("Checking %s", path)
	_, err = os.Stat(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	unlockedPath, err := unlockDatabase(path)
	if err != nil {
		return nil, err
	}
	defer os.Remove(unlockedPath)

	db, err := sql.Open("sqlite", unlockedPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(query)
	if err != nil {
		log.Printf("Query failed: %s", err)
		return nil, err
	}
	defer rows.Close()

	var url string
	for rows.Next() {
		if err := rows.Scan(&url); err != nil {
			return nil, err
		}
		urls = append(urls, url)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return urls, nil
}

func chromiumHistoryPaths() []string {
	roots := []string{
		"${HOME}/Library/Application Support/Google/Chrome",
		"${HOME}/Library/Application Support/Microsoft Edge",
		"${HOME}/Library/Application Support/BraveSoftware/Brave-Browser",
		"${HOME}/.config/google-chrome",
		"${HOME}/.config/microsoft-edge",
		"${HOME}/.config/BraveSoftware/Brave-Browser",
		"${LOCALAPPDATA}/Google/Chrome/User Data",
		"${LOCALAPPDATA}/Microsoft/Edge/User Data",
		"${LOCALAPPDATA}/BraveSoftware/Brave-Browser/User Data",
		"${APPDATA}/Google/Chrome/User Data",
		"${APPDATA}/Microsoft/Edge/User Data",
		"${APPDATA}/BraveSoftware/Brave-Browser/User Data",
		"${USERPROFILE}/AppData/Local/Google/Chrome/User Data",
		"${USERPROFILE}/AppData/Local/Microsoft/Edge/User Data",
		"${USERPROFILE}/AppData/Local/BraveSoftware/Brave-Browser/User Data",
	}

	paths := make([]string, 0, len(roots)*2)
	seen := make(map[string]struct{}, len(roots)*2)
	for _, rootPattern := range roots {
		root := os.ExpandEnv(rootPattern)
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()
			if name != "Default" && !strings.HasPrefix(name, "Profile ") {
				continue
			}
			historyPath := filepath.Join(root, name, "History")
			if _, err := os.Stat(historyPath); err != nil {
				continue
			}
			if _, exists := seen[historyPath]; exists {
				continue
			}
			seen[historyPath] = struct{}{}
			paths = append(paths, historyPath)
		}
	}
	return paths
}
