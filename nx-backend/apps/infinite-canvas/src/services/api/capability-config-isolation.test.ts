import { act, createElement } from "react";
import { createRoot } from "react-dom/client";
import { beforeEach, describe, expect, it, vi } from "vitest";
import axios from "axios";

import { createCapabilityRequestSnapshot, defaultConfig, type AiConfig } from "@/stores/use-config-store";
import { requestEdit, requestGeneration, requestImageQuestion } from "./image";
import { requestAudioGeneration } from "./audio";
import { createVideoGenerationTask, getVideoTaskResourceCountsForTest, pollVideoGenerationTask, releaseVideoGenerationTask, requestVideoGeneration, resetVideoTaskResourcesForTest } from "./video";
import { isSeedanceVideoConfig } from "@/lib/seedance-video";
import { buildGenerationConfig, canvasGenerationCapabilityForRoute, dispatchCanvasGenerationRoute, generationCapabilityForNodeType, type CanvasGenerationRoute } from "@/lib/canvas/canvas-generation-helpers";
import { CanvasNodeType } from "@/types/canvas";
import { VideoSettingsPanel } from "@/components/video-settings-panel";
import { canvasThemes } from "@/lib/canvas-theme";
import { requestCanvasAudioGeneration, requestCanvasImageBatch, requestCanvasImageEdit, requestCanvasImageQuestion, requestCanvasTextStream, requestCanvasVideoGeneration, requestCanvasVideoRetry } from "@/lib/canvas/canvas-generation-dispatch";
import { runModelPlugin } from "./model-plugin";

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
    vi.useRealTimers();
    resetVideoTaskResourcesForTest();
    vi.resetAllMocks();
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

    it("builds isolated runtime snapshots for every canvas generation route", () => {
        const config = isolatedConfig();
        const routes: Array<[CanvasGenerationRoute, "image" | "video" | "text" | "audio", string]> = [
            ["image-batch", "image", "IMAGE_SECRET"],
            ["image-edit", "image", "IMAGE_SECRET"],
            ["image-question", "text", "TEXT_SECRET"],
            ["text-stream", "text", "TEXT_SECRET"],
            ["video-generate", "video", "VIDEO_SECRET"],
            ["video-retry", "video", "VIDEO_SECRET"],
            ["audio-generate", "audio", "AUDIO_SECRET"],
        ];
        for (const [route, capability, key] of routes) {
            const snapshot = createCapabilityRequestSnapshot(config, canvasGenerationCapabilityForRoute(route));
            expect(canvasGenerationCapabilityForRoute(route)).toBe(capability);
            expect(snapshot.capability).toBe(capability);
            expect(snapshot.apiKey).toBe(key);
            expect(JSON.stringify(snapshot)).not.toContain(capability === "image" ? "VIDEO_SECRET" : "IMAGE_SECRET");
        }
    });

    it("dispatches a real capability snapshot to the selected canvas route handler", async () => {
        const handler = vi.fn(async (snapshot) => snapshot);
        const result = await dispatchCanvasGenerationRoute("video-retry", isolatedConfig(), handler);
        expect(handler).toHaveBeenCalledOnce();
        expect(handler.mock.calls[0][0]).toMatchObject({ capability: "video", apiKey: "VIDEO_SECRET", baseUrl: "https://video.example" });
        expect(result.capability).toBe("video");
    });

    it("maps every canvas generation output to one explicit capability", () => {
        expect(generationCapabilityForNodeType(CanvasNodeType.Image)).toBe("image");
        expect(generationCapabilityForNodeType(CanvasNodeType.Video)).toBe("video");
        expect(generationCapabilityForNodeType(CanvasNodeType.Text)).toBe("text");
        expect(generationCapabilityForNodeType(CanvasNodeType.Audio)).toBe("audio");
    });

    it("builds generation configs from capability-specific size and count fields", () => {
        const config = {
            ...isolatedConfig(),
            size: "legacy-text-size",
            count: "7",
            canvasImageCount: "8",
            imageSize: "2048x1152",
            imageCount: "4",
            videoSize: "720x1280",
        };

        expect(buildGenerationConfig(config, undefined, "image")).toMatchObject({ size: "2048x1152", count: "4" });
        expect(buildGenerationConfig(config, undefined, "video")).toMatchObject({ size: "720x1280", count: "7" });
        expect(buildGenerationConfig(config, undefined, "text")).toMatchObject({ size: "legacy-text-size", count: "7" });
    });

    it("uses only image credentials and script for generation and edit while decoding a legacy model override", async () => {
        const config = isolatedConfig();
        const generated = await requestCanvasImageBatch(config, "draw");
        const edited = await requestCanvasImageEdit(config, "edit", [{ id: "ref", name: "ref.png", type: "image/png", dataUrl: "data:image/png;base64,AA==" }]);

        expect(atob(generated[0].dataUrl.split(",")[1])).toBe("https://image.example|IMAGE_SECRET|node-image-model");
        expect(atob(edited[0].dataUrl.split(",")[1])).toBe("https://image.example|IMAGE_SECRET|node-image-model");
        expect(JSON.stringify([generated, edited])).not.toContain("TEXT_SECRET");
    });

    it("uses only text credentials and script for image questions", async () => {
        const config = isolatedConfig();
        config.model = "legacy-text::node-text-model";
        const answer = await requestCanvasImageQuestion(config, [{ role: "user", content: "what is this" }], vi.fn());
        expect(answer).toBe("https://text.example|TEXT_SECRET|node-text-model");
    });

    it("uses only audio credentials and script for audio generation", async () => {
        const config = isolatedConfig();
        config.model = "legacy-audio::node-audio-model";
        const audio = await requestCanvasAudioGeneration(config, "speak");
        expect(await audio.text()).toBe("https://audio.example|AUDIO_SECRET|node-audio-model");
    });

    it("routes real image generate and edit HTTP requests through image base, key, protocol, and model", async () => {
        const config = isolatedConfig();
        config.capabilityConfigs.image.script = "";
        mockedAxios.post.mockResolvedValueOnce({ data: { data: [{ b64_json: "AA==" }] } }).mockResolvedValueOnce({ data: { data: [{ b64_json: "AQ==" }] } });

        await requestCanvasImageBatch(config, "draw");
        await requestCanvasImageEdit(config, "edit", [{ id: "ref", name: "ref.png", type: "image/png", dataUrl: "data:image/png;base64,AA==" }]);

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

        const answer = await requestCanvasTextStream(config, [{ role: "user", content: "say hi" }], vi.fn());
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
        await requestCanvasAudioGeneration(config, "speak");
        expect(mockedAxios.post).toHaveBeenCalledWith(
            "https://audio.example/v1/audio/speech",
            expect.objectContaining({ model: "node-audio-model" }),
            expect.objectContaining({ headers: expect.objectContaining({ Authorization: "Bearer AUDIO_SECRET" }) }),
        );
    });

    it("routes project video generate and retry wrappers through video snapshots", async () => {
        const config = isolatedConfig();
        config.model = "project-video-model";
        config.capabilityConfigs.video.script = 'if (apiKey !== "VIDEO_SECRET" || baseUrl !== "https://video.example") throw new Error("wrong capability"); return "https://cdn.example/video.mp4"';
        expect(await requestCanvasVideoGeneration(config, "move")).toMatchObject({ url: "https://cdn.example/video.mp4" });
        expect(await requestCanvasVideoRetry(config, "move again")).toMatchObject({ url: "https://cdn.example/video.mp4" });
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

    it("refreshes a pending video snapshot so it survives beyond its initial TTL", async () => {
        vi.useFakeTimers();
        vi.setSystemTime(new Date("2026-01-01T00:00:00Z"));
        const config = isolatedConfig();
        config.model = "long-seedance";
        mockedAxios.post.mockResolvedValueOnce({ data: { id: "long-task", status: "queued" } });
        const task = await createVideoGenerationTask(config, "move");
        vi.advanceTimersByTime(29 * 60 * 1000);
        mockedAxios.get.mockResolvedValueOnce({ data: { id: "long-task", status: "running" } });
        expect((await pollVideoGenerationTask(config, task)).status).toBe("pending");
        vi.advanceTimersByTime(11 * 60 * 1000);
        mockedAxios.get.mockResolvedValueOnce({ data: { id: "long-task", status: "failed", error: { message: "finished" } } });
        expect(await pollVideoGenerationTask(config, task)).toEqual({ status: "failed", error: "finished" });
        expect(getVideoTaskResourceCountsForTest().snapshots).toBe(0);
        vi.useRealTimers();
    });

    it("retains the same video snapshot after a transient poll error for retry", async () => {
        const config = isolatedConfig();
        config.model = "retry-video";
        mockedAxios.post.mockResolvedValueOnce({ data: { id: "retry-task", status: "queued" } });
        const task = await createVideoGenerationTask(config, "move");
        mockedAxios.get.mockRejectedValueOnce(new Error("temporary network error"));
        await expect(pollVideoGenerationTask(config, task)).rejects.toThrow("temporary network error");
        expect(getVideoTaskResourceCountsForTest().snapshots).toBe(1);
        config.capabilityConfigs.video = { apiBase: "https://changed.example", apiKey: "CHANGED_SECRET", apiFormat: "openai", modelId: "changed" };
        mockedAxios.get.mockResolvedValueOnce({ data: { id: "retry-task", status: "failed", error: { message: "done" } } });
        expect(await pollVideoGenerationTask(config, task)).toEqual({ status: "failed", error: "done" });
        expect(mockedAxios.get.mock.calls[1][0]).toBe("https://video.example/v1/videos/retry-task");
        expect(mockedAxios.get.mock.calls[1][1]).toEqual(expect.objectContaining({ headers: expect.objectContaining({ Authorization: "Bearer VIDEO_SECRET" }) }));
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

    it("releases abandoned, terminal plugin, and failed-poll video resources", async () => {
        const abandonedConfig = isolatedConfig();
        abandonedConfig.model = "video-abandoned";
        mockedAxios.post.mockResolvedValueOnce({ data: { id: "abandoned", status: "queued" } });
        const abandoned = await createVideoGenerationTask(abandonedConfig, "move");
        expect(getVideoTaskResourceCountsForTest().snapshots).toBeGreaterThan(0);
        releaseVideoGenerationTask(abandoned);
        expect(getVideoTaskResourceCountsForTest().snapshots).toBe(0);

        const pluginConfig = isolatedConfig();
        pluginConfig.model = "video-plugin";
        pluginConfig.capabilityConfigs.video.script = 'return "https://cdn.example/video.mp4"';
        const pluginTask = await createVideoGenerationTask(pluginConfig, "move");
        expect(getVideoTaskResourceCountsForTest().pluginResults).toBe(1);
        expect((await pollVideoGenerationTask(pluginConfig, pluginTask)).status).toBe("completed");
        expect(getVideoTaskResourceCountsForTest().pluginResults).toBe(0);

        const failedConfig = isolatedConfig();
        failedConfig.model = "video-failed";
        mockedAxios.post.mockResolvedValueOnce({ data: { id: "failed", status: "queued" } });
        mockedAxios.get.mockRejectedValueOnce(new Error("poll failed"));
        const failedTask = await createVideoGenerationTask(failedConfig, "move");
        await expect(pollVideoGenerationTask(failedConfig, failedTask)).rejects.toThrow("poll failed");
        expect(getVideoTaskResourceCountsForTest().snapshots).toBe(1);
        releaseVideoGenerationTask(failedTask);
        expect(getVideoTaskResourceCountsForTest().snapshots).toBe(0);
    });

    it("releases the request snapshot when one-shot video polling times out", async () => {
        vi.useFakeTimers();
        const config = isolatedConfig();
        config.model = "video-timeout";
        mockedAxios.post.mockResolvedValueOnce({ data: { id: "timeout-task", status: "queued" } });
        mockedAxios.get.mockResolvedValue({ data: { id: "timeout-task", status: "running" } });
        const assertion = expect(requestVideoGeneration(config, "move")).rejects.toThrow("视频生成超时");
        await vi.runAllTimersAsync();
        await assertion;
        expect(getVideoTaskResourceCountsForTest().snapshots).toBe(0);
        vi.useRealTimers();
    });

    it("expires abandoned video credentials and plugin blobs after the resource TTL", async () => {
        vi.useFakeTimers();
        vi.setSystemTime(new Date("2026-01-01T00:00:00Z"));
        const config = isolatedConfig();
        config.model = "video-expiring";
        mockedAxios.post.mockResolvedValueOnce({ data: { id: "expiring", status: "queued" } });
        await createVideoGenerationTask(config, "move");
        const pluginConfig = isolatedConfig();
        pluginConfig.model = "plugin-expiring";
        pluginConfig.capabilityConfigs.video.script = 'return new Blob(["video"], { type: "video/mp4" })';
        await createVideoGenerationTask(pluginConfig, "move");
        expect(getVideoTaskResourceCountsForTest()).toEqual({ snapshots: 1, pluginResults: 1 });
        vi.advanceTimersByTime(30 * 60 * 1000 + 1);
        expect(getVideoTaskResourceCountsForTest()).toEqual({ snapshots: 0, pluginResults: 0 });
        vi.useRealTimers();
    });

    it("sanitizes API keys from plugin AbortError and axios cancellation while preserving cancellation semantics", async () => {
        const config = createCapabilityRequestSnapshot(isolatedConfig(), "text", "text-model");
        await expect(runModelPlugin({ capability: "text", script: 'throw new DOMException("TEXT_SECRET", "AbortError")', config })).rejects.toMatchObject({ name: "AbortError", message: "请求已取消" });
        mockedAxios.request.mockRejectedValueOnce(new Error("cancelled TEXT_SECRET"));
        mockedAxios.isCancel.mockReturnValueOnce(true);
        await expect(runModelPlugin({ capability: "text", script: 'return await http.get("/cancel")', config })).rejects.toMatchObject({ name: "AbortError", message: "请求已取消" });
    });

    it("sanitizes video AbortError and releases its request snapshot", async () => {
        const config = isolatedConfig();
        config.model = "video-abort";
        mockedAxios.post.mockResolvedValueOnce({ data: { id: "abort-task", status: "queued" } });
        mockedAxios.get.mockRejectedValueOnce(new DOMException("VIDEO_SECRET", "AbortError"));
        const task = await createVideoGenerationTask(config, "move");
        await expect(pollVideoGenerationTask(config, task)).rejects.toMatchObject({ name: "AbortError", message: "请求已取消" });
        expect(getVideoTaskResourceCountsForTest().snapshots).toBe(0);
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
