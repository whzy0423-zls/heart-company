import { create } from "zustand";
import { persist, type PersistStorage, type StorageValue } from "zustand/middleware";

import { nanoid } from "nanoid";
import { localForageStorage } from "@/lib/localforage-storage";
import type { CanvasBackgroundMode } from "@/lib/canvas-theme";
import type { CanvasAssistantSession, CanvasConnection, CanvasNodeData, ViewportTransform } from "@/types/canvas";

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

function isRecord(value: unknown): value is UnknownRecord {
    return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

function stripLegacyChannelId(model: string) {
    const separator = model.indexOf("::");
    return separator < 0 ? model : model.slice(separator + 2);
}

/** Removes browser-only model configuration from arbitrary imported project data. */
export function stripProjectSecrets(value: unknown): unknown {
    if (Array.isArray(value)) return value.map(stripProjectSecrets);
    if (!isRecord(value)) return value;
    return Object.fromEntries(
        Object.entries(value)
            .filter(([key]) => !SENSITIVE_PROJECT_KEYS.has(key.toLowerCase().replace(/[^a-z]/g, "")))
            .map(([key, item]) => [key, stripProjectSecrets(item)]),
    );
}

/** Normalizes node-level legacy overrides without consulting or copying the config store. */
export function normalizeCanvasNodeModelOverrides(value: unknown): CanvasNodeData[] {
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
        if (isRecord(clean.metadata) && typeof clean.metadata.model === "string") {
            clean.metadata = { ...clean.metadata, model: stripLegacyChannelId(clean.metadata.model) };
        }
        return [clean as CanvasNodeData];
    });
}

export function normalizeCanvasProject(value: unknown): CanvasProject {
    const sanitized = stripProjectSecrets(value);
    const source = isRecord(sanitized) ? sanitized : {};
    const now = new Date().toISOString();
    const viewport = isRecord(source.viewport) && typeof source.viewport.x === "number" && typeof source.viewport.y === "number" && typeof source.viewport.k === "number" ? (source.viewport as ViewportTransform) : initialViewport;
    return {
        id: typeof source.id === "string" && source.id ? source.id : nanoid(),
        title: typeof source.title === "string" && source.title ? source.title : "导入画布",
        createdAt: typeof source.createdAt === "string" ? source.createdAt : now,
        updatedAt: typeof source.updatedAt === "string" ? source.updatedAt : now,
        nodes: normalizeCanvasNodeModelOverrides(source.nodes),
        connections: Array.isArray(source.connections) ? (source.connections.filter(isRecord) as CanvasConnection[]) : [],
        chatSessions: Array.isArray(source.chatSessions) ? (source.chatSessions.filter(isRecord) as CanvasAssistantSession[]) : [],
        activeChatId: typeof source.activeChatId === "string" ? source.activeChatId : null,
        backgroundMode: source.backgroundMode === "dots" || source.backgroundMode === "blank" ? source.backgroundMode : "lines",
        showImageInfo: source.showImageInfo === true,
        viewport,
    };
}

export function migratePersistedCanvasState(value: unknown): PersistedCanvasState {
    const source = isRecord(value) ? value : {};
    return { projects: Array.isArray(source.projects) ? source.projects.filter(isRecord).map(normalizeCanvasProject) : [] };
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
