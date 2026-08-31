export type Mode = "strict-lossless" | "jpeg-compatibility";
export type SheetFormat = "png" | "tiff" | "jpg";
export type OutputFormat = "png" | "jpg" | "same" | "tiff";

export interface Settings {
  maximumLongEdge: number;
  maximumShortEdge: number;
  maximumTotalPixels: number;
  mode: Mode;
  format: SheetFormat;
  padding: number;
  paddingMode: "solid" | "edge-extension";
  allowRotation: boolean;
  memoryLimitBytes?: number;
  tileSize: number;
  jpegQuality: number;
  workingBitDepth: number;
  workingColorSpace: string;
  tiffCompression: "zip" | "lzw" | "none";
  storageMode: "low-disk" | "balanced" | "performance";
  rawProcessing: { whiteBalance: "camera" | "neutral" | "custom"; demosaic: "fast" | "balanced" | "high"; autoExposure: boolean; autoContrast: boolean; sharpen: boolean; noiseReduction: boolean };
}

export interface Progress {
  status: string;
  phase: string;
  processed: number;
  total: number;
  current: string;
  message: string;
}

export interface PackRequest {
  inputs: string[];
  outputDir: string;
  settings: Settings;
  skipInvalid: boolean;
}

export interface PackResponse {
  projectPath: string;
  outputDir: string;
  warnings: string[];
  scanSummary: ScanSummary;
  project: { sheets: Array<{ file: string; width: number; height: number; images: unknown[] }> };
}

export interface PreviewResponse {
  sheets: Array<{ width: number; height: number; images: Array<{ filename: string; rect: { x: number; y: number; width: number; height: number }; rotation: number }> }>;
  warnings: string[];
  hasRaw: boolean;
  scanSummary: ScanSummary;
}

export interface ScanSummary {
  found: number;
  successful: number;
  unsupported: number;
  corrupted: number;
  skipped: number;
  cancelled: number;
}

export interface BackendInfo {
  name: string;
  version: string;
  available: boolean;
  error?: string;
  capabilities: {
    probeSupported: boolean;
    decodeSupported: boolean;
    previewSupported: boolean;
    supports16BitRGB: boolean;
    supportsJPEGXL: boolean;
    DNGSDKEnabled: boolean;
    DNGOpcodeProcessing: boolean;
    outputColorSpace: string;
  };
}

export interface CapabilityReport {
  libRawEnabled: boolean;
  DNGSDKEnabled: boolean;
  JPEGXLDNGEnabled: boolean;
  output16BitEnabled: boolean;
  DNGOpcodeProcessing: boolean;
  backends: BackendInfo[];
}

export interface UnpackResponse {
  report: { restoredFiles: string[]; warnings: string[]; usedProportionalScale: boolean; outputFormat: string; lossyReencode: boolean };
}

export interface CandidateMatch {
  path: string;
  sheetId: string;
  status: string;
  score: number;
  reasons: string[];
  encodedWidth: number;
  encodedHeight: number;
  displayWidth: number;
  displayHeight: number;
  format: string;
  modifiedAt: string;
  dimensionStatus: string;
  invalidReason?: string;
}

export interface ResolvedSheet {
  sheetId: string;
  index: number;
  canonicalStem: string;
  status: string;
  dimensionStatus: string;
  path: string;
  reason: string;
  scaleX: number;
  scaleY: number;
  manual: boolean;
  candidate?: CandidateMatch;
  candidates: CandidateMatch[];
}

export interface ResolveResponse {
  projectPath: string;
  sheets: ResolvedSheet[];
  warnings: string[];
}

