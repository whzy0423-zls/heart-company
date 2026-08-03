import { beforeEach, describe, expect, it, vi } from "vitest";
import axios from "axios";

import { defaultConfig, type AiConfig } from "@/stores/use-config-store";
import { requestEdit, requestGeneration, requestImageQuestion } from "./image";
import { requestAudioGeneration } from "./audio";
import { createVideoGenerationTask, pollVideoGenerationTask } from "./video";
import { isSeedanceVideoConfig } from "@/lib/seedance-video";
import { generationCapabilityForNodeType } from "@/lib/canvas/canvas-generation-helpers";
import { CanvasNodeType } from "@/types/canvas";

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
});

describe("capability request isolation", () => {
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

        expect(mockedAxios.post).toHaveBeenCalledWith(
            "https://video.example/v1/videos",
            expect.any(FormData),
            expect.objectContaining({ headers: expect.objectContaining({ Authorization: "Bearer VIDEO_SECRET" }) }),
        );
        expect(mockedAxios.get).toHaveBeenCalledWith(
            "https://video.example/v1/videos/task-1",
            expect.objectContaining({ headers: expect.objectContaining({ Authorization: "Bearer VIDEO_SECRET" }) }),
        );
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

    it("uses only the video protocol when identifying Seedance", () => {
        const config = isolatedConfig();
        config.apiFormat = "openai";
        config.capabilityConfigs.video.apiFormat = "ark";
        expect(isSeedanceVideoConfig(config)).toBe(true);
        config.capabilityConfigs.video.apiFormat = "gemini";
        expect(() => isSeedanceVideoConfig(config)).toThrow("视频模型不支持 gemini 协议");
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
