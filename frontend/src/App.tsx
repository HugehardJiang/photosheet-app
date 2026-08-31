import { useEffect, useRef, useState, type ReactNode } from "react";
import {
  cancel,
  chooseDirectory,
  chooseFolder,
  chooseImages,
  chooseProcessedFiles,
  chooseProject,
  diagnostics,
  onProgress,
  pack,
  preview,
  recentProcessedLocations,
  rememberProcessedLocations,
  resolveProcessed,
  type OutputFormat,
  type Progress,
  type Settings,
  unpack,
} from "./bridge";

type Page = "home" | "pack" | "unpack" | "settings";
type Theme = "system" | "light" | "dark";

const defaultSettings: Settings = {
  maximumLongEdge: 16384,
  maximumShortEdge: 0,
  maximumTotalPixels: 0,
  mode: "strict-lossless",
  format: "png",
  padding: 0,
  paddingMode: "solid",
  allowRotation: false,
  tileSize: 512,
  jpegQuality: 95,
  workingBitDepth: 16,
  workingColorSpace: "sRGB",
  tiffCompression: "zip",
  storageMode: "low-disk",
  rawProcessing: { whiteBalance: "camera", demosaic: "balanced", autoExposure: false, autoContrast: false, sharpen: false, noiseReduction: false },
};

export function App() {
  const [page, setPage] = useState<Page>("home");
  const [theme, setTheme] = useState<Theme>("system");
  const [progress, setProgress] = useState<Progress | null>(null);
  const [error, setError] = useState("");
  const [rawCapabilities, setRawCapabilities] = useState<Awaited<ReturnType<typeof diagnostics>> | null>(null);
  const [recentLocations, setRecentLocations] = useState<string[]>([]);
  const cancelRequested = useRef(false);

  useEffect(() => onProgress((nextProgress) => {
    const terminal = nextProgress.status === "cancelled" || nextProgress.status === "completed" || nextProgress.status === "failed" || nextProgress.phase === "完成" || nextProgress.phase === "已取消";
    if (terminal) {
      cancelRequested.current = false;
      setProgress(null);
      return;
    }
    if (!cancelRequested.current) setProgress(nextProgress);
  }), []);

  useEffect(() => {
    void diagnostics().then(setRawCapabilities).catch(() => setRawCapabilities(null));
    void recentProcessedLocations().then(setRecentLocations).catch(() => setRecentLocations([]));
  }, []);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (!(event.ctrlKey || event.metaKey) || event.altKey) return;
      if (event.key === "1") { event.preventDefault(); setPage("pack"); }
      if (event.key === "2") { event.preventDefault(); setPage("unpack"); }
      if (event.key === ",") { event.preventDefault(); setPage("settings"); }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, []);

  const handleCancel = async () => {
    cancelRequested.current = true;
    setProgress(null);
    try {
      await cancel();
    } catch (cancelError) {
      cancelRequested.current = false;
      setError(String(cancelError));
    }
  };

  const rawReady = Boolean(rawCapabilities?.libRawEnabled && rawCapabilities?.DNGSDKEnabled && rawCapabilities?.JPEGXLDNGEnabled && rawCapabilities?.output16BitEnabled);
  const pageTitle = page === "home" ? "开始" : page === "pack" ? "智能拼接" : page === "unpack" ? "拆分修图结果" : "设置";

  return (
    <div className={`app theme-${theme}`}>
      <header className="app-toolbar">
        <button className="brand" onClick={() => { setPage("home"); setError(""); }} aria-label="返回开始中心">
          <span className="brand-mark">PS</span>
          <span className="brand-name">PhotoSheet</span>
        </button>
        <div className="toolbar-context"><span>工作台</span><i /> <strong>{pageTitle}</strong></div>
        <div className="toolbar-actions">
          <span className={`toolbar-status ${rawReady ? "is-ready" : ""}`}>RAW {rawCapabilities ? (rawReady ? "✓" : "有限") : "—"}</span>
          <button className="toolbar-icon" type="button" title="快捷键：Ctrl/Cmd + 1 拼接，2 拆分，, 设置" onClick={() => setPage("settings")}>?</button>
          <button className="toolbar-icon" type="button" title="打开设置" onClick={() => setPage("settings")}>⚙</button>
          <select className="theme-select" aria-label="主题" value={theme} onChange={(event) => setTheme(event.target.value as Theme)}>
            <option value="system">系统主题</option>
            <option value="light">浅色</option>
            <option value="dark">深色</option>
          </select>
        </div>
      </header>

      <div className="shell-main">
        <Sidebar page={page} onNavigate={(nextPage) => { setPage(nextPage); setError(""); }} />
        <main className="shell-workspace">
          {page === "home" && <Home onNavigate={setPage} capabilities={rawCapabilities} recentLocations={recentLocations} />}
          {page === "pack" && <PackPage onError={setError} onProgress={setProgress} />}
          {page === "unpack" && <UnpackPage onError={setError} onProgress={setProgress} />}
          {page === "settings" && <SettingsPage capabilities={rawCapabilities} theme={theme} onThemeChange={setTheme} />}
          {error && <div className="error-banner" role="alert"><span>{error}</span><button onClick={() => setError("")} aria-label="关闭错误">×</button></div>}
        </main>
      </div>

      <StatusBar progress={progress} rawReady={rawReady} onCancel={() => void handleCancel()} />
    </div>
  );
}