type GUIApp = {
  Diagnostics(): Promise<CapabilityReport>;
  Preview(request: PackRequest): Promise<PreviewResponse>;
  Pack(request: PackRequest): Promise<PackResponse>;
  Unpack(request: { projectPath: string; sheetsDir?: string; processedDirs: string[]; processedFiles: string[]; recursive: boolean; bindings: Record<string, string>; outputDir: string; allowProportionalScale: boolean; outputFormat: OutputFormat; jpegQuality: number }): Promise<UnpackResponse>;
  Resolve(request: { projectPath: string; sheetsDir?: string; processedDirs: string[]; processedFiles: string[]; recursive: boolean; bindings: Record<string, string> }): Promise<ResolveResponse>;
  Verify(request: { projectPath: string; sheetsDir: string; checkFiles: boolean }): Promise<void>;
  Cancel(): Promise<void>;
  ChooseImages(): Promise<string[]>;
  ChooseProcessedFiles(): Promise<string[]>;
  ChooseProject(): Promise<string>;
  ChooseFolder(): Promise<string>;
  ChooseDirectory(title: string): Promise<string>;
  RecentProcessedLocations(): Promise<string[]>;
  RememberProcessedLocations(locations: string[]): Promise<void>;
};

declare global {
  interface Window {
    go?: { main?: { GUIApp?: GUIApp } };
    runtime?: { EventsOn?: (name: string, callback: (event: Progress) => void) => () => void };
  }
}

const getGUI = (): GUIApp | undefined => window.go?.main?.GUIApp;

type UnknownRecord = Record<string, unknown>;

function record(value: unknown): UnknownRecord {
  return typeof value === "object" && value !== null ? value as UnknownRecord : {};
}

function first(value: UnknownRecord, ...keys: string[]): unknown {
  for (const key of keys) {
    if (value[key] !== undefined && value[key] !== null) return value[key];
  }
  return undefined;
}

function stringValue(value: unknown): string {
  return typeof value === "string" ? value : value == null ? "" : String(value);
}

function numberValue(value: unknown): number {
  return typeof value === "number" && Number.isFinite(value) ? value : 0;
}

function normalizeProgress(value: unknown): Progress {
  const raw = record(value);
  return {
    status: stringValue(first(raw, "status", "Status")) || "running",
    phase: stringValue(first(raw, "phase", "Phase")),
    processed: numberValue(first(raw, "processed", "Processed")),
    total: numberValue(first(raw, "total", "Total")),
    current: stringValue(first(raw, "current", "Current")),
    message: stringValue(first(raw, "message", "Message")),
  };
}

function normalizePackResponse(value: unknown): PackResponse {
  const raw = record(value);
  const project = record(first(raw, "project", "Project"));
  const sheets = first(project, "sheets", "Sheets");
  return {
    projectPath: stringValue(first(raw, "projectPath", "ProjectPath")),
    outputDir: stringValue(first(raw, "outputDir", "OutputDir")),
    warnings: Array.isArray(first(raw, "warnings", "Warnings"))
      ? (first(raw, "warnings", "Warnings") as unknown[]).map(stringValue)
      : [],
    scanSummary: normalizeScanSummary(first(raw, "scanSummary", "ScanSummary")),
    project: {
      sheets: Array.isArray(sheets) ? sheets as PackResponse["project"]["sheets"] : [],
    },
  };
}

function normalizeScanSummary(value: unknown): ScanSummary {
  const raw = record(value);
  return {
    found: numberValue(first(raw, "found", "Found")),
    successful: numberValue(first(raw, "successful", "Successful")),
    unsupported: numberValue(first(raw, "unsupported", "Unsupported")),
    corrupted: numberValue(first(raw, "corrupted", "Corrupted")),
    skipped: numberValue(first(raw, "skipped", "Skipped")),
    cancelled: numberValue(first(raw, "cancelled", "Cancelled")),
  };
}

function normalizeUnpackResponse(value: unknown): UnpackResponse {
  const raw = record(value);
  const report = record(first(raw, "report", "Report"));
  const restoredFiles = first(report, "restoredFiles", "RestoredFiles");
  const warnings = first(report, "warnings", "Warnings");
  return {
    report: {
      restoredFiles: Array.isArray(restoredFiles) ? restoredFiles.map(stringValue) : [],
      warnings: Array.isArray(warnings) ? warnings.map(stringValue) : [],
      usedProportionalScale: Boolean(first(report, "usedProportionalScale", "UsedProportionalScale")),
      outputFormat: stringValue(first(report, "outputFormat", "OutputFormat")),
      lossyReencode: Boolean(first(report, "lossyReencode", "LossyReencode")),
    },
  };
}

