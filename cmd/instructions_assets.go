package cmd

import (
	"embed"
	"fmt"
)

//go:embed instructions/*.md
var instructionFS embed.FS

func loadInstructionBody() (string, error) {
	data, err := instructionFS.ReadFile("instructions/ldgr.md")
	if err != nil {
		return "", fmt.Errorf("read embedded ldgr operating guide: %w", err)
	}
	return string(data), nil
}
