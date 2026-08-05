package models

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

type Book struct {
	Title  string `json:"title"`
	Author string `json:"author"`
}

// ParseBooks accepts either a single {"title":...,"author":...} object, a
// .json array of such objects, or a path to a .json file containing either.
// Validation is strict: unknown keys, wrong types, and entries without a
// non-empty title all cause the whole input to be rejected.
func ParseBooks(arg string) ([]Book, error) {
	var data []byte
	if arg != "" && looksLikePath(arg) {
		b, err := os.ReadFile(arg)
		if err != nil {
			return nil, fmt.Errorf("read file %q: %w", arg, err)
		}
		data = b
	} else {
		data = []byte(arg)
	}
	return parseBytes(data)
}

func parseBytes(data []byte) ([]Book, error) {
	trimmed := trimSpace(data)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty input")
	}
	var books []Book
	switch trimmed[0] {
	case '{':
		var book Book
		if err := decodeStrict(trimmed, &book); err != nil {
			return nil, fmt.Errorf("invalid json object: %w", err)
		}
		books = []Book{book}
	case '[':
		if err := decodeStrict(trimmed, &books); err != nil {
			return nil, fmt.Errorf("invalid json array: %w", err)
		}
	default:
		return nil, fmt.Errorf("input must be a json object, json array, or path to a .json file")
	}
	for i, b := range books {
		if strings.TrimSpace(b.Title) == "" {
			return nil, fmt.Errorf("entry %d: title is required", i+1)
		}
	}
	return books, nil
}

// decodeStrict decodes a single JSON value, rejecting unknown fields and any
// data trailing the value.
func decodeStrict(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return fmt.Errorf("trailing data after json value")
	}
	return nil
}

func looksLikePath(arg string) bool {
	if len(arg) > 5 && arg[len(arg)-5:] == ".json" {
		return true
	}
	if arg[0] != '{' && arg[0] != '[' {
		return true
	}
	return false
}

func trimSpace(b []byte) []byte {
	start := 0
	for start < len(b) {
		c := b[start]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			start++
			continue
		}
		break
	}
	end := len(b)
	for end > start {
		c := b[end-1]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			end--
			continue
		}
		break
	}
	return b[start:end]
}
