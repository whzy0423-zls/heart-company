import { readFileSync } from "node:fs";
import { act, createElement } from "react";
import { createRoot } from "react-dom/client";
import { beforeEach, describe, expect, it, vi } from "vitest";
import axios from "axios";

import { defaultConfig, type AiConfig } from "@/stores/use-config-store";
import { requestEdit, requestGeneration, requestImageQuestion } from "./image";
import { requestAudioGeneration } from "./audio";
import { createVideoGenerationTask, pollVideoGenerationTask } from "./video";
import { isSeedanceVideoConfig } from "@/lib/seedance-video";
import { generationCapabilityForNodeType } from "@/lib/canvas/canvas-generation-helpers";
import { CanvasNodeType } from "@/types/canvas";
import { VideoSettingsPanel } from "@/components/video-settings-panel";
import { canvasThemes } from "@/lib/canvas-theme";

vi.mock("axios", () => ({
    default: {
        post: vi.fn(),
        get: vi.fn(),
        request: vi.fn(),
        isCancel: vi.fn(() => false),
        isAxiosError: vi.fn(() => false),
    },
}));

const mockedAxios = vi.mocked(axios);

function isolatedConfig(): AiConfig {
    return {
        ...defaultConfig,
        model: "legacy-channel::node-image-model",
        imageModel: "legacy-image-channel::legacy-image-model",
        videoModel: "legacy-video-channel::legacy-video-model",
        textModel: "legacy-text-channel::legacy-text-model",
        audioModel: "legacy-audio-channel::legacy-audio-model",
        baseUrl: "https://flat.invalid",
        apiKey: "FLAT_SECRET",
        apiFormat: "gemini",
        channels: [
            {
                id: "legacy-channel",
                name: "legacy",
                baseUrl: "https://legacy.invalid",
                apiKey: "LEGACY_SECRET",
                apiFormat: "openai",
                models: [{ name: "node-image-model", capability: "image", script: "throw new Error('legacy script used')" }],
            },
        ],
        capabilityConfigs: {
            image: {
                apiBase: "https://image.example",
                apiKey: "IMAGE_SECRET",
                apiFormat: "openai",
                modelId: "image-default",
                script: "return [`data:image/png;base64,${btoa(`${baseUrl}|${apiKey}|${model}`)}`]",
            },
            video: { apiBase: "https://video.example", apiKey: "VIDEO_SECRET", apiFormat: "openai", modelId: "video-default" },
            text: {
                apiBase: "https://text.example",
                apiKey: "TEXT_SECRET",
                apiFormat: "openai",
                modelId: "text-default",
                script: "return `${baseUrl}|${apiKey}|${model}`",
            },
            audio: {
                apiBase: "https://audio.example",
                apiKey: "AUDIO_SECRET",
                apiFormat: "openai",
                modelId: "audio-default",
                script: "return new Blob([`${baseUrl}|${apiKey}|${model}`], { type: 'audio/mpeg' })",
            },
        },
    };
}

beforeEach(() => {
    vi.clearAllMocks();
    vi.unstubAllGlobals();
});

