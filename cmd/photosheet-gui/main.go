//go:build wails

package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"photosheet/internal/app"
	"photosheet/internal/model"
	"photosheet/internal/raw"
)

//go:embed all:frontend
var frontendAssets embed.FS

type guiPackRequest struct {
	Inputs      []string       `json:"inputs"`
	OutputDir   string         `json:"outputDir"`
	Settings    model.Settings `json:"settings"`
	SkipInvalid bool           `json:"skipInvalid"`
}

type guiUnpackRequest struct {
	ProjectPath            string             `json:"projectPath"`
	SheetsDir              string             `json:"sheetsDir"`
	ProcessedDirs          []string           `json:"processedDirs"`
	ProcessedFiles         []string           `json:"processedFiles"`
	Recursive              bool               `json:"recursive"`
	Bindings               map[string]string  `json:"bindings"`
	OutputDir              string             `json:"outputDir"`
	AllowProportionalScale bool               `json:"allowProportionalScale"`
	OutputFormat           model.OutputFormat `json:"outputFormat"`
	JPEGQuality            int                `json:"jpegQuality"`
}

type guiResolveRequest struct {
	ProjectPath    string            `json:"projectPath"`
	SheetsDir      string            `json:"sheetsDir"`
	ProcessedDirs  []string          `json:"processedDirs"`
	ProcessedFiles []string          `json:"processedFiles"`
	Recursive      bool              `json:"recursive"`
	Bindings       map[string]string `json:"bindings"`
}

type guiVerifyRequest struct {
	ProjectPath string `json:"projectPath"`
	SheetsDir   string `json:"sheetsDir"`
	CheckFiles  bool   `json:"checkFiles"`
}

type GUIApp struct {
	service *app.Service
	mu      sync.Mutex
	ctx     context.Context
	cancel  context.CancelFunc
}

func NewGUIApp() *GUIApp { return &GUIApp{service: app.NewService()} }

func (g *GUIApp) Diagnostics() raw.CapabilityReport {
	return g.service.Diagnostics()
}

func (g *GUIApp) startup(ctx context.Context) {
	g.mu.Lock()
	g.ctx = ctx
	g.mu.Unlock()
}

func (g *GUIApp) shutdown(_ context.Context) {
	g.mu.Lock()
	if g.cancel != nil {
		g.cancel()
	}
	g.cancel = nil
	g.mu.Unlock()
}

func (g *GUIApp) context() context.Context {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.ctx == nil {
		return context.Background()
	}
	return g.ctx
}

func (g *GUIApp) beginJob() (context.Context, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.cancel != nil {
		return nil, errors.New("已有任务正在运行")
	}
	base := g.ctx
	if base == nil {
		base = context.Background()
	}
	ctx, cancel := context.WithCancel(base)
	g.cancel = cancel
	return ctx, nil
}

func (g *GUIApp) endJob() {
	g.mu.Lock()
	g.cancel = nil
	g.mu.Unlock()
}

func (g *GUIApp) emit(ctx context.Context, progress app.Progress) {
	select {
	case <-ctx.Done():
		return
	default:
	}
	runtime.EventsEmit(g.context(), "photosheet:progress", progress)
}

func (g *GUIApp) progressEmitter(ctx context.Context) func(app.Progress) {
	return func(progress app.Progress) {
		g.emit(ctx, progress)
	}
}

func (g *GUIApp) finishJob(ctx context.Context, err error) {
	cancelled := errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled)
	if err == nil && !cancelled {
		return
	}
	progress := app.Progress{Status: "failed", Phase: "失败"}
	if cancelled {
		progress = app.Progress{Status: "cancelled", Phase: "已取消", Message: "任务已取消"}
	} else {
		progress.Message = err.Error()
	}
	runtime.EventsEmit(g.context(), "photosheet:progress", progress)
}

func (g *GUIApp) Pack(request guiPackRequest) (app.PackResponse, error) {
	ctx, err := g.beginJob()
	if err != nil {
		return app.PackResponse{}, err
	}
	defer g.endJob()
	response, err := g.service.Pack(ctx, app.PackRequest{
		Inputs: request.Inputs, OutputDir: request.OutputDir, Settings: request.Settings, SkipInvalid: request.SkipInvalid,
		Progress: g.progressEmitter(ctx),
	})
	g.finishJob(ctx, err)
	return response, err
}

func (g *GUIApp) Preview(request guiPackRequest) (app.PreviewResponse, error) {
	ctx, err := g.beginJob()
	if err != nil {
		return app.PreviewResponse{}, err
	}
	defer g.endJob()
	response, err := g.service.Preview(ctx, app.PackRequest{
		Inputs: request.Inputs, Settings: request.Settings, SkipInvalid: request.SkipInvalid,
		Progress: g.progressEmitter(ctx),
	})
	g.finishJob(ctx, err)
	return response, err
}

func (g *GUIApp) Unpack(request guiUnpackRequest) (app.UnpackResponse, error) {
	ctx, err := g.beginJob()
	if err != nil {
		return app.UnpackResponse{}, err
	}
	defer g.endJob()
	_ = g.rememberProcessedLocations(append(append([]string{}, request.ProcessedDirs...), request.ProcessedFiles...))
	response, err := g.service.Unpack(ctx, app.UnpackRequest{
		ProjectPath: request.ProjectPath, SheetsDir: request.SheetsDir, ProcessedDirs: request.ProcessedDirs, ProcessedFiles: request.ProcessedFiles,
		Recursive: request.Recursive, Bindings: request.Bindings, OutputDir: request.OutputDir,
		AllowProportionalScale: request.AllowProportionalScale, OutputFormat: request.OutputFormat, JPEGQuality: request.JPEGQuality,
		Progress: g.progressEmitter(ctx),
	})
	g.finishJob(ctx, err)
	return response, err
}

