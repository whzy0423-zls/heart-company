import { create } from "zustand";
import { persist, type PersistStorage, type StorageValue } from "zustand/middleware";

import { nanoid } from "nanoid";
import { localForageStorage } from "@/lib/localforage-storage";
import type { CanvasBackgroundMode } from "@/lib/canvas-theme";
import type { CanvasAssistantMessage, CanvasAssistantReference, CanvasAssistantSession, CanvasConnection, CanvasNodeData, ViewportTransform } from "@/types/canvas";
import { createDefaultCapabilityConfigs, guessCapability, type CapabilityConfigs, type ModelCapability } from "@/stores/use-config-store";

export type CanvasProject = {
    id: string;
    title: string;
    createdAt: string;
    updatedAt: string;
    nodes: CanvasNodeData[];
    connections: CanvasConnection[];
    chatSessions: CanvasAssistantSession[];
    activeChatId: string | null;
    backgroundMode: CanvasBackgroundMode;
    showImageInfo: boolean;
    viewport: ViewportTransform;
};

type CanvasStore = {
    hydrated: boolean;
    projects: CanvasProject[];
    createProject: (title?: string) => string;
    importProject: (project: Partial<CanvasProject>) => string;
    openProject: (id: string) => CanvasProject | null;
    renameProject: (id: string, title: string) => void;
    deleteProjects: (ids: string[]) => void;
    replaceProjects: (projects: CanvasProject[]) => void;
    updateProject: (id: string, patch: Partial<Pick<CanvasProject, "nodes" | "connections" | "chatSessions" | "activeChatId" | "backgroundMode" | "showImageInfo" | "viewport">>) => void;
};

const initialViewport: ViewportTransform = { x: 0, y: 0, k: 1 };
const CANVAS_STORE_KEY = "infinite-canvas:canvas_store";
type PersistedCanvasState = Pick<CanvasStore, "projects">;
type UnknownRecord = Record<string, unknown>;
let saveTimer: ReturnType<typeof setTimeout> | null = null;
let queuedPersistState: PersistedCanvasState | null = null;

const SENSITIVE_PROJECT_KEYS = new Set(["apikey", "apibase", "baseurl", "capabilityconfigs", "config", "aiconfig", "modelconfig"]);
const TEXT_MODEL_PATTERN = /(?:gpt|claude|deepseek|qwen|glm|gemini|llama|mistral|text|chat)/i;

