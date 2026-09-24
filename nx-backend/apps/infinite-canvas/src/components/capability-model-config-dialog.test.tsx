import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { CapabilityModelConfigDialog } from "./capability-model-config-dialog";
import { CanvasConfigNodePanel } from "./canvas/canvas-config-node-panel";
import { CanvasTopBar } from "./canvas/canvas-top-bar";
import { createDefaultCapabilityConfigs, defaultConfig, useConfigStore, type ModelCapability } from "@/stores/use-config-store";
import { CanvasNodeType, type CanvasNodeData } from "@/types/canvas";

vi.hoisted(() => {
    Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
    const memory = new Map<string, string>();
    Object.defineProperty(globalThis, "localStorage", {
        configurable: true,
        value: {
            getItem: (key: string) => memory.get(key) ?? null,
            setItem: (key: string, value: string) => memory.set(key, String(value)),
            removeItem: (key: string) => memory.delete(key),
            clear: () => memory.clear(),
        },
    });
});

const capabilities: ModelCapability[] = ["image", "video", "text", "audio"];

function changeField(element: HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement, value: string) {
    act(() => {
        const setter = Object.getOwnPropertyDescriptor(Object.getPrototypeOf(element), "value")?.set;
        setter?.call(element, value);
        element.dispatchEvent(new Event("input", { bubbles: true }));
        element.dispatchEvent(new Event("change", { bubbles: true }));
    });
}

function click(element: Element) {
    act(() => element.dispatchEvent(new MouseEvent("click", { bubbles: true })));
}

function pointerDown(element: Element) {
    act(() => element.dispatchEvent(new PointerEvent("pointerdown", { bubbles: true, button: 0 })));
}

