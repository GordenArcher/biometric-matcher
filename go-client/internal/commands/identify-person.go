package commands

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/GordenArcher/godenv"

	"github.com/GordenArcher/biometric-matcher/gen/biometricpb"
	"github.com/GordenArcher/biometric-matcher/internal/client"
	"github.com/GordenArcher/biometric-matcher/internal/crypto"
	"github.com/GordenArcher/biometric-matcher/internal/storage"
)

// RunIdentifyPerson is the real 1:N path, distinct from RunIdentify
// (which reads a directory of plaintext template files, no database
// involved). Store.ListTemplates already decrypts each row before
// returning it, so this command never handles ciphertext directly, same
// boundary as register/verify-person.
func RunIdentifyPerson(args []string) error {
	fs := flag.NewFlagSet("identify-person", flag.ExitOnError)
	scanPath := fs.String("scan", "", "path to a raw fingerprint scan file")
	matcherAddr := fs.String("matcher", "localhost:50051", "matcher service address")
	dbDSN := fs.String("db", godenv.Get("DATABASE_URL", ""), "Postgres connection string, defaults to $DATABASE_URL (or .env)")
	keyEnv := fs.String("key-env", "TEMPLATE_ENCRYPTION_KEY", "env var holding the base64 AES-256 key")
	limit := fs.Int("limit", 1000, "max number of registered people to search against")
	offset := fs.Int("offset", 0, "pagination offset into the register, for searching beyond -limit")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *scanPath == "" {
		return fmt.Errorf("-scan is required")
	}
	if *dbDSN == "" {
		return fmt.Errorf("-db or $DATABASE_URL must be set")
	}

	scanData, err := os.ReadFile(*scanPath)
	if err != nil {
		return fmt.Errorf("read scan file: %w", err)
	}

	ctx := context.Background()

	enc := crypto.NewEncryptor(crypto.EnvKeyProvider{EnvVar: *keyEnv})

	store, err := storage.Open(ctx, *dbDSN, enc)
	if err != nil {
		return err
	}
	defer store.Close()

	// This is Go's pagination over the register that proto/biometric.proto
	// calls for, the matcher never sees "the whole database", just
	// whatever batch this call decides to hand it.
	records, err := store.ListTemplates(ctx, *limit, *offset)
	if err != nil {
		return fmt.Errorf("load candidate templates: %w", err)
	}

	if len(records) == 0 {
		fmt.Println("no registered people found in this range")
		return nil
	}

	candidates := make([]*biometricpb.TemplateCandidate, 0, len(records))
	for _, r := range records {
		candidates = append(candidates, &biometricpb.TemplateCandidate{
			CandidateId: r.PersonID,
			Template:    r.Template,
		})
	}

	c, err := client.New(*matcherAddr)
	if err != nil {
		return err
	}
	defer c.Close()

	resp, err := c.Identify(ctx, scanData, candidates)
	if err != nil {
		return err
	}

	matches := resp.GetMatches()
	if len(matches) == 0 {
		fmt.Printf("no match found among %d registered people\n", len(records))
		return nil
	}

	for _, m := range matches {
		fmt.Printf("person %s, score %.2f\n", m.GetCandidateId(), m.GetScore())
	}
	return nil
}