function isRecord(value: unknown): value is UnknownRecord {
    return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

function stripLegacyChannelId(model: string) {
    const separator = model.indexOf("::");
    return separator < 0 ? model : model.slice(separator + 2);
}

function isSensitiveProjectKey(key: string) {
    const normalized = key.toLowerCase().replace(/[^a-z]/g, "");
    return SENSITIVE_PROJECT_KEYS.has(normalized) || normalized.endsWith("apikey") || normalized.endsWith("apibase") || normalized.endsWith("baseurl");
}

function nodeCapability(node: UnknownRecord): ModelCapability | undefined {
    const metadata = isRecord(node.metadata) ? node.metadata : {};
    const mode = metadata.generationMode;
    if (mode === "image" || mode === "video" || mode === "text" || mode === "audio") return mode;
    return node.type === "image" || node.type === "video" || node.type === "text" || node.type === "audio" ? node.type : undefined;
}

function recognizedLegacyModelCapability(model: string, defaults: CapabilityConfigs): ModelCapability | undefined {
    for (const capability of Object.keys(defaults) as ModelCapability[]) {
        if (model === defaults[capability].modelId) return capability;
    }
    const guessed = guessCapability(model);
    if (guessed !== "text") return guessed;
    return TEXT_MODEL_PATTERN.test(model) ? "text" : undefined;
}

function normalizeCanvasConnections(value: unknown): CanvasConnection[] {
    if (!Array.isArray(value)) return [];
    return value.flatMap((connection) => {
        if (!isRecord(connection) || typeof connection.id !== "string" || typeof connection.fromNodeId !== "string" || typeof connection.toNodeId !== "string") return [];
        return [{ id: connection.id, fromNodeId: connection.fromNodeId, toNodeId: connection.toNodeId }];
    });
}

function normalizeAssistantReference(value: unknown): CanvasAssistantReference | null {
    if (!isRecord(value) || typeof value.id !== "string" || typeof value.type !== "string" || typeof value.title !== "string") return null;
    return {
        id: value.id,
        type: value.type,
        title: value.title,
        ...(typeof value.dataUrl === "string" ? { dataUrl: value.dataUrl } : {}),
        ...(typeof value.storageKey === "string" ? { storageKey: value.storageKey } : {}),
        ...(typeof value.text === "string" ? { text: value.text } : {}),
    };
}

function normalizeAssistantMessage(value: unknown): CanvasAssistantMessage | null {
    if (!isRecord(value) || typeof value.id !== "string" || !["user", "assistant", "system", "tool", "error"].includes(String(value.role)) || typeof value.text !== "string") return null;
    const references = Array.isArray(value.references) ? value.references.map(normalizeAssistantReference).filter((reference): reference is CanvasAssistantReference => Boolean(reference)) : [];
    return {
        id: value.id,
        role: value.role as CanvasAssistantMessage["role"],
        text: value.text,
        ...(typeof value.title === "string" ? { title: value.title } : {}),
        ...(typeof value.meta === "string" ? { meta: value.meta } : {}),
        ...("detail" in value ? { detail: value.detail } : {}),
        references,
    };
}

function normalizeAssistantSessions(value: unknown, fallbackTimestamp: string): CanvasAssistantSession[] {
    if (!Array.isArray(value)) return [];
    return value.flatMap((session) => {
        if (!isRecord(session) || typeof session.id !== "string") return [];
        const messages = Array.isArray(session.messages) ? session.messages.map(normalizeAssistantMessage).filter((message): message is CanvasAssistantMessage => Boolean(message)) : [];
        return [
            {
                id: session.id,
                title: typeof session.title === "string" ? session.title : "",
                messages,
                createdAt: typeof session.createdAt === "string" ? session.createdAt : fallbackTimestamp,
                updatedAt: typeof session.updatedAt === "string" ? session.updatedAt : fallbackTimestamp,
            },
        ];
    });
}

/** Removes browser-only model configuration from arbitrary imported project data. */
export function stripProjectSecrets(value: unknown): unknown {
    if (Array.isArray(value)) return value.map(stripProjectSecrets);
    if (!isRecord(value)) return value;
    return Object.fromEntries(
        Object.entries(value)
            .filter(([key]) => !isSensitiveProjectKey(key))
            .map(([key, item]) => [key, stripProjectSecrets(item)]),
    );
}

/** Normalizes node-level legacy overrides without consulting or copying the config store. */
export function normalizeCanvasNodeModelOverrides(value: unknown, capabilityDefaults: CapabilityConfigs = createDefaultCapabilityConfigs()): CanvasNodeData[] {
    if (!Array.isArray(value)) return [];
    return value.flatMap((candidate) => {
        if (
            !isRecord(candidate) ||
            typeof candidate.id !== "string" ||
            typeof candidate.type !== "string" ||
            typeof candidate.title !== "string" ||
            !isRecord(candidate.position) ||
            typeof candidate.position.x !== "number" ||
            typeof candidate.position.y !== "number" ||
            typeof candidate.width !== "number" ||
            typeof candidate.height !== "number"
        )
            return [];
        const clean = stripProjectSecrets(candidate) as UnknownRecord;
        if (isRecord(clean.metadata) && typeof clean.metadata.model === "string" && clean.metadata.model.includes("::")) {
            const capability = nodeCapability(clean);
            const model = stripLegacyChannelId(clean.metadata.model);
            const recognizedCapability = model ? recognizedLegacyModelCapability(model, capabilityDefaults) : undefined;
            clean.metadata = {
                ...clean.metadata,
                model: capability && recognizedCapability !== capability ? capabilityDefaults[capability].modelId : model,
            };
        }
        return [clean as CanvasNodeData];
    });
}

export function normalizeCanvasProject(value: unknown, capabilityDefaults: CapabilityConfigs = createDefaultCapabilityConfigs()): CanvasProject {
    const sanitized = stripProjectSecrets(value);
    const source = isRecord(sanitized) ? sanitized : {};
    const now = new Date().toISOString();
    const viewport = isRecord(source.viewport) && typeof source.viewport.x === "number" && typeof source.viewport.y === "number" && typeof source.viewport.k === "number" ? (source.viewport as ViewportTransform) : initialViewport;
    const chatSessions = normalizeAssistantSessions(source.chatSessions, now);
    const activeChatId = typeof source.activeChatId === "string" && chatSessions.some((session) => session.id === source.activeChatId) ? source.activeChatId : null;
    return {
        id: typeof source.id === "string" && source.id ? source.id : nanoid(),
        title: typeof source.title === "string" && source.title ? source.title : "导入画布",
        createdAt: typeof source.createdAt === "string" ? source.createdAt : now,
        updatedAt: typeof source.updatedAt === "string" ? source.updatedAt : now,
        nodes: normalizeCanvasNodeModelOverrides(source.nodes, capabilityDefaults),
        connections: normalizeCanvasConnections(source.connections),
        chatSessions,
        activeChatId,
        backgroundMode: source.backgroundMode === "dots" || source.backgroundMode === "blank" ? source.backgroundMode : "lines",
        showImageInfo: source.showImageInfo === true,
        viewport,
    };
}

export function migratePersistedCanvasState(value: unknown): PersistedCanvasState {
    const source = isRecord(value) ? value : {};
    return { projects: Array.isArray(source.projects) ? source.projects.filter(isRecord).map((project) => normalizeCanvasProject(project)) : [] };
}

const canvasStorage: PersistStorage<CanvasStore> = {
    getItem: async (name) => {
        const value = await localForageStorage.getItem(name);
        if (!value) return null;
        const parsed = JSON.parse(value) as StorageValue<CanvasStore>;
        const state = migratePersistedCanvasState(parsed.state);
        queuedPersistState = state;
        return { ...parsed, state: { ...parsed.state, ...state } };
    },
    setItem: (name, value) => {
        const nextState = value.state as PersistedCanvasState;
        if (queuedPersistState && queuedPersistState.projects === nextState.projects) return;
        queuedPersistState = nextState;
        if (saveTimer) clearTimeout(saveTimer);
        saveTimer = setTimeout(() => {
            saveTimer = null;
            void localForageStorage.setItem(name, JSON.stringify(value));
        }, 400);
    },
    removeItem: (name) => localForageStorage.removeItem(name),
};

export const useCanvasStore = create<CanvasStore>()(
    persist(
        (set, get) => ({
            hydrated: false,
            projects: [],
            createProject: (title = "未命名画布") => {
                const now = new Date().toISOString();
                const id = nanoid();
                const project: CanvasProject = {
                    id,
                    title,
                    createdAt: now,
                    updatedAt: now,
                    nodes: [],
                    connections: [],
                    chatSessions: [],
                    activeChatId: null,
                    backgroundMode: "lines",
                    showImageInfo: false,
                    viewport: initialViewport,
                };
                set((state) => ({ projects: [project, ...state.projects] }));
                return id;
            },
            importProject: (source) => {
                const now = new Date().toISOString();
                const normalized = normalizeCanvasProject(source);
                const project: CanvasProject = {
                    id: nanoid(),
                    title: normalized.title,
                    createdAt: normalized.createdAt,
                    updatedAt: now,
                    nodes: normalized.nodes,
                    connections: normalized.connections,
                    chatSessions: normalized.chatSessions,
                    activeChatId: normalized.activeChatId,
                    backgroundMode: normalized.backgroundMode,
                    showImageInfo: normalized.showImageInfo,
                    viewport: normalized.viewport,
                };
                set((state) => ({ projects: [project, ...state.projects] }));
                return project.id;
            },
            openProject: (id) => {
                return get().projects.find((item) => item.id === id) || null;
            },
            renameProject: (id, title) =>
                set((state) => ({
                    projects: state.projects.map((project) => (project.id === id ? { ...project, title: title.trim() || project.title, updatedAt: new Date().toISOString() } : project)),
                })),
            deleteProjects: (ids) =>
                set((state) => {
                    const projects = state.projects.filter((project) => !ids.includes(project.id));
                    return { projects };
                }),
            replaceProjects: (projects) => set({ projects }),
            updateProject: (id, patch) =>
                set((state) => ({
                    projects: state.projects.map((project) => (project.id === id ? { ...project, ...patch, updatedAt: new Date().toISOString() } : project)),
                })),
        }),
        {
            name: CANVAS_STORE_KEY,
            storage: canvasStorage,
            partialize: (state) =>
                ({
                    projects: state.projects,
                }) as StorageValue<CanvasStore>["state"],
            onRehydrateStorage: () => () => {
                useCanvasStore.setState({ hydrated: true });
            },
            version: 1,
            migrate: (state) => migratePersistedCanvasState(state) as CanvasStore,
        },
    ),
);