func (g *GUIApp) Resolve(request guiResolveRequest) (app.ResolveResponse, error) {
	ctx, err := g.beginJob()
	if err != nil {
		return app.ResolveResponse{}, err
	}
	defer g.endJob()
	response, err := g.service.ResolveProcessed(ctx, app.ResolveRequest{
		ProjectPath: request.ProjectPath, SheetsDir: request.SheetsDir, ProcessedDirs: request.ProcessedDirs,
		ProcessedFiles: request.ProcessedFiles, Recursive: request.Recursive, Bindings: request.Bindings, Progress: g.progressEmitter(ctx),
	})
	if response.ProjectPath == "" {
		response.ProjectPath = request.ProjectPath
	}
	g.finishJob(ctx, err)
	return response, err
}

func (g *GUIApp) Verify(request guiVerifyRequest) error {
	return g.service.Verify(app.VerifyRequest{ProjectPath: request.ProjectPath, SheetsDir: request.SheetsDir, CheckFiles: request.CheckFiles})
}

func (g *GUIApp) Cancel() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.cancel != nil {
		g.cancel()
	}
	return nil
}

func (g *GUIApp) ChooseImages() ([]string, error) {
	return runtime.OpenMultipleFilesDialog(g.context(), runtime.OpenDialogOptions{
		Title:   "选择照片或 RAW 文件",
		Filters: []runtime.FileFilter{{DisplayName: "JPG/PNG/TIFF/RAW images", Pattern: "*.jpg;*.jpeg;*.png;*.tif;*.tiff;*.dng;*.arw;*.cr2;*.cr3;*.nef;*.rw2;*.raf;*.orf;*.pef"}},
	})
}

func (g *GUIApp) ChooseProcessedFiles() ([]string, error) {
	return runtime.OpenMultipleFilesDialog(g.context(), runtime.OpenDialogOptions{
		Title:   "选择处理后的 Sheet 文件",
		Filters: []runtime.FileFilter{{DisplayName: "PNG、JPEG、TIFF 图片", Pattern: "*.jpg;*.jpeg;*.png;*.tif;*.tiff"}},
	})
}

func (g *GUIApp) ChooseProject() (string, error) {
	selected, err := runtime.OpenFileDialog(g.context(), runtime.OpenDialogOptions{
		Title:   "选择 project.json",
		Filters: []runtime.FileFilter{{DisplayName: "JSON 项目文件", Pattern: "*.json"}},
	})
	if err != nil || selected == "" {
		return selected, err
	}
	if !strings.EqualFold(filepath.Base(selected), "project.json") {
		return "", fmt.Errorf("请选择名为 project.json 的项目文件")
	}
	return selected, nil
}

func (g *GUIApp) ChooseFolder() (string, error) {
	return runtime.OpenDirectoryDialog(g.context(), runtime.OpenDialogOptions{Title: "选择照片文件夹"})
}

func (g *GUIApp) ChooseDirectory(title string) (string, error) {
	return runtime.OpenDirectoryDialog(g.context(), runtime.OpenDialogOptions{Title: title})
}

func (g *GUIApp) RecentProcessedLocations() []string {
	data, err := os.ReadFile(recentLocationsFilename())
	if err != nil {
		return []string{}
	}
	var locations []string
	if json.Unmarshal(data, &locations) != nil {
		return []string{}
	}
	return locations
}

func (g *GUIApp) RememberProcessedLocations(locations []string) error {
	return g.rememberProcessedLocations(locations)
}

func (g *GUIApp) rememberProcessedLocations(locations []string) error {
	if len(locations) == 0 {
		return nil
	}
	existing := g.RecentProcessedLocations()
	all := append(append([]string{}, locations...), existing...)
	seen := make(map[string]struct{}, len(all))
	result := make([]string, 0, 8)
	for _, location := range all {
		absolute, err := filepath.Abs(location)
		if err != nil || absolute == "." {
			continue
		}
		if _, ok := seen[absolute]; ok {
			continue
		}
		seen[absolute] = struct{}{}
		result = append(result, absolute)
		if len(result) == 8 {
			break
		}
	}
	if len(result) == 0 {
		return nil
	}
	filename := recentLocationsFilename()
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		return fmt.Errorf("创建 PhotoSheet 配置目录失败: %w", err)
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filename, append(data, '\n'), 0o644)
}

func recentLocationsFilename() string {
	configDir, err := os.UserConfigDir()
	if err != nil || configDir == "" {
		return filepath.Join(os.TempDir(), "PhotoSheet", "recent-processed-locations.json")
	}
	return filepath.Join(configDir, "PhotoSheet", "recent-processed-locations.json")
}

func main() {
	gui := NewGUIApp()
	assets, err := fs.Sub(frontendAssets, "frontend/dist")
	if err != nil {
		fmt.Fprintf(os.Stderr, "PhotoSheet 界面资源缺失：%v\n", err)
		os.Exit(1)
	}
	if err := wails.Run(&options.App{
		Title:      "PhotoSheet",
		Width:      1360,
		Height:     860,
		MinWidth:   1040,
		MinHeight:  680,
		Assets:     assets,
		OnStartup:  gui.startup,
		OnShutdown: gui.shutdown,
		Bind:       []interface{}{gui},
	}); err != nil {
		fmt.Fprintf(os.Stderr, "PhotoSheet 启动失败：%v\n", err)
		os.Exit(1)
	}
}