function Sidebar({ page, onNavigate }: { page: Page; onNavigate: (page: Page) => void }) {
  return <aside className="sidebar">
    <div className="sidebar-section">
      <p className="sidebar-label">工作</p>
      <NavItem active={page === "home"} icon="⌂" label="开始" onClick={() => onNavigate("home")} />
      <NavItem active={page === "pack"} icon="＋" label="智能拼接" shortcut="⌘1" onClick={() => onNavigate("pack")} />
      <NavItem active={page === "unpack"} icon="↙" label="拆分结果" shortcut="⌘2" onClick={() => onNavigate("unpack")} />
    </div>
    <div className="sidebar-section sidebar-projects">
      <p className="sidebar-label">项目</p>
      <NavItem active={false} icon="▤" label="最近位置" onClick={() => onNavigate("home")} />
    </div>
    <div className="sidebar-spacer" />
    <div className="sidebar-section sidebar-system">
      <p className="sidebar-label">系统</p>
      <NavItem active={page === "settings"} icon="⚙" label="设置" shortcut="⌘," onClick={() => onNavigate("settings")} />
      <div className="sidebar-footnote"><span className="status-led" />源文件只读</div>
    </div>
  </aside>;
}

function NavItem({ active, icon, label, shortcut, onClick }: { active: boolean; icon: string; label: string; shortcut?: string; onClick: () => void }) {
  return <button className={`nav-item ${active ? "active" : ""}`} onClick={onClick}>
    <span className="nav-icon">{icon}</span><span>{label}</span>{shortcut && <kbd>{shortcut}</kbd>}
  </button>;
}

function Home({ onNavigate, capabilities, recentLocations }: { onNavigate: (page: Page) => void; capabilities: Awaited<ReturnType<typeof diagnostics>> | null; recentLocations: string[] }) {
  const rawReady = Boolean(capabilities?.libRawEnabled && capabilities?.DNGSDKEnabled && capabilities?.JPEGXLDNGEnabled && capabilities?.output16BitEnabled);
  return <section className="workspace-view start-center">
    <WorkspaceHeading eyebrow="开始中心" title="开始" trailing={<span className="shortcut-hint">Ctrl/Cmd + 1 拼接　·　2 拆分　·　, 设置</span>} />
    <div className="start-grid">
      <section className="start-panel command-panel">
        <PanelHeading title="工作入口" meta="选择一个工作区开始" />
        <button className="command-row primary" onClick={() => onNavigate("pack")}>
          <span className="command-icon">＋</span><span><strong>新建拼接任务</strong><small>选择照片，生成 Sheet 与 project.json</small></span><b>⌘1</b>
        </button>
        <button className="command-row" onClick={() => onNavigate("unpack")}>
          <span className="command-icon">↙</span><span><strong>拆分修图结果</strong><small>根据 project.json 恢复单张照片</small></span><b>⌘2</b>
        </button>
        <div className="start-rule" />
        <div className="principles"><span><i className="dot green" />源文件只读</span><span><i className="dot blue" />坐标可验证</span><span><i className="dot amber" />JPEG 重编码会提示</span></div>
      </section>
      <section className="start-panel recent-panel">
        <PanelHeading title="最近位置" meta="处理后的 Sheet 来源" />
        {recentLocations.length ? <div className="recent-location-list">{recentLocations.slice(0, 5).map((location) => <div className="recent-location" key={location}><span className="file-glyph">▤</span><span title={location}>{location}</span></div>)}</div> : <div className="empty-state"><span className="empty-glyph">○</span><span>还没有最近位置</span><small>在拆分工作区添加 Sheet 文件夹后会显示在这里。</small></div>}
        <div className="engine-strip"><span className={`status-led ${rawReady ? "ready" : ""}`} /><span>RAW 引擎</span><strong>{capabilities ? (rawReady ? "正常" : "有限能力") : "检测中"}</strong><button onClick={() => onNavigate("settings")}>查看诊断</button></div>
      </section>
    </div>
  </section>;
}