function normalizeCandidate(value: unknown): CandidateMatch {
  const raw = record(value);
  const reasons = first(raw, "reasons", "Reasons");
  return {
    path: stringValue(first(raw, "path", "Path")),
    sheetId: stringValue(first(raw, "sheetId", "SheetID", "SheetId")),
    status: stringValue(first(raw, "status", "Status")),
    score: numberValue(first(raw, "score", "Score")),
    reasons: Array.isArray(reasons) ? reasons.map(stringValue) : [],
    encodedWidth: numberValue(first(raw, "encodedWidth", "EncodedWidth")),
    encodedHeight: numberValue(first(raw, "encodedHeight", "EncodedHeight")),
    displayWidth: numberValue(first(raw, "displayWidth", "DisplayWidth")),
    displayHeight: numberValue(first(raw, "displayHeight", "DisplayHeight")),
    format: stringValue(first(raw, "format", "Format")),
    modifiedAt: stringValue(first(raw, "modifiedAt", "ModifiedAt")),
    dimensionStatus: stringValue(first(raw, "dimensionStatus", "DimensionStatus")),
    invalidReason: stringValue(first(raw, "invalidReason", "InvalidReason")) || undefined,
  };
}

function normalizeResolvedSheet(value: unknown): ResolvedSheet {
  const raw = record(value);
  const candidates = first(raw, "candidates", "Candidates");
  const candidate = first(raw, "candidate", "Candidate");
  return {
    sheetId: stringValue(first(raw, "sheetId", "SheetID", "SheetId")),
    index: numberValue(first(raw, "index", "Index")),
    canonicalStem: stringValue(first(raw, "canonicalStem", "CanonicalStem")),
    status: stringValue(first(raw, "status", "Status")),
    dimensionStatus: stringValue(first(raw, "dimensionStatus", "DimensionStatus")),
    path: stringValue(first(raw, "path", "Path")),
    reason: stringValue(first(raw, "reason", "Reason")),
    scaleX: numberValue(first(raw, "scaleX", "ScaleX")),
    scaleY: numberValue(first(raw, "scaleY", "ScaleY")),
    manual: Boolean(first(raw, "manual", "Manual")),
    candidate: candidate ? normalizeCandidate(candidate) : undefined,
    candidates: Array.isArray(candidates) ? candidates.map(normalizeCandidate) : [],
  };
}

function normalizeResolveResponse(value: unknown): ResolveResponse {
  const raw = record(value);
  const sheets = first(raw, "sheets", "Sheets");
  const warnings = first(raw, "warnings", "Warnings");
  return {
    projectPath: stringValue(first(raw, "projectPath", "ProjectPath")),
    sheets: Array.isArray(sheets) ? sheets.map(normalizeResolvedSheet) : [],
    warnings: Array.isArray(warnings) ? warnings.map(stringValue) : [],
  };
}

export async function chooseImages(): Promise<string[]> {
  const gui = getGUI();
  if (gui) return gui.ChooseImages();
  return [];
}

export async function chooseProcessedFiles(): Promise<string[]> {
  const gui = getGUI();
  if (gui) return gui.ChooseProcessedFiles();
  return [];
}

export async function chooseFolder(): Promise<string> {
  const gui = getGUI();
  if (gui) return gui.ChooseFolder();
  return "";
}

export async function chooseProject(): Promise<string> {
  const gui = getGUI();
  if (gui) return gui.ChooseProject();
  return "";
}

export async function chooseDirectory(title: string): Promise<string> {
  const gui = getGUI();
  if (gui) return gui.ChooseDirectory(title);
  return "";
}

export async function pack(request: PackRequest): Promise<PackResponse> {
  const gui = getGUI();
  if (!gui) throw new Error("请在 PhotoSheet 桌面应用中运行此操作");
  return normalizePackResponse(await gui.Pack(request));
}

