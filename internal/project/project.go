package project

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"photosheet/internal/model"
)

// AppVersion is a variable so release builds can stamp a semantic version with
// -ldflags -X without changing the source tree.
var AppVersion = "0.1.0"

func New(id string, settings model.Settings) (*model.Project, error) {
	normalized, err := settings.Normalize()
	if err != nil {
		return nil, err
	}
	return &model.Project{
		SchemaVersion: model.CurrentSchemaVersion,
		AppVersion:    AppVersion,
		ProjectID:     id,
		CreatedAt:     nowUTC().Format(time.RFC3339),
		Mode:          normalized.Mode,
		Settings:      normalized,
		Sheets:        []model.Sheet{},
	}, nil
}

// nowUTC is a variable to make project serialization deterministic in tests.
var nowUTC = func() (t time.Time) { return time.Now().UTC() }

func SaveAtomic(p *model.Project, filename string) error {
	if p == nil {
		return errors.New("不能保存空 project")
	}
	if err := p.Validate(); err != nil {
		return fmt.Errorf("project 无效: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		return fmt.Errorf("创建 project 目录失败: %w", err)
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("编码 project.json 失败: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(filename), ".project-*.tmp")
	if err != nil {
		return fmt.Errorf("创建 project 临时文件失败: %w", err)
	}
	tmpName := tmp.Name()
	keep := false
	defer func() {
		_ = tmp.Close()
		if !keep {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0o644); err != nil {
		return fmt.Errorf("设置 project 临时文件权限失败: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("写入 project.json 失败: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("同步 project.json 失败: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("关闭 project 临时文件失败: %w", err)
	}
	if err := os.Rename(tmpName, filename); err != nil {
		return fmt.Errorf("原子替换 project.json 失败: %w", err)
	}
	keep = true
	return nil
}

func Load(filename string) (*model.Project, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("无法读取 project.json %s: %w", filename, err)
	}
	defer f.Close()
	decoder := json.NewDecoder(io.LimitReader(f, 64<<20))
	var p model.Project
	if err := decoder.Decode(&p); err != nil {
		return nil, fmt.Errorf("project.json 格式无效: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, errors.New("project.json 含有多余 JSON 内容")
		}
		return nil, fmt.Errorf("project.json 尾部格式无效: %w", err)
	}
	if err := migrate(&p); err != nil {
		return nil, err
	}
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("project.json 校验失败: %w", err)
	}
	return &p, nil
}

func migrate(p *model.Project) error {
	legacySchema := p.SchemaVersion < model.CurrentSchemaVersion
	if p.SchemaVersion == 0 {
		// v0 files from early prototypes had no version but otherwise used the v1 shape.
		p.SchemaVersion = 1
	}
	if p.SchemaVersion > model.CurrentSchemaVersion {
		return fmt.Errorf("project.json 版本 %d 高于当前支持的版本 %d", p.SchemaVersion, model.CurrentSchemaVersion)
	}
	if p.AppVersion == "" {
		p.AppVersion = AppVersion
	}
	if p.Mode == "" {
		p.Mode = p.Settings.Mode
	}
	if legacySchema && p.Settings.WorkingBitDepth == 0 {
		// v1/v2 projects were rendered through the legacy uint8 pipeline. Keep
		// their semantics stable; newly created projects default to 16-bit.
		p.Settings.WorkingBitDepth = 8
	}
	for i := range p.Sheets {
		sheet := &p.Sheets[i]
		if sheet.Format == "" {
			sheet.Format = p.Settings.Format
		}
		if sheet.Index <= 0 {
			sheet.Index = i + 1
		}
		if sheet.ID == "" {
			sheet.ID = model.StableSheetID(p.ProjectID, sheet.Index)
		}
		if sheet.CanonicalStem == "" {
			sheet.CanonicalStem = model.CanonicalStemFromFile(sheet.File)
		}
	}
	// v2 adds identity fields used by the processed-Sheet resolver. The
	// migration is in-memory; project.json is only rewritten by an explicit
	// SaveAtomic/upgrade operation.
	if p.SchemaVersion < model.CurrentSchemaVersion {
		p.SchemaVersion = model.CurrentSchemaVersion
	}
	return nil
}

func WriteSidecar(sheet model.Sheet, filename string, p *model.Project) error {
	if p == nil {
		return errors.New("不能为 nil project 写 sidecar")
	}
	data, err := json.MarshalIndent(struct {
		SchemaVersion int         `json:"schemaVersion"`
		AppVersion    string      `json:"appVersion"`
		ProjectID     string      `json:"projectId"`
		Sheet         model.Sheet `json:"sheet"`
	}{p.SchemaVersion, p.AppVersion, p.ProjectID, sheet}, "", "  ")
	if err != nil {
		return fmt.Errorf("编码 Sheet sidecar 失败: %w", err)
	}
	data = append(data, '\n')
	return writeBytesAtomic(filename, data)
}

func writeBytesAtomic(filename string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(filename), ".photosheet-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	keep := false
	defer func() {
		_ = tmp.Close()
		if !keep {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := io.Copy(tmp, bytes.NewReader(data)); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, filename); err != nil {
		return err
	}
	keep = true
	return nil
}