function SettingsPage({ capabilities, theme, onThemeChange }: { capabilities: Awaited<ReturnType<typeof diagnostics>> | null; theme: Theme; onThemeChange: (theme: Theme) => void }) {
  const rawRows = capabilities ? [
    ["LibRaw", capabilities.libRawEnabled],
    ["Adobe DNG SDK", capabilities.DNGSDKEnabled],
    ["JPEG XL DNG", capabilities.JPEGXLDNGEnabled],
    ["16-bit RGB", capabilities.output16BitEnabled],
    ["DNG Opcode", capabilities.DNGOpcodeProcessing],
  ] as const : [];
  return <section className="workspace-view settings-view">
    <WorkspaceHeading eyebrow="系统" title="设置" trailing={<span className="shortcut-hint">应用级选项</span>} />
    <div className="settings-layout">
      <aside className="inspector-panel settings-nav-panel">
        <div className="inspector-scroll compact-scroll"><div className="settings-nav active">常规</div><div className="settings-nav">RAW / Diagnostics</div><div className="settings-nav">快捷键</div></div>
      </aside>
      <section className="content-panel settings-content">
        <PanelHeading title="常规" meta="窗口与界面" />
        <div className="settings-form-row"><div><strong>界面主题</strong><small>保持桌面工具的低干扰显示</small></div><select value={theme} onChange={(event) => onThemeChange(event.target.value as Theme)}><option value="system">跟随系统</option><option value="light">浅色</option><option value="dark">深色</option></select></div>
        <div className="settings-divider" />
        <PanelHeading title="RAW / Diagnostics" meta={capabilities ? "启动时检测" : "正在检测"} />
        <div className="diagnostic-table">{rawRows.map(([label, enabled]) => <div className="diagnostic-row" key={label}><span>{label}</span><strong className={enabled ? "ok" : "muted"}>{enabled ? "enabled" : "disabled"}</strong></div>)}</div>
        {!capabilities && <div className="empty-inline">桌面桥接尚未返回诊断信息。</div>}
        {capabilities && !rawRows.every(([, enabled]) => enabled) && <p className="warning-note">当前构建仍可识别部分 RAW/DNG，但压缩 DNG 可能无法解码；不会把识别结果伪装成完整支持。</p>}
        <div className="settings-divider" />
        <PanelHeading title="键盘" meta="快速进入工作区" />
        <div className="shortcut-table"><div><kbd>Ctrl/Cmd + 1</kbd><span>智能拼接</span></div><div><kbd>Ctrl/Cmd + 2</kbd><span>拆分修图结果</span></div><div><kbd>Ctrl/Cmd + ,</kbd><span>设置</span></div></div>
      </section>
    </div>
  </section>;
}

