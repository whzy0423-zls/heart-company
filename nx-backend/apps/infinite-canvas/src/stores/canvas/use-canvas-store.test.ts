import { beforeEach, describe, expect, it, vi } from "vitest";

const storage = vi.hoisted(() => new Map<string, string>());

vi.mock("@/lib/localforage-storage", () => ({
    localForageStorage: {
        getItem: async (key: string) => storage.get(key) ?? null,
        setItem: async (key: string, value: string) => void storage.set(key, value),
        removeItem: async (key: string) => void storage.delete(key),
    },
}));

import { migratePersistedCanvasState, normalizeCanvasProject, useCanvasStore } from "./use-canvas-store";

const legacyProject = {
    id: "legacy-project",
    title: "Legacy",
    createdAt: "2026-08-03T00:00:00.000Z",
    updatedAt: "2026-08-03T00:00:00.000Z",
    nodes: [
        { id: "image", type: "image", title: "Image", position: { x: 0, y: 0 }, width: 100, height: 100, metadata: { model: "image-channel::image-model" } },
        { id: "text", type: "text", title: "Text", position: { x: 10, y: 10 }, width: 100, height: 100, metadata: { model: "explicit-legacy-model" } },
    ],
    connections: [],
    chatSessions: [],
    activeChatId: null,
    backgroundMode: "lines",
    showImageInfo: false,
    viewport: { x: 0, y: 0, k: 1 },
};

describe("canvas project model migration", () => {
    beforeEach(() => {
        storage.clear();
        useCanvasStore.setState({ hydrated: false, projects: [] });
    });

    it("normalizes legacy channel model overrides during local project rehydrate", async () => {
        storage.set("infinite-canvas:canvas_store", JSON.stringify({ state: { projects: [legacyProject] }, version: 0 }));

        await useCanvasStore.persist.rehydrate();

        expect(useCanvasStore.getState().projects[0]?.nodes[0]?.metadata?.model).toBe("image-model");
        expect(useCanvasStore.getState().projects[0]?.nodes[1]?.metadata?.model).toBe("explicit-legacy-model");
    });

    it("normalizes imported models without moving embedded credentials into the project", () => {
        const projectId = useCanvasStore.getState().importProject({
            ...legacyProject,
            nodes: [legacyProject.nodes[0], null, { id: "video", type: "video", title: "Video", position: { x: 0, y: 0 }, width: 100, height: 100, metadata: { model: "channel::video-model", apiKey: "DO_NOT_IMPORT", apiBase: "https://secret.example" } }],
            capabilityConfigs: { image: { apiKey: "DO_NOT_IMPORT_EITHER" } },
        } as never);
        const imported = useCanvasStore.getState().openProject(projectId);

        expect(imported?.nodes.map((node) => node.metadata?.model)).toEqual(["image-model", "video-model"]);
        expect(JSON.stringify(imported)).not.toContain("DO_NOT_IMPORT");
        expect(imported).not.toHaveProperty("capabilityConfigs");
    });

    it("handles damaged partial projects safely and is idempotent", () => {
        const normalized = normalizeCanvasProject({
            id: "partial",
            nodes: [null, 1, { id: "usable", type: "text", title: "Usable", position: { x: 0, y: 0 }, width: 100, height: 100, metadata: { model: "a::b::raw-model" } }],
        });
        const migrated = migratePersistedCanvasState({ projects: [normalized, null] });

        expect(normalized.nodes).toHaveLength(1);
        expect(normalized.nodes[0]?.metadata?.model).toBe("b::raw-model");
        expect(migratePersistedCanvasState(migrated)).toEqual(migrated);
    });
});
