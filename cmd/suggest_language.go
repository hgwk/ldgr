package cmd

import (
	"fmt"
	"io"
)

func addWritingLanguage(skeleton map[string]any, writingLanguage string) {
	if writingLanguage != "" {
		skeleton["writing_language"] = writingLanguage
	}
}

func printWritingLanguageHint(stdout io.Writer, writingLanguage string) {
	if writingLanguage != "" {
		fmt.Fprintf(stdout, "Writing language: %s\n\n", writingLanguage)
	}
}

func localizedTaskPlaceholder(writingLanguage string) string {
	if writingLanguage == "ko" {
		return "<한 줄 작업 설명>"
	}
	return "<one-line>"
}

func localizedAcceptancePlaceholder(writingLanguage string) []any {
	if writingLanguage == "ko" {
		return []any{"<검증 가능한 완료 조건>"}
	}
	return []any{}
}

func localizedShippedResult(writingLanguage, task string) string {
	if writingLanguage == "ko" {
		return "출시 완료: " + task
	}
	return "shipped: " + task
}