function PackPage({ onError, onProgress }: { onError: (message: string) => void; onProgress: (progress: Progress | null) => void }) {
  const [settings, setSettings] = useState<Settings>(defaultSettings);
  const [inputs, setInputs] = useState<string[]>([]);
  const [outputDir, setOutputDir] = useState("");
  const [skipInvalid, setSkipInvalid] = useState(true);
  const [result, setResult] = useState<string[]>([]);
  const [layout, setLayout] = useState<Awaited<ReturnType<typeof preview>> | null>(null);

  useEffect(() => {
    let active = true;
    if (!inputs.length) { setLayout(null); return () => { active = false; }; }
    void preview({ inputs, outputDir: "", settings, skipInvalid }).then((response) => { if (active) setLayout(response); }).catch(() => { if (active) setLayout(null); });
    return () => { active = false; };
  }, [inputs, settings, skipInvalid]);

  const update = <K extends keyof Settings>(key: K, value: Settings[K]) => setSettings((current) => ({ ...current, [key]: value }));
  const selectImages = async () => { const selected = await chooseImages(); if (selected.length) setInputs(selected); };
  const selectFolder = async () => { const selected = await chooseFolder(); if (selected) setInputs([selected]); };
  const selectOutput = async () => { const selected = await chooseDirectory("选择项目输出目录"); if (selected) setOutputDir(selected); };
  const start = async () => {
    onError("");
    if (!inputs.length || !outputDir) { onError("请先选择照片或文件夹，并指定项目输出目录。"); return; }
    try {
      const response = await pack({ inputs, outputDir, settings, skipInvalid });
      setResult(response.warnings);
      onProgress({ status: "completed", phase: "完成", processed: 1, total: 1, current: response.projectPath, message: `已生成 ${response.project.sheets.length} 个 Sheet` });
    } catch (error) { onError(String(error)); }
  };

  return <section className="workspace-view workspace-tool">
    <WorkspaceHeading eyebrow="工作 / 拼接" title="智能拼接" trailing={<div className="heading-actions"><span className="info-badge">{inputs.length ? `${inputs.length} 个输入` : "未选择输入"}</span><button className="primary-button" onClick={() => void start()}>开始生成 <span>⌘↵</span></button></div>} />
    <div className="tool-layout">
      <aside className="inspector-panel">
        <div className="panel-heading inspector-heading"><strong>设置</strong><span>输入与输出</span></div>
        <div className="inspector-scroll">
          <SettingGroup title="输入照片"><div className="input-summary">{inputs.length ? `${inputs.length} 个文件 / 文件夹已选择` : "尚未选择照片"}</div><div className="button-row"><button className="small-button" onClick={() => void selectImages()}>选择文件</button><button className="small-button" onClick={() => void selectFolder()}>选择文件夹</button></div></SettingGroup>
          <SettingGroup title="画布限制"><label className="field-label">最大长边 <NumberInput value={settings.maximumLongEdge} onChange={(value) => update("maximumLongEdge", value)} /> <span>px</span></label><label className="field-label">最大短边 <NumberInput value={settings.maximumShortEdge} placeholder="自动" onChange={(value) => update("maximumShortEdge", value)} /> <span>px</span></label><label className="field-label">最大总像素 <NumberInput value={settings.maximumTotalPixels} placeholder="不限制" onChange={(value) => update("maximumTotalPixels", value)} /> <span>px²</span></label></SettingGroup>
          <SettingGroup title="输出方式"><div className="segmented"><button className={settings.mode === "strict-lossless" ? "active" : ""} onClick={() => { update("mode", "strict-lossless"); update("format", "png"); }}>严格无损</button><button className={settings.mode === "jpeg-compatibility" ? "active" : ""} onClick={() => update("mode", "jpeg-compatibility")}>JPEG 兼容</button></div>{settings.mode === "jpeg-compatibility" && <p className="warning-note">JPEG 会重新编码，不是数学意义上的完全无损。</p>}<label className="field-label">Sheet 格式 <select value={settings.format} onChange={(event) => update("format", event.target.value as Settings["format"])}><option value="png">PNG 16-bit</option><option value="tiff">TIFF 16-bit ZIP</option><option value="jpg" disabled={settings.mode !== "jpeg-compatibility"}>JPG</option></select></label><label className="field-label">工作位深 <select value={settings.workingBitDepth} onChange={(event) => update("workingBitDepth", Number(event.target.value))}><option value={16}>16-bit</option><option value={8}>8-bit</option></select></label></SettingGroup>
          {layout?.hasRaw && <SettingGroup title="RAW 处理"><p className="setting-hint">RAW 先显影为高位深 RGB；RAW 压缩无损不等同于 RGB 结构无损。</p><label className="field-label">白平衡 <select value={settings.rawProcessing.whiteBalance} onChange={(event) => update("rawProcessing", { ...settings.rawProcessing, whiteBalance: event.target.value as Settings["rawProcessing"]["whiteBalance"] })}><option value="camera">相机白平衡</option><option value="neutral">中性</option><option value="custom">自定义</option></select></label><label className="field-label">Demosaic <select value={settings.rawProcessing.demosaic} onChange={(event) => update("rawProcessing", { ...settings.rawProcessing, demosaic: event.target.value as Settings["rawProcessing"]["demosaic"] })}><option value="high">高质量</option><option value="balanced">平衡</option><option value="fast">快速</option></select></label><label className="field-label">工作色彩空间 <select value={settings.workingColorSpace} onChange={(event) => update("workingColorSpace", event.target.value)}><option value="sRGB">sRGB</option><option value="Adobe RGB">Adobe RGB</option><option value="Display P3">Display P3</option><option value="ProPhoto RGB">ProPhoto RGB</option></select></label><label className="field-label">存储策略 <select value={settings.storageMode} onChange={(event) => update("storageMode", event.target.value as Settings["storageMode"])}><option value="low-disk">节省硬盘空间</option><option value="balanced">平衡</option><option value="performance">性能优先</option></select></label></SettingGroup>}
          <SettingGroup title="排版细节"><label className="field-label">Padding <select value={settings.padding} onChange={(event) => update("padding", Number(event.target.value))}>{[0, 8, 16, 32, 64].map((value) => <option key={value} value={value}>{value} px</option>)}</select></label><label className="check-row"><input type="checkbox" checked={settings.paddingMode === "edge-extension"} onChange={(event) => update("paddingMode", event.target.checked ? "edge-extension" : "solid")} /> 边缘像素延展</label><label className="check-row"><input type="checkbox" checked={settings.allowRotation} onChange={(event) => update("allowRotation", event.target.checked)} /> 允许旋转 90°</label><label className="check-row"><input type="checkbox" checked={skipInvalid} onChange={(event) => setSkipInvalid(event.target.checked)} /> 跳过无效文件并记录日志</label></SettingGroup>
          <SettingGroup title="项目位置"><FilePickerRow path={outputDir} placeholder="尚未指定" buttonLabel="选择" onSelect={selectOutput} /></SettingGroup>
          {layout && <ScanSummary summary={layout.scanSummary} />}{result.map((warning) => <p className="result-note" key={warning}>{warning}</p>)}
        </div>
        <div className="inspector-footer"><div><span>预计</span><strong>{layout ? `${layout.sheets.length} 个 Sheet` : "—"}</strong></div><button className="primary-button full" onClick={() => void start()}>开始生成 <span>→</span></button></div>
      </aside>
      <LayoutPreview settings={settings} inputCount={inputs.length} layout={layout} />
    </div>
  </section>;
}

