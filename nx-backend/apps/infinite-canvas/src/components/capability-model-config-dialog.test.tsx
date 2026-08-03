import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { CapabilityModelConfigDialog } from "./capability-model-config-dialog";
import { CanvasTopBar } from "./canvas/canvas-top-bar";
import { createDefaultCapabilityConfigs, defaultConfig, useConfigStore, type ModelCapability } from "@/stores/use-config-store";

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
            config: { ...defaultConfig, capabilityConfigs },
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
        expect((document.querySelector('[data-testid="video-api-base"]') as HTMLInputElement).value).toBe("https://video.example/v1");
        expect((document.querySelector('[data-testid="video-model-id"]') as HTMLInputElement).value).toBe("video-model");

        for (const capability of capabilities) {
            click(document.querySelector(`[data-capability-tab="${capability}"]`)!);
            expect((document.querySelector(`[data-testid="${capability}-api-base"]`) as HTMLInputElement).value).toBe(`https://${capability}.example/v1`);
            expect((document.querySelector(`[data-testid="${capability}-model-id"]`) as HTMLInputElement).value).toBe(`${capability}-model`);
        }
        expect(useConfigStore.getState().targetCapability).toBe("audio");
    });

    it("uses a password input, keeps the advanced script collapsed, and saves only the active capability", () => {
        const before = useConfigStore.getState().config.capabilityConfigs;
        act(() => root.render(<CapabilityModelConfigDialog />));

        const keyInput = document.querySelector('[data-testid="video-api-key"]') as HTMLInputElement;
        expect(keyInput.type).toBe("password");
        expect(document.body.textContent).not.toContain("video-secret-key");

        const advanced = document.querySelector('[data-testid="video-advanced"]') as HTMLDetailsElement;
        expect(advanced.open).toBe(false);
        click(advanced.querySelector("summary")!);
        expect(advanced.open).toBe(true);

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
        expect(document.body.textContent).not.toContain("new-video-secret");
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

        const button = Array.from(container.querySelectorAll("button")).find((item) => item.textContent?.includes("模型配置"));
        expect(button).toBeTruthy();
        click(button!);
        expect(onModelConfig).toHaveBeenCalledTimes(1);

        act(() => root.unmount());
        container.remove();
    });
});
