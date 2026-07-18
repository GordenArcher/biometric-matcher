package commands

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/GordenArcher/biometric-matcher/gen/biometricpb"
	"github.com/GordenArcher/biometric-matcher/internal/client"
)

// RunIdentify does a 1:N search against every template file in a
// directory, using the filename (minus extension) as the candidate ID.
// This stands in for what would normally be a paginated Postgres query
// in a real gateway, good enough for demoing the identify RPC without
// standing up a database first.
func RunIdentify(args []string) error {
	fs := flag.NewFlagSet("identify", flag.ExitOnError)
	scanPath := fs.String("scan", "", "path to a raw fingerprint scan file")
	templatesDir := fs.String("templates", "", "directory of template files to search against")
	target := fs.String("matcher", "localhost:50051", "matcher service address")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *scanPath == "" || *templatesDir == "" {
		return fmt.Errorf("both -scan and -templates are required")
	}

	scanData, err := os.ReadFile(*scanPath)
	if err != nil {
		return fmt.Errorf("read scan file: %w", err)
	}

	candidates, err := loadCandidates(*templatesDir)
	if err != nil {
		return err
	}

	c, err := client.New(*target)
	if err != nil {
		return err
	}
	defer c.Close()

	resp, err := c.Identify(context.Background(), scanData, candidates)
	if err != nil {
		return err
	}

	matches := resp.GetMatches()
	if len(matches) == 0 {
		fmt.Println("no match found")
		return nil
	}

	for _, m := range matches {
		fmt.Printf("candidate %s, score %.2f\n", m.GetCandidateId(), m.GetScore())
	}
	return nil
}

func loadCandidates(dir string) ([]*biometricpb.TemplateCandidate, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read templates dir: %w", err)
	}

	var candidates []*biometricpb.TemplateCandidate
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			// Skip and keep going rather than failing the whole batch on
			// one bad file, identify is a search operation, one unreadable
			// candidate shouldn't block matching against the rest.
			fmt.Fprintf(os.Stderr, "skipping %s: %v\n", path, err)
			continue
		}

		id := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		candidates = append(candidates, &biometricpb.TemplateCandidate{
			CandidateId: id,
			Template:    data,
		})
	}

	return candidates, nil
}