function ScanSummary({ summary }: { summary: { found: number; successful: number; unsupported: number; corrupted: number; skipped: number } }) {
  return <div className="scan-summary" aria-label="扫描结果"><span>发现<strong>{summary.found}</strong></span><span>成功<strong>{summary.successful}</strong></span><span>不支持<strong>{summary.unsupported}</strong></span><span>损坏<strong>{summary.corrupted}</strong></span><span>跳过<strong>{summary.skipped}</strong></span></div>;
}

function UnpackPage({ onError, onProgress }: { onError: (message: string) => void; onProgress: (progress: Progress | null) => void }) {
  const [projectPath, setProjectPath] = useState("");
  const [processedDirs, setProcessedDirs] = useState<string[]>([]);
  const [processedFiles, setProcessedFiles] = useState<string[]>([]);
  const [recursive, setRecursive] = useState(true);
  const [bindings, setBindings] = useState<Record<string, string>>({});
  const [resolution, setResolution] = useState<Awaited<ReturnType<typeof resolveProcessed>> | null>(null);
  const [recentLocations, setRecentLocations] = useState<string[]>([]);
  const [resolving, setResolving] = useState(false);
  const [outputDir, setOutputDir] = useState("");
  const [allowScale, setAllowScale] = useState(false);
  const [format, setFormat] = useState<OutputFormat>("png");
  const [message, setMessage] = useState("");

  useEffect(() => { void recentProcessedLocations().then(setRecentLocations); }, []);
  useEffect(() => {
    let active = true;
    if (!projectPath || (!processedDirs.length && !processedFiles.length && !Object.keys(bindings).length)) { setResolution(null); return () => { active = false; }; }
    setResolving(true);
    void resolveProcessed({ projectPath, processedDirs, processedFiles, recursive, bindings }).then((value) => { if (active) setResolution(value); }).catch((error) => { if (active) onError(String(error)); }).finally(() => { if (active) setResolving(false); });
    return () => { active = false; };
  }, [projectPath, processedDirs, processedFiles, recursive, bindings, onError]);

  const selectProject = async () => { const selected = await chooseProject(); if (selected) { setProjectPath(selected); setBindings({}); } };
  const selectProcessedDir = async () => { const selected = await chooseDirectory("选择处理后的 Sheet 文件夹"); if (selected && !processedDirs.includes(selected)) setProcessedDirs((current) => [...current, selected]); };
  const selectProcessedFiles = async () => { const selected = await chooseProcessedFiles(); if (selected.length) setProcessedFiles((current) => Array.from(new Set([...current, ...selected]))); };
  const addRecentLocation = (location: string) => { if (!processedDirs.includes(location) && !processedFiles.includes(location)) setProcessedDirs((current) => [...current, location]); };
  const chooseBinding = async (sheetId: string) => { const selected = await chooseProcessedFiles(); if (selected[0]) setBindings((current) => ({ ...current, [sheetId]: selected[0] })); };
  const selectOutput = async () => { const selected = await chooseDirectory("选择恢复照片输出目录"); if (selected) setOutputDir(selected); };
  const start = async () => {
    onError("");
    if (!projectPath) { onError("请选择 project.json。"); return; }
    try {
      await rememberProcessedLocations([...processedDirs, ...processedFiles]);
      const response = await unpack({ projectPath, processedDirs, processedFiles, recursive, bindings, outputDir, allowProportionalScale: allowScale, outputFormat: format, jpegQuality: 95 });
      setMessage(`已恢复 ${response.report.restoredFiles.length} 张照片`);
      onProgress({ status: "completed", phase: "完成", processed: 1, total: 1, current: outputDir, message: "拆分完成" });
    } catch (error) { onError(String(error)); }
  };

  const matchedCount = resolution?.sheets.filter((sheet) => sheet.status === "matched").length ?? 0;
  const attentionCount = resolution?.sheets.filter((sheet) => sheet.status !== "matched").length ?? 0;
  return <section className="workspace-view workspace-tool">
    <WorkspaceHeading eyebrow="工作 / 拆分" title="拆分修图结果" trailing={<div className="heading-actions"><span className="info-badge">{resolution ? `${matchedCount} 已匹配` : "等待匹配"}</span><button className="primary-button" onClick={() => void start()}>开始拆分 <span>⌘↵</span></button></div>} />
    <div className="tool-layout unpack-layout">
      <aside className="inspector-panel">
        <div className="panel-heading inspector-heading"><strong>工程</strong><span>来源与恢复</span></div>
        <div className="inspector-scroll">
          <SettingGroup title="项目文件"><FilePickerRow path={projectPath} placeholder="请选择 project.json" buttonLabel="选择" onSelect={selectProject} /></SettingGroup>
          <SettingGroup title="处理后的 Sheet"><div className="action-group source-actions"><button className="small-button" onClick={() => void selectProcessedDir()}>添加文件夹</button><button className="small-button" onClick={() => void selectProcessedFiles()}>添加文件</button></div><label className="check-row compact-check"><input type="checkbox" checked={recursive} onChange={(event) => setRecursive(event.target.checked)} /> 递归扫描</label><div className="source-list">{processedDirs.map((path) => <PathChip key={`dir-${path}`} kind="文件夹" path={path} onRemove={() => setProcessedDirs((current) => current.filter((item) => item !== path))} />)}{processedFiles.map((path) => <PathChip key={`file-${path}`} kind="文件" path={path} onRemove={() => setProcessedFiles((current) => current.filter((item) => item !== path))} />)}{!processedDirs.length && !processedFiles.length && <div className="path-field">未添加来源</div>}</div>{recentLocations.length > 0 && <div className="recent-row"><span>最近位置</span><select defaultValue="" onChange={(event) => { if (event.target.value) addRecentLocation(event.target.value); }}><option value="">选择位置</option>{recentLocations.map((location) => <option key={location} value={location}>{location}</option>)}</select></div>}</SettingGroup>
          <SettingGroup title="恢复位置"><FilePickerRow path={outputDir} placeholder="默认使用项目旁的 restored" buttonLabel="选择" onSelect={selectOutput} /></SettingGroup>
          <SettingGroup title="恢复选项"><label className="check-row"><input type="checkbox" checked={allowScale} onChange={(event) => setAllowScale(event.target.checked)} /> 确认等比例缩放后恢复</label><label className="field-label">恢复格式 <select value={format} onChange={(event) => setFormat(event.target.value as OutputFormat)}><option value="png">PNG（默认）</option><option value="same">与 Sheet 相同</option><option value="tiff">TIFF（无损）</option><option value="jpg">JPG（重新编码）</option></select></label></SettingGroup>
        </div>
        <div className="inspector-footer"><div><span>状态</span><strong>{message || (resolution ? `${attentionCount} 项需处理` : "未开始")}</strong></div><button className="primary-button full" onClick={() => void start()}>开始拆分 <span>→</span></button></div>
      </aside>
      <section className="content-panel matching-panel">
        <div className="panel-heading"><strong>匹配结果</strong><span>{resolving ? "正在读取候选文件…" : resolution ? `${resolution.sheets.length} 个 Sheet` : "选择来源后自动解析"}</span></div>
        <div className="match-table-head"><span>Sheet</span><span>状态</span><span>候选 / 操作</span></div>
        <div className="match-list">{resolving && <div className="match-empty">正在读取候选文件…</div>}{!resolving && !resolution && <div className="match-empty large"><span className="empty-glyph">⌁</span><strong>等待匹配</strong><small>选择 project.json 与处理后的 Sheet 来源后，匹配表会显示每个 Sheet 的状态。</small></div>}{resolution?.sheets.map((sheet) => <div className={`match-row status-${sheet.status}`} key={sheet.sheetId}><div className="match-index">{String(sheet.index + 1).padStart(2, "0")}</div><div className="match-main"><strong>{sheet.canonicalStem || sheet.sheetId}</strong><small>{sheet.path || sheet.reason || "未找到候选文件"}</small></div><div className="match-state"><span className="state-dot" />{statusLabel(sheet.status)}</div>{sheet.status === "ambiguous" && <button className="small-button" onClick={() => void chooseBinding(sheet.sheetId)}>手动选择</button>}</div>)}</div>
        <div className="matching-footer"><span>{resolution ? `${resolution.sheets.length} Sheet` : "—"}</span><span className="match-good">{matchedCount} 已匹配</span><span className="match-attention">{attentionCount} 需处理</span></div>
      </section>
    </div>
  </section>;
}

