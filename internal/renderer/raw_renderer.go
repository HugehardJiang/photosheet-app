package renderer

import (
	"context"
	"errors"
	"fmt"
	"image"

	"photosheet/internal/imaging"
	"photosheet/internal/metadata"
	"photosheet/internal/model"
	"photosheet/internal/raw"
)

func RenderSheet16(ctx context.Context, filename string, sheet model.Sheet, images []SourceImage, settings model.Settings, iccProfile []byte, options Options) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("16-bit RAW 渲染已取消: %w", err)
	}
	normalized, err := settings.Normalize()
	if err != nil {
		return err
	}
	if normalized.WorkingBitDepth != 16 {
		return errors.New("RenderSheet16 要求 workingBitDepth=16")
	}
	if normalized.Format == model.FormatJPEG {
		return errors.New("16-bit RAW 工作流不能直接输出 JPEG；请先明确选择 8-bit JPEG 兼容模式")
	}
	if len(images) == 0 || sheet.Width <= 0 || sheet.Height <= 0 {
		return errors.New("16-bit Sheet 尺寸或图片无效")
	}
	memoryLimit, tileSize := options.MemoryLimitBytes, options.TileSize
	if normalized.MemoryLimitBytes > 0 {
		memoryLimit = normalized.MemoryLimitBytes
	}
	if normalized.TileSize > 0 {
		tileSize = normalized.TileSize
	}
	canvas, err := NewCanvas16WithStorage(sheet.Width, sheet.Height, memoryLimit, tileSize, options.TempRoot, normalized.StorageMode)
	if err != nil {
		return fmt.Errorf("创建 16-bit 画布失败: %w", err)
	}
	defer canvas.Close()
	decodeOptions := raw.DecodeOptions{WhiteBalance: string(normalized.RawProcessing.WhiteBalance), Demosaic: string(normalized.RawProcessing.Demosaic), WorkingColorSpace: normalized.WorkingColorSpace}
	for i, source := range images {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("16-bit RAW 渲染已取消: %w", err)
		}
		decoded, profile, err := imaging.DecodeNormalizedFile16Context(ctx, source.SourcePath, decodeOptions)
		if err != nil {
			return fmt.Errorf("无法读取 %s: %w", source.Record.Filename, err)
		}
		if decoded.Bounds().Dx() != source.Record.DisplayWidth || decoded.Bounds().Dy() != source.Record.DisplayHeight {
			return fmt.Errorf("%s 解码后的显示尺寸为 %dx%d，与扫描记录 %dx%d 不一致", source.Record.Filename, decoded.Bounds().Dx(), decoded.Bounds().Dy(), source.Record.DisplayWidth, source.Record.DisplayHeight)
		}
		if source.Record.ICCProfileHash != metadata.ICCProfileHash(profile) && source.Record.ICCProfileHash != "" {
			return fmt.Errorf("%s 的 ICC Profile 发生变化，拒绝静默继续", source.Record.Filename)
		}
		if source.Record.Rotation == 90 {
			decoded = imaging.Rotate90CW16(decoded)
		}
		content := source.Record.ContentRect
		if decoded.Bounds().Dx() != content.Width || decoded.Bounds().Dy() != content.Height {
			return fmt.Errorf("%s 排版尺寸与解码尺寸不一致", source.Record.Filename)
		}
		outer := source.Record.OuterRect
		paddingX, paddingY := content.X-outer.X, content.Y-outer.Y
		if paddingX != paddingY || paddingX < 0 {
			return fmt.Errorf("%s 的 padding 坐标无效", source.Record.Filename)
		}
		if err := canvas.DrawImage16(outer.X, outer.Y, decoded, paddingX, normalized.PaddingMode); err != nil {
			return fmt.Errorf("绘制 %s 失败: %w", source.Record.Filename, err)
		}
		if options.Progress != nil {
			options.Progress(Progress{Completed: i + 1, Total: len(images), Current: source.Record.Filename})
		}
	}
	switch normalized.Format {
	case model.FormatPNG:
		return WritePNG16Atomic(filename, canvas, iccProfile)
	case model.FormatTIFF:
		return WriteTIFF16CanvasAtomic(filename, canvas, normalized.TIFFCompression, iccProfile)
	default:
		return errors.New("不支持的 16-bit Sheet 格式")
	}
}

// Keep image imported in this file's public dependency graph explicit for
// documentation tooling that discovers the 16-bit pixel type.
var _ image.Image = (*image.NRGBA64)(nil)
