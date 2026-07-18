package commands

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/GordenArcher/biometric-matcher/internal/client"
)

// RunVerify does a 1:1 check between a fresh scan and a previously
// enrolled template file. This mirrors what a real verification flow
// would do after decrypting a stored template, decryption is skipped
// here since this reads a plaintext template file straight off disk.
func RunVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	scanPath := fs.String("scan", "", "path to a raw fingerprint scan file")
	templatePath := fs.String("template", "", "path to a previously enrolled template file")
	target := fs.String("matcher", "localhost:50051", "matcher service address")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *scanPath == "" || *templatePath == "" {
		return fmt.Errorf("both -scan and -template are required")
	}

	scanData, err := os.ReadFile(*scanPath)
	if err != nil {
		return fmt.Errorf("read scan file: %w", err)
	}

	templateData, err := os.ReadFile(*templatePath)
	if err != nil {
		return fmt.Errorf("read template file: %w", err)
	}

	c, err := client.New(*target)
	if err != nil {
		return err
	}
	defer c.Close()

	resp, err := c.Verify(context.Background(), scanData, templateData)
	if err != nil {
		return err
	}

	fmt.Printf("match: %v, score: %.2f\n", resp.GetMatch(), resp.GetScore())
	return nil
}