function statusLabel(status: string): string {
  switch (status) { case "matched": return "已匹配"; case "ambiguous": return "候选不唯一"; case "missing": return "缺失"; case "invalid": return "文件无效"; case "uniform-scaled": return "等比例缩放"; case "unsafe-transform": return "非等比例形变"; default: return status || "未解析"; }
}

function WorkspaceHeading({ eyebrow, title, trailing }: { eyebrow: string; title: string; trailing?: ReactNode }) {
  return <div className="workspace-heading"><div><span className="title-kicker">{eyebrow}</span><h1>{title}</h1></div>{trailing}</div>;
}

function PanelHeading({ title, meta }: { title: string; meta?: string }) {
  return <div className="panel-heading"><strong>{title}</strong>{meta && <span>{meta}</span>}</div>;
}

function SettingGroup({ title, children }: { title: string; children: ReactNode }) {
  return <section className="setting-group"><h3>{title}</h3>{children}</section>;
}

function FilePickerRow({ path, placeholder, buttonLabel, onSelect }: { path: string; placeholder: string; buttonLabel: string; onSelect: () => void | Promise<void> }) {
  const displayPath = path || placeholder;
  return <div className="file-picker-row"><div className="path-field" title={displayPath}>{displayPath}</div><button className="small-button" onClick={() => void onSelect()}>{buttonLabel}</button></div>;
}

