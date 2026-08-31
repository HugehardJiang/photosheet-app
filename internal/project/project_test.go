package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"photosheet/internal/model"
)

func TestLoadAllowsUnknownFieldsAndRejectsUnsafeSheet(t *testing.T) {
	root := t.TempDir()
	filename := filepath.Join(root, "project.json")
	content := `{"schemaVersion":1,"appVersion":"test","projectId":"abc","createdAt":"2025-01-01T00:00:00Z","mode":"strict-lossless","settings":{"maximumLongEdge":32,"mode":"strict-lossless","format":"png","padding":0,"paddingMode":"solid","allowRotation":false,"tileSize":64,"jpegQuality":95},"sheets":[],"futureField":{"keep":true}}`
	if err := os.WriteFile(filename, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(filename); err != nil {
		t.Fatal(err)
	}
	unsafe := filepath.Join(root, "unsafe.json")
	unsafeContent := strings.Replace(content, `"sheets":[]`, `"sheets":[{"id":"s","file":"../escape.png","width":1,"height":1,"format":"png","images":[]}]`, 1)
	if err := os.WriteFile(unsafe, []byte(unsafeContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(unsafe); err == nil {
		t.Fatal("expected unsafe sheet path to be rejected")
	}
}

func TestSettingsNormalizeStrictLosslessRejectsJPEG(t *testing.T) {
	settings := model.DefaultSettings()
	settings.Format = model.FormatJPEG
	if _, err := settings.Normalize(); err == nil {
		t.Fatal("expected strict lossless JPEG to be rejected")
	}
}

func TestLoadMigratesV1SheetIdentityInMemory(t *testing.T) {
	root := t.TempDir()
	filename := filepath.Join(root, "project.json")
	content := `{"schemaVersion":1,"appVersion":"test","projectId":"abc12345","createdAt":"2025-01-01T00:00:00Z","mode":"strict-lossless","settings":{"maximumLongEdge":32,"mode":"strict-lossless","format":"png","padding":0,"paddingMode":"solid","allowRotation":false,"tileSize":64,"jpegQuality":95},"sheets":[{"id":"sheet-001","file":"PS_abc12345_S001.png","width":4,"height":4,"format":"png","images":[]}]}`
	if err := os.WriteFile(filename, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := Load(filename)
	if err != nil {
		t.Fatal(err)
	}
	if p.SchemaVersion != model.CurrentSchemaVersion {
		t.Fatalf("schema=%d, want %d", p.SchemaVersion, model.CurrentSchemaVersion)
	}
	sheet := p.Sheets[0]
	if sheet.Index != 1 || sheet.CanonicalStem != "PS_abc12345_S001" || sheet.ID != "sheet-001" {
		t.Fatalf("migrated sheet=%#v", sheet)
	}
}