describe("capability request isolation", () => {
    it("renders the video settings panel while Seedance credentials are incomplete", () => {
        const config = isolatedConfig();
        config.capabilityConfigs.video = { apiBase: "https://video.example", apiKey: "", apiFormat: "ark", modelId: "seedance-2" };
        const container = document.createElement("div");
        document.body.appendChild(container);
        const root = createRoot(container);
        expect(() => act(() => root.render(createElement(VideoSettingsPanel, { config, onConfigChange: vi.fn(), theme: canvasThemes.light })))).not.toThrow();
        act(() => root.unmount());
        container.remove();
    });

    it("keeps project batch, text stream, and video retry call sites tied to explicit capabilities", () => {
        const source = readFileSync("src/pages/canvas/project.tsx", "utf8");
        expect(source).toContain("const generationConfig = buildGenerationConfig(effectiveConfig, sourceNode, mode);");
        expect(source).toMatch(/if \(mode === "image"\)[\s\S]*?targetIds\.map\([\s\S]*?request(?:Edit|Generation)\(\{ \.\.\.generationConfig/);
        expect(source).toMatch(/requestImageQuestion\(\s*generationConfig,[\s\S]*?buildNodeResponseMessages/);
        expect(source).toContain("const retryCapability = generationCapabilityForNodeType(node.type);");
        expect(source).toMatch(/if \(node\.type === CanvasNodeType\.Video\)[\s\S]*?requestVideoGeneration\(generationConfig/);
    });

    it("maps every canvas generation output to one explicit capability", () => {
        expect(generationCapabilityForNodeType(CanvasNodeType.Image)).toBe("image");
        expect(generationCapabilityForNodeType(CanvasNodeType.Video)).toBe("video");
        expect(generationCapabilityForNodeType(CanvasNodeType.Text)).toBe("text");
        expect(generationCapabilityForNodeType(CanvasNodeType.Audio)).toBe("audio");
    });

    it("uses only image credentials and script for generation and edit while decoding a legacy model override", async () => {
        const config = isolatedConfig();
        const generated = await requestGeneration(config, "draw");
        const edited = await requestEdit(config, "edit", [{ id: "ref", name: "ref.png", type: "image/png", dataUrl: "data:image/png;base64,AA==" }]);

        expect(atob(generated[0].dataUrl.split(",")[1])).toBe("https://image.example|IMAGE_SECRET|node-image-model");
        expect(atob(edited[0].dataUrl.split(",")[1])).toBe("https://image.example|IMAGE_SECRET|node-image-model");
        expect(JSON.stringify([generated, edited])).not.toContain("TEXT_SECRET");
    });

    it("uses only text credentials and script for image questions", async () => {
        const config = isolatedConfig();
        config.model = "legacy-text::node-text-model";
        const answer = await requestImageQuestion(config, [{ role: "user", content: "what is this" }], vi.fn());
        expect(answer).toBe("https://text.example|TEXT_SECRET|node-text-model");
    });

    it("uses only audio credentials and script for audio generation", async () => {
        const config = isolatedConfig();
        config.model = "legacy-audio::node-audio-model";
        const audio = await requestAudioGeneration(config, "speak");
        expect(await audio.text()).toBe("https://audio.example|AUDIO_SECRET|node-audio-model");
    });

    it("routes real image generate and edit HTTP requests through image base, key, protocol, and model", async () => {
        const config = isolatedConfig();
        config.capabilityConfigs.image.script = "";
        mockedAxios.post.mockResolvedValueOnce({ data: { data: [{ b64_json: "AA==" }] } }).mockResolvedValueOnce({ data: { data: [{ b64_json: "AQ==" }] } });

        await requestGeneration(config, "draw");
        await requestEdit(config, "edit", [{ id: "ref", name: "ref.png", type: "image/png", dataUrl: "data:image/png;base64,AA==" }]);

        const [generateUrl, generateBody, generateOptions] = mockedAxios.post.mock.calls[0];
        expect(generateUrl).toBe("https://image.example/v1/images/generations");
        expect(generateBody).toMatchObject({ model: "node-image-model" });
        expect(generateOptions).toEqual(expect.objectContaining({ headers: expect.objectContaining({ Authorization: "Bearer IMAGE_SECRET" }) }));
        const [editUrl, editBody, editOptions] = mockedAxios.post.mock.calls[1];
        expect(editUrl).toBe("https://image.example/v1/images/edits");
        expect(editBody).toBeInstanceOf(FormData);
        expect((editBody as FormData).get("model")).toBe("node-image-model");
        expect(editOptions).toEqual(expect.objectContaining({ headers: expect.objectContaining({ Authorization: "Bearer IMAGE_SECRET" }) }));
    });

    it("routes real text streaming through text base, key, protocol, and model", async () => {
        const config = isolatedConfig();
        config.model = "legacy-text::node-text-model";
        config.capabilityConfigs.text.script = "";
        const fetchMock = vi.fn().mockResolvedValue(
            new Response('data: {"type":"response.output_text.delta","delta":"hello"}\n\ndata: [DONE]\n\n', {
                status: 200,
                headers: { "Content-Type": "text/event-stream" },
            }),
        );
        vi.stubGlobal("fetch", fetchMock);

        const answer = await requestImageQuestion(config, [{ role: "user", content: "say hi" }], vi.fn());
        expect(answer).toBe("hello");
        expect(fetchMock).toHaveBeenCalledWith(
            "https://text.example/v1/responses",
            expect.objectContaining({
                headers: expect.objectContaining({ Authorization: "Bearer TEXT_SECRET" }),
                body: expect.stringContaining('"model":"node-text-model"'),
            }),
        );
    });

    it("routes real audio HTTP through audio base, key, protocol, and model", async () => {
        const config = isolatedConfig();
        config.model = "legacy-audio::node-audio-model";
        config.capabilityConfigs.audio.script = "";
        mockedAxios.post.mockResolvedValueOnce({ data: new Blob(["audio"], { type: "audio/mpeg" }) });
        await requestAudioGeneration(config, "speak");
        expect(mockedAxios.post).toHaveBeenCalledWith(
            "https://audio.example/v1/audio/speech",
            expect.objectContaining({ model: "node-audio-model" }),
            expect.objectContaining({ headers: expect.objectContaining({ Authorization: "Bearer AUDIO_SECRET" }) }),
        );
    });

    it("keeps the video request-start snapshot for polling after config changes", async () => {
        const config = isolatedConfig();
        config.model = "legacy-video::node-video-model";
        mockedAxios.post.mockResolvedValueOnce({ data: { id: "task-1", status: "queued" } });
        mockedAxios.get.mockResolvedValueOnce({ data: { id: "task-1", status: "running" } });

        const task = await createVideoGenerationTask(config, "move");
        config.capabilityConfigs.video = { apiBase: "https://changed.example", apiKey: "CHANGED_SECRET", apiFormat: "openai", modelId: "changed-model" };
        config.baseUrl = "https://also-changed.invalid";
        config.apiKey = "ALSO_CHANGED_SECRET";
        await pollVideoGenerationTask(config, task);

        expect(mockedAxios.post).toHaveBeenCalledWith("https://video.example/v1/videos", expect.any(FormData), expect.objectContaining({ headers: expect.objectContaining({ Authorization: "Bearer VIDEO_SECRET" }) }));
        expect(mockedAxios.get).toHaveBeenCalledWith("https://video.example/v1/videos/task-1", expect.objectContaining({ headers: expect.objectContaining({ Authorization: "Bearer VIDEO_SECRET" }) }));
    });

    it("does not collide video snapshots when different API bases return the same task id", async () => {
        const first = isolatedConfig();
        first.model = "first-video";
        const second = isolatedConfig();
        second.model = "second-video";
        second.capabilityConfigs.video = { apiBase: "https://video-two.example", apiKey: "VIDEO_TWO_SECRET", apiFormat: "openai", modelId: "video-two" };
        mockedAxios.post.mockResolvedValue({ data: { id: "task-1", status: "queued" } });
        const firstTask = await createVideoGenerationTask(first, "first");
        const secondTask = await createVideoGenerationTask(second, "second");
        mockedAxios.get.mockResolvedValue({ data: { id: "task-1", status: "running" } });

        await pollVideoGenerationTask(first, firstTask);
        await pollVideoGenerationTask(second, secondTask);
        expect(mockedAxios.get.mock.calls[0][0]).toBe("https://video.example/v1/videos/task-1");
        expect(mockedAxios.get.mock.calls[0][1]).toEqual(expect.objectContaining({ headers: expect.objectContaining({ Authorization: "Bearer VIDEO_SECRET" }) }));
        expect(mockedAxios.get.mock.calls[1][0]).toBe("https://video-two.example/v1/videos/task-1");
        expect(mockedAxios.get.mock.calls[1][1]).toEqual(expect.objectContaining({ headers: expect.objectContaining({ Authorization: "Bearer VIDEO_TWO_SECRET" }) }));
    });

    it("routes Seedance create and poll through the video capability snapshot", async () => {
        const config = isolatedConfig();
        config.model = "legacy-video::seedance-2";
        config.capabilityConfigs.video = { apiBase: "https://ark-video.example", apiKey: "ARK_VIDEO_SECRET", apiFormat: "ark", modelId: "seedance-2" };
        mockedAxios.post.mockResolvedValueOnce({ data: { id: "seedance-task", status: "queued" } });
        mockedAxios.get.mockResolvedValueOnce({ data: { id: "seedance-task", status: "running" } });
        const task = await createVideoGenerationTask(config, "move");
        await pollVideoGenerationTask(config, task);
        expect(mockedAxios.post).toHaveBeenCalledWith(
            "https://ark-video.example/v1/contents/generations/tasks",
            expect.objectContaining({ model: "seedance-2" }),
            expect.objectContaining({ headers: expect.objectContaining({ Authorization: "Bearer ARK_VIDEO_SECRET" }) }),
        );
        expect(mockedAxios.get).toHaveBeenCalledWith("https://ark-video.example/v1/contents/generations/tasks/seedance-task", expect.objectContaining({ headers: expect.objectContaining({ Authorization: "Bearer ARK_VIDEO_SECRET" }) }));
    });

    it("redacts the video key from failed polling states", async () => {
        const config = isolatedConfig();
        config.model = "node-video-model";
        mockedAxios.post.mockResolvedValueOnce({ data: { id: "task-secret", status: "queued" } });
        mockedAxios.get.mockResolvedValueOnce({ data: { id: "task-secret", status: "failed", error: { message: "echo VIDEO_SECRET" } } });
        const task = await createVideoGenerationTask(config, "move");
        const state = await pollVideoGenerationTask(config, task);
        expect(state).toEqual({ status: "failed", error: "echo [REDACTED]" });
    });

    it("identifies Seedance without validating incomplete video credentials", () => {
        const config = isolatedConfig();
        config.apiFormat = "openai";
        config.capabilityConfigs.video = { apiBase: "https://video.example", apiKey: "", apiFormat: "ark", modelId: "seedance-2" };
        expect(() => isSeedanceVideoConfig(config)).not.toThrow();
        expect(isSeedanceVideoConfig(config)).toBe(true);
        config.capabilityConfigs.video.apiFormat = "gemini";
        expect(isSeedanceVideoConfig(config)).toBe(false);
    });

    it("redacts the current capability API key from provider errors", async () => {
        const config = isolatedConfig();
        config.capabilityConfigs.image.script = "";
        mockedAxios.post.mockRejectedValueOnce(new Error("provider echoed IMAGE_SECRET"));
        await expect(requestGeneration(config, "fail")).rejects.not.toThrow("IMAGE_SECRET");
    });

    it("redacts the current capability API key from plugin exceptions", async () => {
        const config = isolatedConfig();
        config.capabilityConfigs.text.script = "throw new Error(`provider echoed ${apiKey}`)";
        await expect(requestImageQuestion(config, [{ role: "user", content: "fail" }], vi.fn())).rejects.not.toThrow("TEXT_SECRET");
    });
});