function PathChip({ kind, path, onRemove }: { kind: string; path: string; onRemove: () => void }) {
  return <div className="source-chip"><span className="source-chip-kind">{kind}</span><span className="source-chip-path" title={path}>{path}</span><button aria-label={`移除${kind}来源`} onClick={onRemove}>×</button></div>;
}

function NumberInput({ value, placeholder, onChange }: { value: number; placeholder?: string; onChange: (value: number) => void }) {
  return <input className="number-input" type="number" min={0} value={value || ""} placeholder={placeholder} onChange={(event) => onChange(Number(event.target.value || 0))} />;
}

function LayoutPreview({ settings, inputCount, layout }: { settings: Settings; inputCount: number; layout: Awaited<ReturnType<typeof preview>> | null }) {
  const [zoom, setZoom] = useState(100);
  const sheet = layout?.sheets[0];
  return <section className="content-panel preview-panel"><div className="preview-toolbar"><div><strong>布局预览</strong><span>{layout ? `${layout.sheets.length} 个 Sheet` : inputCount ? "正在计算" : "等待输入"}</span></div><div className="preview-tools"><button onClick={() => setZoom((current) => Math.max(50, current - 10))}>−</button><button className="zoom-readout" onClick={() => setZoom(100)}>{zoom}%</button><button onClick={() => setZoom((current) => Math.min(150, current + 10))}>＋</button><button onClick={() => setZoom(100)}>适合</button></div></div><div className="preview-viewport"><div className="preview-stage"><div className="preview-sheet" style={{ aspectRatio: sheet ? `${sheet.width} / ${sheet.height}` : "1.35", transform: `scale(${zoom / 100})` }}>{sheet ? sheet.images.map((image, index) => <div key={`${image.filename}-${index}`} className="layout-rect" title={`${image.filename} · ${image.rect.width}×${image.rect.height}`} style={{ left: `${image.rect.x / sheet.width * 100}%`, top: `${image.rect.y / sheet.height * 100}%`, width: `${image.rect.width / sheet.width * 100}%`, height: `${image.rect.height / sheet.height * 100}%` }} />) : <><div className="preview-photo photo-a">{inputCount > 0 && "预览"}</div><div className="preview-photo photo-b" /><div className="preview-photo photo-c" /></>}</div><p className="preview-caption">矩形来自 Go 排版结果；缩略图仅用于界面预览。</p></div></div><div className="preview-stats"><div><span>模式</span><strong>{settings.mode === "strict-lossless" ? "严格无损" : "JPEG 兼容"}</strong></div><div><span>最大长边</span><strong>{settings.maximumLongEdge.toLocaleString()} px</strong></div><div><span>Padding</span><strong>{settings.padding} px</strong></div><div><span>旋转</span><strong>{settings.allowRotation ? "允许 90°" : "关闭"}</strong></div></div></section>;
}

function StatusBar({ progress, rawReady, onCancel }: { progress: Progress | null; rawReady: boolean; onCancel: () => void }) {
  const percent = progress?.total ? Math.round(progress.processed / progress.total * 100) : progress ? 8 : 0;
  return <footer className="status-bar"><div className="status-left"><span className={`status-led ${progress ? "busy" : "ready"}`} /><strong>{progress ? progress.phase : "READY"}</strong><span className="status-separator">|</span><span>{progress ? (progress.message || progress.current || "处理中") : "PhotoSheet 本地工作区"}</span></div>{progress ? <div className="status-progress"><div className="progress-track"><div style={{ width: `${percent}%` }} /></div><span>{percent}%</span><button className="status-cancel" onClick={onCancel}>取消</button></div> : <div className="status-right"><span>RAW {rawReady ? "✓" : "—"}</span><span>16-bit</span><span>源文件只读</span></div>}</footer>;
}
