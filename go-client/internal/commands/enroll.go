package commands

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/GordenArcher/biometric-matcher/internal/client"
)

// RunEnroll reads a raw scan file, sends it to the matcher for template
// extraction, and writes the resulting template to disk. Encryption is
// intentionally not applied here yet, this command is for local testing
// against the matcher, not for feeding a real register, see README.
func RunEnroll(args []string) error {
	fs := flag.NewFlagSet("enroll", flag.ExitOnError)
	scanPath := fs.String("scan", "", "path to a raw fingerprint scan file")
	outPath := fs.String("out", "", "path to write the resulting template to")
	target := fs.String("matcher", "localhost:50051", "matcher service address")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *scanPath == "" || *outPath == "" {
		return fmt.Errorf("both -scan and -out are required")
	}

	scanData, err := os.ReadFile(*scanPath)
	if err != nil {
		return fmt.Errorf("read scan file: %w", err)
	}

	c, err := client.New(*target)
	if err != nil {
		return err
	}
	defer c.Close()

	resp, err := c.Enroll(context.Background(), scanData)
	if err != nil {
		return err
	}

	// 0644 rather than something tighter since this is a local dev/demo
	// tool, not the path that would ever write a production template to
	// disk unencrypted, tighten this if that assumption ever changes.
	if err := os.WriteFile(*outPath, resp.GetTemplate(), 0644); err != nil {
		return fmt.Errorf("write template file: %w", err)
	}

	fmt.Printf("enrolled, quality score %.2f, template written to %s\n", resp.GetQualityScore(), *outPath)
	return nil
}
