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
import { hydrateAssistantImages } from "@/lib/canvas/canvas-generation-helpers";

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

    it("falls back mismatched or unknown legacy channel overrides by node capability while preserving bare overrides", () => {
        const normalized = normalizeCanvasProject({
            ...legacyProject,
            nodes: [
                { id: "image", type: "image", title: "Image", position: { x: 0, y: 0 }, width: 100, height: 100, metadata: { model: "legacy::video-model" } },
                { id: "video", type: "video", title: "Video", position: { x: 0, y: 0 }, width: 100, height: 100, metadata: { model: "legacy::mystery-model" } },
                { id: "audio", type: "audio", title: "Audio", position: { x: 0, y: 0 }, width: 100, height: 100, metadata: { model: "legacy::" } },
                { id: "text", type: "text", title: "Text", position: { x: 0, y: 0 }, width: 100, height: 100, metadata: { model: "bare-custom-model" } },
                { id: "config", type: "config", title: "Config", position: { x: 0, y: 0 }, width: 100, height: 100, metadata: { generationMode: "image", model: "legacy::audio-model" } },
            ],
        });

        expect(normalized.nodes.map((node) => node.metadata?.model)).toEqual(["gpt-image-2", "grok-imagine-video", "gpt-4o-mini-tts", "bare-custom-model", "gpt-image-2"]);
    });

    it("handles damaged partial projects safely and is idempotent", () => {
        const normalized = normalizeCanvasProject({
            id: "partial",
            nodes: [null, 1, { id: "usable", type: "text", title: "Usable", position: { x: 0, y: 0 }, width: 100, height: 100, metadata: { model: "a::text-model" } }],
        });
        const migrated = migratePersistedCanvasState({ projects: [normalized, null] });

        expect(normalized.nodes).toHaveLength(1);
        expect(normalized.nodes[0]?.metadata?.model).toBe("text-model");
        expect(migratePersistedCanvasState(migrated)).toEqual(migrated);
    });

    it("normalizes damaged assistant sessions, messages, and references before hydration", async () => {
        const normalized = normalizeCanvasProject({
            ...legacyProject,
            chatSessions: [
                null,
                { id: "empty", title: "Empty", createdAt: "created", updatedAt: "updated", messages: null },
                {
                    id: "valid",
                    title: "Valid",
                    createdAt: "created",
                    updatedAt: "updated",
                    messages: [
                        null,
                        { id: "message-1", role: "assistant", text: "ok", references: "broken", extra: "drop" },
                        {
                            id: "message-2",
                            role: "user",
                            text: "ref",
                            references: [null, { id: "ref-1", type: "image", title: "Reference", text: "keep", apiKey: "DROP_SECRET", extra: "drop" }],
                        },
                    ],
                },
                { title: "missing id", messages: [] },
            ],
        });

        await expect(hydrateAssistantImages(normalized.chatSessions)).resolves.toHaveLength(2);
        expect(normalized.chatSessions[0]?.messages).toEqual([]);
        expect(normalized.chatSessions[1]?.messages).toHaveLength(2);
        expect(normalized.chatSessions[1]?.messages[0]?.references).toEqual([]);
        expect(normalized.chatSessions[1]?.messages[1]?.references).toEqual([{ id: "ref-1", type: "image", title: "Reference", text: "keep" }]);
        expect(JSON.stringify(normalized.chatSessions)).not.toContain("DROP_SECRET");
        expect(JSON.stringify(normalized.chatSessions)).not.toContain('"extra"');
    });
});
