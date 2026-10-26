package client

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"text/template"
	"time"

	"github.com/google/uuid"
)

// templateVars defines variables available in payload templates.
// This is the authoritative source for all template variables.
// Add new template variables here as needed.
type templateVars struct {
	Random      string // 8-character random hex string (e.g., 728d5ee0)
	UUID        string // Full UUID v4 (e.g., dda65639-774b-4ad1-bd98-4d43bd4d9425)
	Timestamp   int64  // Unix timestamp in seconds (e.g., 1768990421)
	TimestampMs int64  // Unix timestamp in milliseconds (e.g., 1768990421000)
}

// newTemplateVars creates a new set of template variables with current values
func newTemplateVars() *templateVars {
	now := time.Now()

	// Generate 8-character random hex string
	randomBytes := make([]byte, 4) // 4 bytes = 8 hex chars
	if _, err := rand.Read(randomBytes); err != nil {
		// crypto/rand.Read guarantees a nil error; an entropy failure is unrecoverable.
		panic(fmt.Sprintf("failed to generate random template value: %v", err))
	}

	return &templateVars{
		Timestamp:   now.Unix(),
		TimestampMs: now.UnixMilli(),
		Random:      hex.EncodeToString(randomBytes),
		UUID:        uuid.New().String(),
	}
}

// renderTemplate renders a payload template with dynamic variables
func renderTemplate(templateContent []byte) ([]byte, error) {
	tmpl, err := template.New("payload").Parse(string(templateContent))
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}

	vars := newTemplateVars()
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vars); err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.Bytes(), nil
}

func loadPayloadFromFile[T any](payloadPath string) (*T, error) {
	// #nosec G304 -- payloadPath is a user-provided test data file path
	data, err := os.ReadFile(payloadPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read payload file %s: %w", payloadPath, err)
	}

	// Render template to replace dynamic variables
	renderedData, err := renderTemplate(data)
	if err != nil {
		return nil, fmt.Errorf("failed to render template: %w", err)
	}

	var payload T
	if err := json.Unmarshal(renderedData, &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	return &payload, nil
}
