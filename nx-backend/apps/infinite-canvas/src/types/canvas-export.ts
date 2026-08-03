import type { CanvasProject } from "@/stores/canvas/use-canvas-store";

export const CANVAS_EXPORT_VERSION = 3 as const;

export type CanvasExportFile = {
    app: "infinite-canvas";
    version: typeof CANVAS_EXPORT_VERSION;
    exportedAt: string;
    projects: CanvasProjectExportItem[];
};

export type CanvasProjectExportItem = {
    project: CanvasProject;
    files: CanvasExportAsset[];
};

export type CanvasExportAsset = {
    storageKey: string;
    path: string;
    mimeType: string;
    bytes: number;
};