export async function preview(request: PackRequest): Promise<PreviewResponse> {
  const gui = getGUI();
  if (!gui) throw new Error("请在 PhotoSheet 桌面应用中运行此操作");
  const value = record(await gui.Preview(request));
  const sheets = first(value, "sheets", "Sheets");
  return {
    sheets: Array.isArray(sheets) ? sheets as PreviewResponse["sheets"] : [],
    warnings: Array.isArray(first(value, "warnings", "Warnings")) ? (first(value, "warnings", "Warnings") as unknown[]).map(stringValue) : [],
    hasRaw: Boolean(first(value, "hasRaw", "HasRAW", "HasRaw")),
    scanSummary: normalizeScanSummary(first(value, "scanSummary", "ScanSummary")),
  };
}

export async function diagnostics(): Promise<CapabilityReport> {
  const gui = getGUI();
  if (!gui) throw new Error("请在 PhotoSheet 桌面应用中运行此操作");
  const value = record(await gui.Diagnostics());
  const backends = first(value, "backends", "Backends");
  return {
    libRawEnabled: Boolean(first(value, "libRawEnabled", "LibRawEnabled")),
    DNGSDKEnabled: Boolean(first(value, "DNGSDKEnabled", "DngsdkEnabled")),
    JPEGXLDNGEnabled: Boolean(first(value, "JPEGXLDNGEnabled", "JpegxldngEnabled")),
    output16BitEnabled: Boolean(first(value, "output16BitEnabled", "Output16BitEnabled")),
    DNGOpcodeProcessing: Boolean(first(value, "DNGOpcodeProcessing", "DngopcodeProcessing")),
    backends: Array.isArray(backends) ? backends.map((item) => {
      const backend = record(item);
      const caps = record(first(backend, "capabilities", "Capabilities"));
      return {
        name: stringValue(first(backend, "name", "Name")),
        version: stringValue(first(backend, "version", "Version")),
        available: Boolean(first(backend, "available", "Available")),
        error: stringValue(first(backend, "error", "Error")) || undefined,
        capabilities: {
          probeSupported: Boolean(first(caps, "probeSupported", "ProbeSupported")),
          decodeSupported: Boolean(first(caps, "decodeSupported", "DecodeSupported")),
          previewSupported: Boolean(first(caps, "previewSupported", "PreviewSupported")),
          supports16BitRGB: Boolean(first(caps, "supports16BitRGB", "Supports16BitRGB")),
          supportsJPEGXL: Boolean(first(caps, "supportsJPEGXL", "SupportsJPEGXL")),
          DNGSDKEnabled: Boolean(first(caps, "DNGSDKEnabled", "DngsdkEnabled")),
          DNGOpcodeProcessing: Boolean(first(caps, "DNGOpcodeProcessing", "DngopcodeProcessing")),
          outputColorSpace: stringValue(first(caps, "outputColorSpace", "OutputColorSpace")),
        },
      };
    }) : [],
  };
}

export async function unpack(request: Parameters<GUIApp["Unpack"]>[0]): Promise<UnpackResponse> {
  const gui = getGUI();
  if (!gui) throw new Error("请在 PhotoSheet 桌面应用中运行此操作");
  return normalizeUnpackResponse(await gui.Unpack(request));
}

export async function resolveProcessed(request: Parameters<GUIApp["Resolve"]>[0]): Promise<ResolveResponse> {
  const gui = getGUI();
  if (!gui) throw new Error("请在 PhotoSheet 桌面应用中运行此操作");
  return normalizeResolveResponse(await gui.Resolve(request));
}

export async function recentProcessedLocations(): Promise<string[]> {
  const gui = getGUI();
  if (gui) return gui.RecentProcessedLocations();
  return [];
}

export async function rememberProcessedLocations(locations: string[]): Promise<void> {
  const gui = getGUI();
  if (gui) await gui.RememberProcessedLocations(locations);
}

export async function verify(request: Parameters<GUIApp["Verify"]>[0]): Promise<void> {
  const gui = getGUI();
  if (!gui) throw new Error("请在 PhotoSheet 桌面应用中运行此操作");
  return gui.Verify(request);
}

export async function cancel(): Promise<void> {
  const gui = getGUI();
  if (gui) await gui.Cancel();
}

export function onProgress(callback: (event: Progress) => void): () => void {
  const eventsOn = window.runtime?.EventsOn;
  if (eventsOn) return eventsOn("photosheet:progress", (event) => callback(normalizeProgress(event)));
  return () => undefined;
}
