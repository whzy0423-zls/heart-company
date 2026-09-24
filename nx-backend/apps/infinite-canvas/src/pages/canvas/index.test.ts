import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
    readZip: vi.fn(),
    setImageBlob: vi.fn(),
    setMediaBlob: vi.fn(),
}));

vi.mock("@/lib/zip", () => ({ readZip: mocks.readZip, createZip: vi.fn() }));
vi.mock("@/services/image-storage", () => ({ setImageBlob: mocks.setImageBlob, getImageBlob: vi.fn() }));
vi.mock("@/services/file-storage", () => ({ setMediaBlob: mocks.setMediaBlob, getMediaBlob: vi.fn() }));
vi.mock("@/lib/localforage-storage", () => ({
    localForageStorage: { getItem: vi.fn(async () => null), setItem: vi.fn(async () => undefined), removeItem: vi.fn(async () => undefined) },
}));

import { importCanvasArchive } from "./index";
import type { CanvasProject } from "@/stores/canvas/use-canvas-store";

describe("canvas page import boundary", () => {
    beforeEach(() => vi.clearAllMocks());

    it("reads the zip, restores attachments, and passes a normalized sanitized project to importProject", async () => {
        const project = {
            id: "legacy",
            title: "Imported",
            nodes: [{ id: "video", type: "video", title: "Video", position: { x: 0, y: 0 }, width: 100, height: 100, metadata: { model: "channel::video-model", apiKey: "ZIP_SECRET", apiBase: "https://zip-secret.example" } }],
            connections: [],
            chatSessions: [],
            viewport: { x: 0, y: 0, k: 1 },
        };
        const manifest = new Blob([
            JSON.stringify({
                app: "infinite-canvas",
                version: 3,
                projects: [{ project, files: [{ storageKey: "image:asset-1", path: "projects/legacy/files/image.png", mimeType: "image/png", bytes: 3 }] }],
            }),
        ]);
        const asset = new Blob(["img"], { type: "image/png" });
        mocks.readZip.mockResolvedValue(
            new Map([
                ["projects.json", manifest],
                ["projects/legacy/files/image.png", asset],
            ]),
        );
        const importProject = vi.fn<(project: Partial<CanvasProject>) => string>(() => "new-project");

        const count = await importCanvasArchive(new File(["zip"], "canvas.zip"), importProject);

        expect(count).toBe(1);
        expect(mocks.readZip).toHaveBeenCalledOnce();
        expect(mocks.setImageBlob).toHaveBeenCalledWith("image:asset-1", asset);
        expect(mocks.setMediaBlob).not.toHaveBeenCalled();
        expect(importProject).toHaveBeenCalledOnce();
        const imported = importProject.mock.calls[0]![0] as CanvasProject;
        expect(imported.nodes[0]?.metadata?.model).toBe("video-model");
        expect(JSON.stringify(imported)).not.toContain("ZIP_SECRET");
        expect(JSON.stringify(imported)).not.toContain("https://zip-secret.example");
    });
});
