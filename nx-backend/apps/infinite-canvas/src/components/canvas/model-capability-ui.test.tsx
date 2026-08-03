import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ModelPicker } from "@/components/model-picker";
import { CanvasNodePromptPanel } from "./canvas-node-prompt-panel";
import { CanvasConfigNodePanel } from "./canvas-config-node-panel";
import { createDefaultCapabilityConfigs, defaultConfig, type AiConfig, useConfigStore } from "@/stores/use-config-store";
import { CanvasNodeType, type CanvasNodeData } from "@/types/canvas";

vi.mock("./canvas-prompt-library", () => ({ CanvasPromptLibrary: () => null }));
vi.mock("./canvas-prompt-chip-input", () => ({
    CanvasPromptChipInput: (props: { value: string; onChange: (value: string) => void }) => <textarea data-testid="prompt-input" value={props.value} onChange={(event) => props.onChange(event.target.value)} />,
}));
const popoverState = vi.hoisted(() => ({ videoConfigs: [] as Array<{ size?: string; videoSize?: string; count?: string; imageCount?: string }> }));

vi.mock("./canvas-image-settings-popover", () => ({ CanvasImageSettingsPopover: () => null }));
vi.mock("./canvas-video-settings-popover", () => ({
    CanvasVideoSettingsPopover: ({ config }: { config: { size?: string; videoSize?: string; count?: string; imageCount?: string } }) => {
        popoverState.videoConfigs.push(config);
        return <div data-testid="video-settings-config">{config.videoSize || config.size}</div>;
    },
}));
vi.mock("./canvas-audio-settings-popover", () => ({ CanvasAudioSettingsPopover: () => null }));
vi.mock("./canvas-text-settings-popover", () => ({ CanvasTextSettingsPopover: () => null }));

vi.hoisted(() => {
    Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
    const memory = new Map<string, string>();
    Object.defineProperty(globalThis, "localStorage", {
        configurable: true,
        value: { getItem: (key: string) => memory.get(key) ?? null, setItem: (key: string, value: string) => memory.set(key, String(value)), removeItem: (key: string) => memory.delete(key), clear: () => memory.clear() },
    });
});

function configFixture(): AiConfig {
    const capabilityConfigs = createDefaultCapabilityConfigs();
    capabilityConfigs.image = { ...capabilityConfigs.image, apiBase: "https://image", apiKey: "image-secret", modelId: "image-model" };
    capabilityConfigs.video = { ...capabilityConfigs.video, apiBase: "https://video", apiKey: "video-secret", modelId: "video-model" };
    capabilityConfigs.text = { ...capabilityConfigs.text, apiBase: "https://text", apiKey: "text-secret", modelId: "text-model" };
    capabilityConfigs.audio = { ...capabilityConfigs.audio, apiBase: "https://audio", apiKey: "audio-secret", modelId: "audio-model" };
    return { ...defaultConfig, capabilityConfigs, channels: [], models: [] };
}

function render(element: React.ReactNode) {
    const container = document.createElement("div");
    document.body.appendChild(container);
    const root = createRoot(container);
    act(() => root.render(element));
    return { container, root };
}

function pointerDown(element: Element) {
    act(() => element.dispatchEvent(new PointerEvent("pointerdown", { bubbles: true, button: 0 })));
}

function click(element: Element) {
    act(() => element.dispatchEvent(new MouseEvent("click", { bubbles: true })));
}

function cleanup({ container, root }: { container: HTMLDivElement; root: Root }) {
    act(() => root.unmount());
    container.remove();
    document.body.innerHTML = "";
}

