import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
    createZip: vi.fn<(files: { name: string; data: BlobPart }[]) => Promise<Blob>>(async () => new Blob(["zip"])),
    getImageBlob: vi.fn(),
    getMediaBlob: vi.fn(),
    saveAs: vi.fn(),
}));

vi.mock("@/lib/zip", () => ({ createZip: mocks.createZip }));
vi.mock("@/services/image-storage", () => ({ getImageBlob: mocks.getImageBlob }));
vi.mock("@/services/file-storage", () => ({ getMediaBlob: mocks.getMediaBlob }));
vi.mock("file-saver", () => ({ saveAs: mocks.saveAs }));

import { exportCanvasProjects, normalizeCanvasExportFile, sanitizeCanvasProjectForExport } from "./canvas-export";
import type { CanvasProject } from "@/stores/canvas/use-canvas-store";

function fixtureProject(): CanvasProject {
    return {
        id: "project-1",
        title: "Export boundary",
        createdAt: "2026-08-03T00:00:00.000Z",
        updatedAt: "2026-08-03T00:00:00.000Z",
        nodes: [
            {
                id: "node-1",
                type: "image",
                title: "Image",
                position: { x: 0, y: 0 },
                width: 320,
                height: 240,
                metadata: {
                    model: "legacy-channel::image-model",
                    prompt: "keep this prompt",
                    nested: {
                        apiKey: "NODE_SECRET",
                        apiBase: "https://node-secret.example/v1",
                        capabilityConfigs: { image: { apiKey: "CAPABILITY_SECRET" } },
                        imageApiKey: "IMAGE_SECRET",
                        imageApiBase: "https://image-secret.example/v1",
                        videoApiKey: "VIDEO_SECRET",
                        videoApiBase: "https://video-secret.example/v1",
                        textApiKey: "TEXT_SECRET",
                        textApiBase: "https://text-secret.example/v1",
                        audioApiKey: "AUDIO_SECRET",
                        audioApiBase: "https://audio-secret.example/v1",
                    },
                },
            },
        ],
        connections: [],
        chatSessions: [
            {
                id: "chat-1",
                title: "Chat",
                createdAt: "2026-08-03T00:00:00.000Z",
                updatedAt: "2026-08-03T00:00:00.000Z",
                messages: [
                    {
                        id: "message-1",
                        role: "assistant",
                        text: "ok",
                        detail: {
                            config: {
                                baseUrl: "https://chat-secret.example/v1",
                                apiKey: "CHAT_SECRET",
                                model: "private-model",
                            },
                        },
                    },
                ],
            },
        ],
        activeChatId: "chat-1",
        backgroundMode: "lines",
        showImageInfo: false,
        viewport: { x: 0, y: 0, k: 1 },
    } as unknown as CanvasProject;
}

function collectKeysAndStrings(value: unknown, keys: string[] = [], strings: string[] = []) {
    if (typeof value === "string") strings.push(value);
    if (!value || typeof value !== "object") return { keys, strings };
    if (Array.isArray(value)) {
        value.forEach((item) => collectKeysAndStrings(item, keys, strings));
        return { keys, strings };
    }
    Object.entries(value).forEach(([key, item]) => {
        keys.push(key);
        collectKeysAndStrings(item, keys, strings);
    });
    return { keys, strings };
}

describe("canvas project export boundary", () => {
    beforeEach(() => vi.clearAllMocks());

    it("keeps the raw node model override while recursively removing model credentials and config objects", () => {
        const exported = sanitizeCanvasProjectForExport(fixtureProject());
        const scanned = collectKeysAndStrings(exported);
        const normalizedKeys = scanned.keys.map((key) => key.toLowerCase().replace(/[^a-z]/g, ""));

        expect(exported.nodes[0]?.metadata?.model).toBe("image-model");
        expect(exported.nodes[0]?.metadata?.prompt).toBe("keep this prompt");
        expect(normalizedKeys).not.toContain("apikey");
        expect(normalizedKeys).not.toContain("apibase");
        expect(normalizedKeys).not.toContain("baseurl");
        expect(normalizedKeys).not.toContain("capabilityconfigs");
        expect(normalizedKeys).not.toContain("config");
        expect(scanned.strings).not.toContain("NODE_SECRET");
        expect(scanned.strings).not.toContain("CAPABILITY_SECRET");
        expect(scanned.strings).not.toContain("CHAT_SECRET");
        expect(scanned.strings).not.toContain("IMAGE_SECRET");
        expect(scanned.strings).not.toContain("VIDEO_SECRET");
        expect(scanned.strings).not.toContain("TEXT_SECRET");
        expect(scanned.strings).not.toContain("AUDIO_SECRET");
        expect(scanned.strings).not.toContain("https://node-secret.example/v1");
        expect(scanned.strings).not.toContain("https://chat-secret.example/v1");
        expect(scanned.strings.filter((value) => value.includes("-secret.example/v1"))).toEqual([]);
    });

    it("normalizes legacy models at the projects.json boundary used by the import page", () => {
        const imported = normalizeCanvasExportFile({
            app: "infinite-canvas",
            version: 3,
            exportedAt: "2026-08-03T00:00:00.000Z",
            projects: [
                { project: fixtureProject(), files: [{ storageKey: "image:safe", path: "safe.png", mimeType: "image/png", bytes: 3, apiKey: "ASSET_SECRET", extra: "drop" }] },
                { project: null, files: [] },
                { project: "bad", files: [] },
                null,
                { broken: true },
            ],
        });

        expect(imported.projects).toHaveLength(1);
        expect(imported.projects[0]?.project.nodes[0]?.metadata?.model).toBe("image-model");
        expect(imported.projects[0]?.files[0]).toEqual({ storageKey: "image:safe", path: "safe.png", mimeType: "image/png", bytes: 3 });
        expect(JSON.stringify(imported)).not.toContain("NODE_SECRET");
        expect(JSON.stringify(imported)).not.toContain("ASSET_SECRET");
    });

    it("collects archive assets only after sensitive project subtrees are removed", async () => {
        const project = fixtureProject() as CanvasProject & { config: unknown };
        project.nodes[0]!.metadata!.storageKey = "image:safe";
        project.config = { apiKey: "SECRET", storageKey: "image:orphan" };
        mocks.getImageBlob.mockImplementation(async (key: string) => new Blob([key], { type: "image/png" }));

        await exportCanvasProjects([project], "boundary");

        expect(mocks.getImageBlob).toHaveBeenCalledWith("image:safe");
        expect(mocks.getImageBlob).not.toHaveBeenCalledWith("image:orphan");
        const zipFiles = mocks.createZip.mock.calls[0]![0] as { name: string; data: BlobPart }[];
        expect(zipFiles.map((file) => file.name).join("\n")).not.toContain("orphan");
    });
});