describe("CapabilityModelConfigDialog", () => {
    let container: HTMLDivElement;
    let root: Root;

    beforeEach(() => {
        container = document.createElement("div");
        document.body.appendChild(container);
        root = createRoot(container);
        const capabilityConfigs = createDefaultCapabilityConfigs();
        for (const capability of capabilities) {
            capabilityConfigs[capability] = {
                apiBase: `https://${capability}.example/v1`,
                apiKey: `${capability}-secret-key`,
                apiFormat: capability === "image" ? "gemini" : "openai",
                modelId: `${capability}-model`,
                script: `${capability} script`,
            };
        }
        useConfigStore.setState({
            config: { ...defaultConfig, capabilityConfigs, size: "legacy-text-size", count: "7", canvasImageCount: "8", imageSize: "1:1", imageCount: "3", videoSize: "1280x720" },
            isConfigOpen: true,
            targetCapability: "video",
            shouldPromptContinue: false,
        });
    });

    afterEach(() => {
        act(() => root.unmount());
        container.remove();
        document.body.innerHTML = "";
    });

    it("opens the requested capability and keeps all four capability sections independent", () => {
        act(() => root.render(<CapabilityModelConfigDialog />));

        expect(document.querySelector('[data-capability-tab="video"]')?.getAttribute("aria-selected")).toBe("true");
        expect(document.querySelectorAll("[data-capability-tab]")).toHaveLength(4);
        click(document.querySelector('[data-capability-tab="image"]')!);
        expect(document.body.textContent).not.toContain("图片理解");
        click(document.querySelector('[data-capability-tab="video"]')!);
        expect((document.querySelector('[data-testid="video-api-base"]') as HTMLInputElement).value).toBe("https://video.example/v1");
        expect((document.querySelector('[data-testid="video-model-id"]') as HTMLInputElement).value).toBe("video-model");

        for (const capability of capabilities) {
            click(document.querySelector(`[data-capability-tab="${capability}"]`)!);
            expect((document.querySelector(`[data-testid="${capability}-api-base"]`) as HTMLInputElement).value).toBe(`https://${capability}.example/v1`);
            expect((document.querySelector(`[data-testid="${capability}-model-id"]`) as HTMLInputElement).value).toBe(`${capability}-model`);
        }
        expect(useConfigStore.getState().targetCapability).toBe("audio");
    });

    it("exposes keyboard-navigable tabs and an associated tabpanel", () => {
        act(() => root.render(<CapabilityModelConfigDialog />));

        const tablist = document.querySelector('[role="tablist"]');
        expect(tablist).toBeTruthy();
        const videoTab = document.querySelector('[role="tab"][data-capability-tab="video"]') as HTMLButtonElement;
        expect(videoTab.id).toBe("capability-tab-video");
        expect(videoTab.getAttribute("aria-controls")).toBe("capability-panel-video");
        expect(videoTab.getAttribute("aria-selected")).toBe("true");
        expect(document.querySelector('[role="tabpanel"]')?.getAttribute("aria-labelledby")).toBe("capability-tab-video");

        act(() => videoTab.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowRight", bubbles: true })));
        expect(useConfigStore.getState().targetCapability).toBe("text");
        act(() => (document.querySelector('[role="tab"][data-capability-tab="text"]') as HTMLButtonElement).dispatchEvent(new KeyboardEvent("keydown", { key: "End", bubbles: true })));
        expect(useConfigStore.getState().targetCapability).toBe("audio");
        act(() => (document.querySelector('[role="tab"][data-capability-tab="audio"]') as HTMLButtonElement).dispatchEvent(new KeyboardEvent("keydown", { key: "Home", bubbles: true })));
        expect(useConfigStore.getState().targetCapability).toBe("image");
    });

    it("toggles API key visibility, keeps advanced script collapsed, validates required fields, and reports a successful continuation save", () => {
        const before = useConfigStore.getState().config.capabilityConfigs;
        useConfigStore.setState({ shouldPromptContinue: true });
        act(() => root.render(<CapabilityModelConfigDialog />));

        const keyInput = document.querySelector('[data-testid="video-api-key"]') as HTMLInputElement;
        expect(keyInput.type).toBe("password");
        expect(document.body.textContent).not.toContain("video-secret-key");
        click(document.querySelector('[data-testid="video-api-key-visibility"]')!);
        expect(keyInput.type).toBe("text");
        click(document.querySelector('[data-testid="video-api-key-visibility"]')!);
        expect(keyInput.type).toBe("password");

        const advanced = document.querySelector('[data-testid="video-advanced"]') as HTMLDetailsElement;
        expect(advanced.open).toBe(false);
        click(advanced.querySelector("summary")!);
        expect(advanced.open).toBe(true);

        changeField(document.querySelector('[data-testid="video-api-base"]') as HTMLInputElement, "");
        changeField(keyInput, "");
        changeField(document.querySelector('[data-testid="video-model-id"]') as HTMLInputElement, "");
        click(document.querySelector('[data-testid="save-capability-config"]')!);
        expect(useConfigStore.getState().isConfigOpen).toBe(true);
        expect(document.querySelector('[role="alert"]')?.textContent).toContain("API Base");
        expect(document.querySelector('[role="alert"]')?.textContent).toContain("API Key");
        expect(document.querySelector('[role="alert"]')?.textContent).toContain("模型 ID");
        expect(document.querySelector('[role="alert"]')?.getAttribute("aria-live")).toBe("assertive");
        const invalidApiBase = document.querySelector('[data-testid="video-api-base"]') as HTMLInputElement;
        expect(invalidApiBase.getAttribute("aria-invalid")).toBe("true");
        expect(invalidApiBase.getAttribute("aria-describedby")).toBe("video-api-base-error");
        expect(document.activeElement).toBe(invalidApiBase);

        changeField(document.querySelector('[data-testid="video-api-base"]') as HTMLInputElement, "https://new-video.example/v1");
        changeField(keyInput, "new-video-secret");
        changeField(document.querySelector('[data-testid="video-api-format"]') as HTMLSelectElement, "ark");
        changeField(document.querySelector('[data-testid="video-model-id"]') as HTMLInputElement, "video-model-v2");
        changeField(document.querySelector('[data-testid="video-script"]') as HTMLTextAreaElement, "return request;");
        click(document.querySelector('[data-testid="save-capability-config"]')!);

        const after = useConfigStore.getState().config.capabilityConfigs;
        expect(after.video).toEqual({
            apiBase: "https://new-video.example/v1",
            apiKey: "new-video-secret",
            apiFormat: "ark",
            modelId: "video-model-v2",
            script: "return request;",
        });
        expect(after.image).toBe(before.image);
        expect(after.text).toBe(before.text);
        expect(after.audio).toBe(before.audio);
        expect(useConfigStore.getState().isConfigOpen).toBe(false);
        expect(useConfigStore.getState().shouldPromptContinue).toBe(false);
        expect(document.body.textContent).toContain("配置已保存，请重新执行刚才的生成操作");
        expect(document.body.textContent).not.toContain("new-video-secret");
    });

    it.each([
        {
            capability: "image" as const,
            fields: [
                ["image-quality", "high"],
                ["image-size", "16:9"],
                ["image-background", "transparent"],
                ["image-count", "4"],
            ],
            expected: { quality: "high", imageSize: "16:9", background: "transparent", imageCount: "4", videoSize: "1280x720", size: "legacy-text-size", count: "7" },
        },
        {
            capability: "video" as const,
            fields: [
                ["video-size", "720x1280"],
                ["video-seconds", "12"],
                ["video-quality", "1080"],
                ["video-generate-audio", "false"],
                ["video-watermark", "true"],
            ],
            expected: { videoSize: "720x1280", videoSeconds: "12", vquality: "1080", videoGenerateAudio: "false", videoWatermark: "true", imageSize: "1:1", imageCount: "3", size: "legacy-text-size", count: "7" },
        },
        {
            capability: "text" as const,
            fields: [
                ["text-system-prompt", "You are concise."],
                ["text-reasoning-effort", "high"],
            ],
            expected: { systemPrompt: "You are concise.", reasoningEffort: "high" },
        },
        {
            capability: "audio" as const,
            fields: [
                ["audio-voice", "nova"],
                ["audio-format", "wav"],
                ["audio-speed", "1.25"],
                ["audio-instructions", "Warm and calm"],
            ],
            expected: { audioVoice: "nova", audioFormat: "wav", audioSpeed: "1.25", audioInstructions: "Warm and calm" },
        },
    ])("edits and saves $capability-specific parameters without changing other capability objects", ({ capability, fields, expected }) => {
        useConfigStore.getState().openConfigDialog(false, "channels", capability);
        const before = useConfigStore.getState().config.capabilityConfigs;
        act(() => root.render(<CapabilityModelConfigDialog />));

        changeField(document.querySelector(`[data-testid="${capability}-model-id"]`) as HTMLInputElement, `${capability}-updated-model`);
        for (const [testId, value] of fields) {
            changeField(document.querySelector(`[data-testid="${testId}"]`) as HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement, value);
        }
        click(document.querySelector('[data-testid="save-capability-config"]')!);

        const state = useConfigStore.getState();
        expect(state.config).toMatchObject(expected);
        expect(state.config.capabilityConfigs[capability].modelId).toBe(`${capability}-updated-model`);
        for (const other of capabilities.filter((item) => item !== capability)) {
            expect(state.config.capabilityConfigs[other]).toBe(before[other]);
        }
    });

    it("saving image size and count leaves the video size isolated", () => {
        useConfigStore.getState().openConfigDialog(false, "channels", "image");
        act(() => root.render(<CapabilityModelConfigDialog />));

        changeField(document.querySelector('[data-testid="image-size"]') as HTMLInputElement, "16:9");
        changeField(document.querySelector('[data-testid="image-count"]') as HTMLInputElement, "5");
        click(document.querySelector('[data-testid="save-capability-config"]')!);

        expect(useConfigStore.getState().config).toMatchObject({
            imageSize: "16:9",
            imageCount: "5",
            videoSize: "1280x720",
            size: "legacy-text-size",
            count: "7",
        });
    });

    it("saving video size leaves image size and count isolated", () => {
        useConfigStore.getState().openConfigDialog(false, "channels", "video");
        act(() => root.render(<CapabilityModelConfigDialog />));

        changeField(document.querySelector('[data-testid="video-size"]') as HTMLInputElement, "720x1280");
        click(document.querySelector('[data-testid="save-capability-config"]')!);

        expect(useConfigStore.getState().config).toMatchObject({
            imageSize: "1:1",
            imageCount: "3",
            videoSize: "720x1280",
            size: "legacy-text-size",
            count: "7",
        });
    });

    it("clears the continuation flag when the dialog is cancelled", () => {
        useConfigStore.setState({ shouldPromptContinue: true });
        act(() => root.render(<CapabilityModelConfigDialog />));
        click(document.querySelector('[data-testid="cancel-capability-config"]')!);
        expect(useConfigStore.getState()).toMatchObject({ isConfigOpen: false, shouldPromptContinue: false });
    });

    it("rejects a persisted protocol that the target capability does not support", () => {
        const config = useConfigStore.getState().config;
        useConfigStore.setState({
            config: {
                ...config,
                capabilityConfigs: {
                    ...config.capabilityConfigs,
                    video: { ...config.capabilityConfigs.video, apiFormat: "gemini" },
                },
            },
        });
        act(() => root.render(<CapabilityModelConfigDialog />));
        click(document.querySelector('[data-testid="save-capability-config"]')!);
        expect(useConfigStore.getState().isConfigOpen).toBe(true);
        expect(document.querySelector('[role="alert"]')?.textContent).toContain("不支持 gemini 协议");
    });
});

describe("real missing-config canvas entry", () => {
    it("opens the dialog at the generation capability used by the config node", () => {
        const container = document.createElement("div");
        document.body.appendChild(container);
        const root = createRoot(container);
        useConfigStore.setState({
            config: {
                ...defaultConfig,
                channels: [],
                models: [],
                capabilityConfigs: {
                    ...createDefaultCapabilityConfigs(),
                    video: { apiBase: "", apiKey: "", apiFormat: "openai", modelId: "" },
                },
            },
            isConfigOpen: false,
            targetCapability: "image",
        });
        const node: CanvasNodeData = {
            id: "config-video",
            type: CanvasNodeType.Config,
            title: "视频配置",
            position: { x: 0, y: 0 },
            width: 420,
            height: 260,
            metadata: { generationMode: "video", composerContent: "生成视频" },
        };

        act(() =>
            root.render(
                <CanvasConfigNodePanel
                    node={node}
                    isRunning={false}
                    inputSummary={{ textCount: 1, imageCount: 0, videoCount: 0, audioCount: 0 }}
                    onConfigChange={() => undefined}
                    onGenerate={() => undefined}
                    onStop={() => undefined}
                    onComposerToggle={() => undefined}
                />,
            ),
        );
        const trigger = container.querySelector(".canvas-composer-model-picker")!;
        pointerDown(trigger);
        click(trigger);
        expect(useConfigStore.getState()).toMatchObject({ isConfigOpen: true, targetCapability: "video", shouldPromptContinue: true });

        act(() => root.unmount());
        container.remove();
    });
});

describe("CanvasTopBar model settings entry", () => {
    it("renders an always-visible model settings button and invokes its callback", () => {
        const container = document.createElement("div");
        document.body.appendChild(container);
        const root = createRoot(container);
        const onModelConfig = vi.fn();

        act(() =>
            root.render(
                <CanvasTopBar
                    title="测试画布"
                    titleDraft="测试画布"
                    isTitleEditing={false}
                    onTitleDraftChange={() => undefined}
                    onStartTitleEditing={() => undefined}
                    onFinishTitleEditing={() => undefined}
                    onCancelTitleEditing={() => undefined}
                    canUndo={false}
                    canRedo={false}
                    onProjects={() => undefined}
                    onCreateProject={() => undefined}
                    onDeleteProject={() => undefined}
                    onExportProject={() => undefined}
                    onImportImage={() => undefined}
                    onModelConfig={onModelConfig}
                    onUndo={() => undefined}
                    onRedo={() => undefined}
                />,
            ),
        );

        const button = container.querySelector<HTMLButtonElement>('button[aria-label="模型配置"]');
        expect(button).toBeTruthy();
        click(button!);
        expect(onModelConfig).toHaveBeenCalledTimes(1);

        act(() => root.unmount());
        container.remove();
    });
});