describe("capability-specific node model UI", () => {
    beforeEach(() => {
        popoverState.videoConfigs = [];
        useConfigStore.setState({ config: configFixture(), isConfigOpen: false, targetCapability: "image" });
    });

    afterEach(() => {
        document.body.innerHTML = "";
    });

    it("uses only the selected capability model and hides other capability models", () => {
        const { container, root } = render(<ModelPicker config={configFixture()} capability="image" value="" onChange={() => undefined} />);
        const trigger = container.querySelector("[data-slot=select-trigger]") as HTMLButtonElement;
        expect(trigger.textContent).toContain("image-model");
        expect(trigger.textContent).not.toContain("video-model");
        expect(trigger.textContent).not.toContain("text-model");
        expect(trigger.textContent).not.toContain("audio-model");
        cleanup({ container, root });
    });

    it("calls the missing-config entry when the selected capability credentials are incomplete", () => {
        const config = configFixture();
        config.capabilityConfigs.audio = { ...config.capabilityConfigs.audio, apiKey: "" };
        const onMissingConfig = vi.fn();
        const { container, root } = render(<ModelPicker config={config} capability="audio" value="" onChange={() => undefined} onMissingConfig={onMissingConfig} />);
        const trigger = container.querySelector("[data-slot=select-trigger]") as HTMLButtonElement;
        pointerDown(trigger);
        click(trigger);
        expect(onMissingConfig).toHaveBeenCalledTimes(1);
        expect(document.querySelector("[data-slot=select-content]")).toBeNull();
        expect(trigger.getAttribute("data-state")).toBe("closed");
        cleanup({ container, root });
    });

    it("renders legacy channel model overrides by raw model name and offers capability default for reset", () => {
        const config = configFixture();
        config.channels = [{ id: "legacy-channel", name: "Legacy Channel", baseUrl: "https://legacy", apiKey: "secret", apiFormat: "openai", models: [{ name: "legacy-image", capability: "image" }] }];
        const onChange = vi.fn();
        const { container, root } = render(<ModelPicker config={config} capability="image" value="legacy-channel::legacy-image" onChange={onChange} />);
        const trigger = container.querySelector("[data-slot=select-trigger]") as HTMLButtonElement;
        expect(trigger.textContent).toContain("legacy-image");
        expect(trigger.textContent).not.toContain("Legacy Channel");
        expect(trigger.getAttribute("title")).toBe("legacy-image");
        pointerDown(trigger);
        click(trigger);
        const options = Array.from(document.querySelectorAll("[data-slot=select-item]"));
        expect(options.map((item) => item.textContent)).toEqual(expect.arrayContaining([expect.stringContaining("image-model"), expect.stringContaining("legacy-image")]));
        const defaultOption = options.find((item) => item.textContent?.includes("image-model"))!;
        pointerDown(defaultOption);
        click(defaultOption);
        expect(onChange).toHaveBeenCalledWith(undefined);
        expect(document.body.textContent).not.toContain("video-model");
        expect(document.body.textContent).not.toContain("secret");
        cleanup({ container, root });
    });

    it.each([
        [CanvasNodeType.Image, "image"],
        [CanvasNodeType.Video, "video"],
        [CanvasNodeType.Text, "text"],
        [CanvasNodeType.Audio, "audio"],
    ] as const)("defaults prompt node %s to its own capability model", (nodeType, model) => {
        const node: CanvasNodeData = { id: String(nodeType), type: nodeType, title: "测试", position: { x: 0, y: 0 }, width: 400, height: 300, metadata: {} };
        const { container, root } = render(<CanvasNodePromptPanel node={node} isRunning={false} onPromptChange={() => undefined} onConfigChange={() => undefined} onGenerate={() => undefined} onStop={() => undefined} />);
        const trigger = container.querySelector("[data-slot=select-trigger]") as HTMLButtonElement;
        expect(trigger.textContent).toContain(`${model}-model`);
        for (const other of ["image", "video", "text", "audio"].filter((item) => item !== model)) {
            expect(trigger.textContent).not.toContain(`${other}-model`);
        }
        cleanup({ container, root });
    });

    it("ignores a model override from another capability", () => {
        const node: CanvasNodeData = { id: "config-video", type: CanvasNodeType.Config, title: "视频", position: { x: 0, y: 0 }, width: 400, height: 300, metadata: { generationMode: "video", model: "image-model" } };
        const { container, root } = render(
            <CanvasConfigNodePanel
                node={node}
                isRunning={false}
                inputSummary={{ textCount: 0, imageCount: 0, videoCount: 0, audioCount: 0 }}
                onConfigChange={() => undefined}
                onGenerate={() => undefined}
                onStop={() => undefined}
                onComposerToggle={() => undefined}
            />,
        );
        expect(container.querySelector("[data-slot=select-trigger]")?.textContent).toContain("video-model");
        expect(container.textContent).not.toContain("image-model");
        cleanup({ container, root });
    });

    it("clears a config node model override when generation capability changes", () => {
        const node: CanvasNodeData = { id: "config-image", type: CanvasNodeType.Config, title: "图片", position: { x: 0, y: 0 }, width: 400, height: 300, metadata: { generationMode: "image", model: "image-model" } };
        const onConfigChange = vi.fn();
        const { container, root } = render(
            <CanvasConfigNodePanel
                node={node}
                isRunning={false}
                inputSummary={{ textCount: 0, imageCount: 0, videoCount: 0, audioCount: 0 }}
                onConfigChange={onConfigChange}
                onGenerate={() => undefined}
                onStop={() => undefined}
                onComposerToggle={() => undefined}
            />,
        );
        const videoOption = Array.from(container.querySelectorAll(".ant-segmented-item")).find((item) => item.textContent?.includes("视频"))!;
        click(videoOption);
        expect(onConfigChange).toHaveBeenCalledWith("config-image", expect.objectContaining({ generationMode: "video", model: undefined }));
        cleanup({ container, root });
    });

    it("uses videoSize after switching a config node from image to video", () => {
        const config = { ...configFixture(), size: "legacy-text-size", count: "7", imageSize: "16:9", imageCount: "4", videoSize: "720x1280" };
        useConfigStore.setState({ config });
        const node: CanvasNodeData = {
            id: "config-image",
            type: CanvasNodeType.Config,
            title: "图片配置",
            position: { x: 0, y: 0 },
            width: 400,
            height: 300,
            metadata: { generationMode: "image", model: "image-model", size: "16:9", count: 4, imageSize: "16:9", imageCount: 4 },
        };
        const onConfigChange = vi.fn();
        const { container, root } = render(
            <CanvasConfigNodePanel
                node={node}
                isRunning={false}
                inputSummary={{ textCount: 0, imageCount: 0, videoCount: 0, audioCount: 0 }}
                onConfigChange={onConfigChange}
                onGenerate={() => undefined}
                onStop={() => undefined}
                onComposerToggle={() => undefined}
            />,
        );

        const videoOption = Array.from(container.querySelectorAll(".ant-segmented-item")).find((item) => item.textContent?.includes("视频"))!;
        click(videoOption);
        const patch = onConfigChange.mock.calls[0][1];
        const switchedNode = { ...node, metadata: { ...node.metadata, ...patch } };
        popoverState.videoConfigs = [];
        act(() =>
            root.render(
                <CanvasConfigNodePanel
                    node={switchedNode}
                    isRunning={false}
                    inputSummary={{ textCount: 0, imageCount: 0, videoCount: 0, audioCount: 0 }}
                    onConfigChange={onConfigChange}
                    onGenerate={() => undefined}
                    onStop={() => undefined}
                    onComposerToggle={() => undefined}
                />,
            ),
        );

        expect(patch).toMatchObject({ generationMode: "video", model: undefined, size: undefined, count: undefined });
        expect(popoverState.videoConfigs.at(-1)).toMatchObject({ videoSize: "720x1280", size: "720x1280", count: "7" });
        cleanup({ container, root });
    });

    it("defaults config node to its generation capability model", () => {
        const node: CanvasNodeData = { id: "config-video", type: CanvasNodeType.Config, title: "视频", position: { x: 0, y: 0 }, width: 400, height: 300, metadata: { generationMode: "video" } };
        const { container, root } = render(
            <CanvasConfigNodePanel
                node={node}
                isRunning={false}
                inputSummary={{ textCount: 0, imageCount: 0, videoCount: 0, audioCount: 0 }}
                onConfigChange={() => undefined}
                onGenerate={() => undefined}
                onStop={() => undefined}
                onComposerToggle={() => undefined}
            />,
        );
        expect(container.querySelector("[data-slot=select-trigger]")?.textContent).toContain("video-model");
        cleanup({ container, root });
    });
});
