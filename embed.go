// Package dropper provides embedded assets for the dropper binary.
package dropper

import "embed"

// VersionsYAML contains the embedded versions.yaml manifest.
//
//go:embed versions.yaml
var VersionsYAML []byte

// StaticFS contains the embedded static assets (CSS, JS, HTMX).
//
//go:embed static/*
var StaticFS embed.FS

// TemplateFS contains the embedded HTML templates.
//
//go:embed templates/*
var TemplateFS embed.FS
