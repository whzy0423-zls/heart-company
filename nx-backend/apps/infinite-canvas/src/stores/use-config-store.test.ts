import { describe, expect, it } from "vitest";
import {
    defaultConfig,
    migrateConfigState,
    resolveCapabilityRequestConfig,
    updateCapabilityConfig,
    validateCapabilityConfig,
    type AiConfig,
} from "./use-config-store";

describe("capability model config store", () => {
    it("updates one capability without changing the other capability configs", () => {
        const before = structuredClone(defaultConfig);
        const after = updateCapabilityConfig(before, "image", { apiKey: "IMG_KEY", modelId: "image-v2" });
        expect(after.capabilityConfigs.image.apiKey).toBe("IMG_KEY");
        expect(after.capabilityConfigs.image.modelId).toBe("image-v2");
        expect(after.capabilityConfigs.video).toEqual(before.capabilityConfigs.video);
        expect(after.capabilityConfigs.text).toEqual(before.capabilityConfigs.text);
        expect(after.capabilityConfigs.audio).toEqual(before.capabilityConfigs.audio);
    });

    it("migrates channel credentials and decodes channelId::model", () => {
        const migrated = migrateConfigState({
            config: {
                channels: [
                    {
                        id: "img-channel",
                        name: "Images",
                        baseUrl: "https://img.example",
                        apiKey: "img-secret",
                        apiFormat: "gemini",
                        models: [{ name: "gemini-image", capability: "image", script: "return image" }],
                    },
                    {
                        id: "vid-channel",
                        baseUrl: "https://vid.example",
                        apiKey: "vid-secret",
                        apiFormat: "ark",
                        models: [{ name: "seedance-1", capability: "video" }],
                    },
                ],
                imageModel: "img-channel::gemini-image",
                videoModel: "vid-channel::seedance-1",
                textModel: "missing::text-model",
                audioModel: "audio-model",
            },
            webdav: { directory: "keep-me" },
        });
        expect(migrated.config.capabilityConfigs.image).toMatchObject({ apiBase: "https://img.example", apiKey: "img-secret", apiFormat: "gemini", modelId: "gemini-image", script: "return image" });
        expect(migrated.config.capabilityConfigs.video).toMatchObject({ apiBase: "https://vid.example", apiKey: "vid-secret", apiFormat: "ark", modelId: "seedance-1" });
        expect(migrated.config.capabilityConfigs.text.modelId).toBe("");
        expect(migrated.config.capabilityConfigs.audio.modelId).toBe("");
        expect(migrated.webdav.directory).toBe("keep-me");
        expect(migrateConfigState(migrated)).toEqual(migrated);
    });

    it("migrates the legacy flat config when channels are absent", () => {
        const migrated = migrateConfigState({
            config: {
                baseUrl: "https://legacy.example",
                apiKey: "legacy-key",
                apiFormat: "openai",
                model: "legacy-text",
                imageModel: "legacy-image",
                videoModel: "legacy-video",
                textModel: "legacy-text",
                audioModel: "legacy-audio",
            },
        });
        for (const capability of ["image", "video", "text", "audio"] as const) {
            expect(migrated.config.capabilityConfigs[capability]).toMatchObject({ apiBase: "https://legacy.example", apiKey: "legacy-key", apiFormat: "openai" });
        }
        expect(migrated.config.capabilityConfigs.image.modelId).toBe("legacy-image");
    });

    it("keeps a partially damaged new config isolated instead of borrowing defaults or credentials", () => {
        const migrated = migrateConfigState({ config: { capabilityConfigs: { image: { apiKey: "only-image-key" } } } });
        expect(migrated.config.capabilityConfigs.image).toMatchObject({ apiBase: "", apiKey: "only-image-key", modelId: "" });
        expect(migrated.config.capabilityConfigs.video).toMatchObject({ apiBase: "", apiKey: "", modelId: "" });
    });

    it("resolves only the requested capability and applies a model override", () => {
        const config: AiConfig = {
            ...defaultConfig,
            capabilityConfigs: {
                ...defaultConfig.capabilityConfigs,
                image: { ...defaultConfig.capabilityConfigs.image, apiBase: "https://image", apiKey: "image-key", modelId: "image-model" },
                video: { ...defaultConfig.capabilityConfigs.video, apiBase: "https://video", apiKey: "video-key", modelId: "video-model" },
            },
        };
        expect(resolveCapabilityRequestConfig(config, "image")).toMatchObject({ apiBase: "https://image", apiKey: "image-key", model: "image-model" });
        expect(resolveCapabilityRequestConfig(config, "video", "video-override")).toMatchObject({ apiBase: "https://video", apiKey: "video-key", model: "video-override" });
    });

    it("reports capability-specific missing fields and unsupported protocols", () => {
        const invalid = { ...defaultConfig.capabilityConfigs.audio, apiBase: "", apiKey: "", modelId: "", apiFormat: "gemini" as const };
        const errors = validateCapabilityConfig("audio", invalid);
        expect(errors.map((error) => error.code)).toEqual(expect.arrayContaining(["missing_api_base", "missing_api_key", "missing_model_id", "unsupported_protocol"]));
        expect(errors.every((error) => error.capability === "audio")).toBe(true);
    });
});
