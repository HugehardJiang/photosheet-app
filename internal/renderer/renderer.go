package renderer

import (
	"context"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"

	"photosheet/internal/imaging"
	"photosheet/internal/metadata"
	"photosheet/internal/model"
	"photosheet/internal/platform"
)

type SourceImage struct {
	Record     model.Image
	SourcePath string
}

type Progress struct {
	Completed int
	Total     int
	Current   string
}

type Options struct {
	MemoryLimitBytes int64
	TileSize         int
	TempRoot         string
	Progress         func(Progress)
}

func RenderSheet(ctx context.Context, filename string, sheet model.Sheet, images []SourceImage, settings model.Settings, iccProfile []byte, options Options) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("渲染已取消: %w", err)
	}
	normalized, err := settings.Normalize()
	if err != nil {
		return err
	}
	if sheet.Width <= 0 || sheet.Height <= 0 {
		return errors.New("Sheet 尺寸无效")
	}
	if len(images) == 0 {
		return errors.New("Sheet 没有图片")
	}
	if normalized.WorkingBitDepth == 16 && normalized.Format != model.FormatJPEG {
		return RenderSheet16(ctx, filename, sheet, images, normalized, iccProfile, options)
	}
	memoryLimit := options.MemoryLimitBytes
	if normalized.MemoryLimitBytes > 0 {
		memoryLimit = normalized.MemoryLimitBytes
	}
	tileSize := options.TileSize
	if normalized.TileSize > 0 {
		tileSize = normalized.TileSize
	}
	canvas, err := NewCanvas(sheet.Width, sheet.Height, memoryLimit, tileSize, options.TempRoot)
	if err != nil {
		return fmt.Errorf("创建画布失败: %w", err)
	}
	defer canvas.Close()
	for i, source := range images {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("渲染已取消: %w", err)
		}
		decoded, profile, err := imaging.DecodeNormalizedFile(source.SourcePath)
		if err != nil {
			return fmt.Errorf("无法读取 %s: %w", source.Record.Filename, err)
		}
		if decoded.Bounds().Dx() != source.Record.DisplayWidth || decoded.Bounds().Dy() != source.Record.DisplayHeight {
			return fmt.Errorf("%s 解码后的显示尺寸为 %dx%d，与扫描记录 %dx%d 不一致", source.Record.Filename, decoded.Bounds().Dx(), decoded.Bounds().Dy(), source.Record.DisplayWidth, source.Record.DisplayHeight)
		}
		if source.Record.ICCProfileHash != metadata.ICCProfileHash(profile) {
			return fmt.Errorf("%s 的 ICC Profile 发生变化，拒绝静默继续", source.Record.Filename)
		}
		if source.Record.Rotation == 90 {
			decoded = imaging.Rotate90CW(decoded)
		}
		content := source.Record.ContentRect
		if decoded.Bounds().Dx() != content.Width || decoded.Bounds().Dy() != content.Height {
			return fmt.Errorf("%s 排版尺寸与解码尺寸不一致：排版为 %dx%d，解码为 %dx%d", source.Record.Filename, content.Width, content.Height, decoded.Bounds().Dx(), decoded.Bounds().Dy())
		}
		outer := source.Record.OuterRect
		paddingX := content.X - outer.X
		paddingY := content.Y - outer.Y
		if paddingX != paddingY || paddingX < 0 {
			return fmt.Errorf("%s 的 padding 坐标无效", source.Record.Filename)
		}
		if err := canvas.DrawImage(outer.X, outer.Y, decoded, paddingX, normalized.PaddingMode); err != nil {
			return fmt.Errorf("绘制 %s 失败: %w", source.Record.Filename, err)
		}
		if options.Progress != nil {
			options.Progress(Progress{Completed: i + 1, Total: len(images), Current: source.Record.Filename})
		}
	}
	if normalized.Format == model.FormatJPEG {
		if normalized.Mode != model.ModeJPEGCompatibility {
			return errors.New("严格无损模式禁止 JPEG Sheet")
		}
		if err := WriteJPEGAtomic(filename, canvas, normalized.JPEGQuality); err != nil {
			return err
		}
		return nil
	}
	if normalized.Format == model.FormatTIFF {
		return errors.New("8-bit TIFF Sheet 需要显式选择 16-bit 工作位深")
	}
	if err := WritePNGAtomic(filename, canvas, iccProfile); err != nil {
		return err
	}
	return nil
}

func VerifyImageSize(filename string, width, height int) error {
	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	config, _, err := image.DecodeConfig(f)
	if err != nil {
		return fmt.Errorf("无法读取 Sheet 尺寸: %w", err)
	}
	if config.Width != width || config.Height != height {
		return fmt.Errorf("Sheet 尺寸为 %dx%d，JSON 要求 %dx%d", config.Width, config.Height, width, height)
	}
	return nil
}

func EnsureOutputDirectory(filename string) error {
	if filename == "" {
		return errors.New("输出文件名为空")
	}
	return os.MkdirAll(filepath.Dir(filename), 0o755)
}

func CheckDiskSpace(filename string, width, height int, format model.SheetFormat) error {
	return CheckDiskSpaceForDepth(filename, width, height, format, 8)
}

func CheckDiskSpaceForDepth(filename string, width, height int, format model.SheetFormat, bitDepth int) error {
	if width <= 0 || height <= 0 {
		return errors.New("无法估算无效画布的磁盘空间")
	}
	free, err := platform.FreeBytes(filepath.Dir(filename))
	if err != nil || free == 0 {
		// Some virtual/network filesystems do not expose free space. Rendering
		// still proceeds and the atomic temp-file path reports a real error.
		return nil
	}
	bytesPerPixel := int64(4)
	if bitDepth >= 16 {
		bytesPerPixel = 8
	}
	if format == model.FormatJPEG {
		bytesPerPixel = 2
	}
	estimate := uint64(int64(width)*int64(height)*bytesPerPixel) + 16<<20
	if estimate > free || estimate > free*9/10 {
		return fmt.Errorf("磁盘空间不足：预计至少需要约 %d MB，当前可用约 %d MB", estimate/(1<<20), free/(1<<20))
	}
	return nil
}
