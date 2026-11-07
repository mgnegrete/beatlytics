// Command import loads Spotify extended streaming history into beatlytics.db.
//
//	go run ./cmd/import --user <id> Streaming_History_Audio_*.json
//
// Without --user it lists known users and their IDs.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/mgnegrete/beatlytics/internal/importer"
	"github.com/mgnegrete/beatlytics/internal/store"
)

func main() {
	dbPath := flag.String("db", "beatlytics.db", "path to the SQLite database")
	userID := flag.String("user", "", "Beatlytics user ID to import into")
	flag.Parse()

	db, err := store.Open(*dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open db:", err)
		os.Exit(1)
	}

	if *userID == "" {
		users, err := db.ListUsers()
		if err != nil {
			fmt.Fprintln(os.Stderr, "list users:", err)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "Pass --user with one of these IDs:")
		for _, u := range users {
			fmt.Fprintf(os.Stderr, "  %s  %s\n", u.ID, u.DisplayName)
		}
		os.Exit(2)
	}

	var files []string
	for _, pattern := range flag.Args() {
		matches, err := filepath.Glob(pattern)
		if err != nil || len(matches) == 0 {
			matches = []string{pattern}
		}
		files = append(files, matches...)
	}
	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "no input files; usage: import --user <id> <file>...")
		os.Exit(2)
	}

	failed := false
	total := 0
	for _, path := range files {
		f, err := os.Open(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			failed = true
			continue
		}
		res, err := importer.ImportFile(db, *userID, filepath.Base(path), f)
		f.Close()
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			failed = true
			continue
		}
		total += res.Imported
		fmt.Printf("%s: imported %d%s\n", filepath.Base(path), res.Imported, formatSkips(res.Skipped))
	}
	fmt.Printf("total imported: %d\n", total)
	if failed {
		os.Exit(1)
	}
}

func formatSkips(m map[string]int) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	s := ", skipped"
	for _, k := range keys {
		s += fmt.Sprintf(" %s=%d", k, m[k])
	}
	return s
}
