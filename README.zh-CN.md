# PhotoSheet

PhotoSheet 是一款跨平台桌面工具：把多张照片组合为处理 Sheet，再依据项目元数据将处理后的 Sheet 恢复为单张图片。

**项目状态：Beta。** JPG/PNG/TIFF 核心工作流是当前公开基线。RAW/DNG 分为公开的纯 Go fallback 和需要单独审查的 native backend，见 docs/RAW_PIPELINE.md。

[English](README.md)

## 工作流

```text
照片
  ↓
PhotoSheet Pack
  ↓
处理 Sheet + project.json
  ↓
外部图像软件
  ↓
处理后的 Sheet
  ↓
PhotoSheet Restore
  ↓
单张图片
```

PhotoSheet 只读打开源文件，不修改、移动或删除原始照片。project.json 记录稳定的 Sheet identity、图片坐标、尺寸、方向和哈希；恢复时可以处理重命名、后缀变化以及不同目录中的处理结果。

## 功能

- 扫描 JPG/JPEG、PNG、TIFF。
- 读取 DNG 元数据；纯 Go fallback 支持受限的未压缩和 Deflate DNG。
- PNG/TIFF Sheet，以及真实 16-bit 工作流。
- 严格无损模式与明确分开的 JPEG 兼容模式。
- 确定性的 MaxRects 排版，可选 90° 旋转和 padding。
- project.json v3，并兼容旧项目的内存迁移。
- 按 canonical stem、明确后缀、尺寸和手动绑定解析处理后 Sheet。
- PNG/JPEG/TIFF 拆图，包含路径穿越和危险变换检查。
- CLI 与 Wails v2 + React/TypeScript GUI，共用 Go 应用层。
- 可选 updater：校验签名 manifest 和 SHA-256。

当前开发目录中可能存在的 LibRaw/Adobe DNG SDK/JPEG XL 原生二进制不会进入公开 core build，也被 Git 忽略。它们的许可证、notice、源码对应关系和再分发条件尚未完成审查；不要把它们称为公共发布物。

## Strict Lossless（严格无损）是什么意思

严格无损模式会把输入解码为 RGB 像素，并在 PNG 或无损 TIFF Sheet 的拼接、拆分过程中保持这些 RGB 像素；不会在这个往返阶段 resize、锐化或使用有损色彩转换。

这不可能恢复 JPEG 源文件已经丢失的信息。JPEG 输出一定会重新编码，Quality=100 也不是数学意义上的完全无损。RAW 也要区分两件事：demosaic/白平衡会把 sensor data 转换为工作 RGB，因此 RAW Source Integrity 不等于 RGB Pixel Integrity。

## RAW / DNG 边界

```text
DNG / RAW → probe → decode → demosaic → 白平衡 → 工作色彩空间 → 16-bit RGB → Sheet
```

公开 core fallback 只负责支持的未压缩/Deflate DNG 子集。JPEG-compressed DNG、Lossy JPEG DNG、JPEG XL DNG 和相机特定 RAW 变体需要经过许可证审查的 native backend。native backend 即使合法配置，也输出面向显示的 16-bit RGB，不会在 Sheet 工作流中保留原始 sensor structure。

详见 docs/RAW_PIPELINE.md、docs/RAW_BACKEND.md 和 THIRD_PARTY_NOTICES.md。

## 快速使用

1. 选择照片或照片文件夹。
2. 设置 Sheet 最大尺寸和输出格式。
3. 生成 Sheet 与 project.json。
4. 在外部图像软件中处理 Sheet。
5. 选择项目和处理后的 Sheet 文件/目录。
6. 查看自动匹配结果，手动处理歧义候选。
7. 拆出单张图片，并保留原文件作为备份。

最近使用的处理后位置保存在用户配置目录中，不写入 project.json。

## 构建核心项目

公开 core build 需要：

- Go 1.22 或更高版本；
- Node.js 22 LTS 或更高版本和 npm；
- Git。

在 fresh clone 中执行：

```bash
go mod download
go test ./...
go vet ./...

cd frontend
npm ci
npm run typecheck
npm run build
cd ..

mkdir dist
go build -trimpath -o dist/photosheet ./cmd/photosheet
go build -trimpath -o dist/photosheet-updater ./cmd/photosheet-updater
```

CLI 可用 photosheet --help 查看帮助。

### GUI 开发与构建

PhotoSheet 使用 Wails v2.10.2。GUI 需要目标平台的原生 WebView 和编译工具：

```bash
cd frontend
npm ci
npm run build:wails
cd ..
go test -tags wails ./cmd/photosheet-gui

cd cmd/photosheet-gui
go run github.com/wailsapp/wails/v2/cmd/wails@v2.10.2 dev
go run github.com/wailsapp/wails/v2/cmd/wails@v2.10.2 build -clean -tags "wails production"
```

平台要求见 BUILD_GUIDE.md。core build 不需要 LibRaw、Adobe DNG SDK、JPEG XL、签名证书或公证凭据。

### 可选 native RAW build

native backend 只允许显式启用，CI 不会下载它。只有在确认 SDK 允许目标源码/二进制分发，并准备好完整 notice 后，才可按 docs/RAW_BACKEND.md 放置本地 artifact 并使用 -tags photosheet_native。

```text
Windows x64：internal/raw/native/windows/libraw.dll
macOS arm64：internal/raw/native/darwin/arm64/libphotosheet_raw.dylib
macOS x86_64：internal/raw/native/darwin/amd64/libphotosheet_raw.dylib
```

这些路径被 Git 忽略。Linux 与 Windows ARM64 默认使用公开 fallback。

## 架构

```text
React/TypeScript 前端
          ↓ Wails bindings
Go 应用层
          ↓
Scanner · Packing · Renderer · Resolver · Extractor
          ↓
PNG/JPEG/TIFF codecs · project schema · 可选 RAW backend
```

前端负责 UI、交互和预览；Go 负责文件读取、排版、渲染、项目校验和拆图。本仓库是应用，不是稳定的公开 Go SDK；internal/ 下的包不是外部 API。

详见 docs/ARCHITECTURE.md 和 docs/PROJECT_FORMAT.md。

## 隐私与安全边界

- Pack/Restore 的图片处理在本地完成；只有 updater 需要网络。
- 源图片以只读方式打开。
- project.json 是数据，不会被当作脚本执行。
- resolver 会拒绝路径穿越、符号链接逃逸、伪造图片、重叠矩形和非等比例变换。
- 外部软件可能改变分辨率、位深、ICC、压缩方式或画布尺寸；恢复前请保留原图并查看警告。

不要在 Issue 中上传私人照片、RAW 或项目文件。优先提供诊断信息、元数据和最小化的程序生成 fixture；只有确认拥有分享权限时才提供样本。

## License

PhotoSheet 使用 Apache License 2.0，详见 LICENSE。第三方条款见 THIRD_PARTY_NOTICES.md，它们不等于 PhotoSheet 自身许可证。可选 native RAW 二进制仍不会进入仓库或 Release，直到其单独的再分发审查完成。

## 贡献与安全

请先阅读 CONTRIBUTING.md、SECURITY.md 和 CODE_OF_CONDUCT.md。

## 发布状态

tag 驱动的 workflow 当前只发布未签名的 core CLI/updater 和 SHA256SUMS。native RAW 二进制、installer、签名和公证要等许可证审查与平台验收完成后再决定，见 docs/RELEASE.md。
