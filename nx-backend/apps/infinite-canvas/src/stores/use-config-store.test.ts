import { describe, expect, it, vi } from "vitest";

vi.hoisted(() => {
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
import { CONFIG_STORE_KEY, defaultConfig, isAiConfigReady, migrateConfigState, resolveCapabilityRequestConfig, updateCapabilityConfig, validateCapabilityConfig, useConfigStore, type AiConfig } from "./use-config-store";

describe("capability model config store", () => {
    it("persists capability credentials and dedicated parameters, then rehydrates them from localStorage", async () => {
        localStorage.clear();
        useConfigStore.setState({ config: structuredClone(defaultConfig) });
        useConfigStore.getState().updateCapabilityConfig("audio", { apiBase: "https://audio.persist/v1", apiKey: "persisted-audio-key", modelId: "audio-persisted" });
        useConfigStore.getState().updateConfig("audioVoice", "nova");
        useConfigStore.getState().updateConfig("audioFormat", "wav");

        const persisted = localStorage.getItem(CONFIG_STORE_KEY);
        expect(persisted).toContain("audio-persisted");
        expect(persisted).toContain("nova");

        useConfigStore.setState({ config: structuredClone(defaultConfig) });
        localStorage.setItem(CONFIG_STORE_KEY, persisted!);
        await useConfigStore.persist.rehydrate();

        expect(useConfigStore.getState().config.capabilityConfigs.audio).toMatchObject({ apiBase: "https://audio.persist/v1", apiKey: "persisted-audio-key", modelId: "audio-persisted" });
        expect(useConfigStore.getState().config).toMatchObject({ audioVoice: "nova", audioFormat: "wav" });
    });

    it("opens model settings at an explicit capability while keeping legacy arguments compatible", () => {
        useConfigStore.setState({ isConfigOpen: false, targetCapability: "image", shouldPromptContinue: false });
        useConfigStore.getState().openConfigDialog(true, "channels", "audio");
        expect(useConfigStore.getState()).toMatchObject({ isConfigOpen: true, shouldPromptContinue: true, configTab: "channels", targetCapability: "audio" });

        useConfigStore.getState().setConfigDialogOpen(false);
        useConfigStore.getState().openConfigDialog();
        expect(useConfigStore.getState().targetCapability).toBe("audio");
    });

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

    it("does not migrate a same-name channel model from the wrong capability", () => {
        const migrated = migrateConfigState({
            config: {
                channels: [{ id: "shared", baseUrl: "https://wrong", apiKey: "wrong-key", apiFormat: "openai", models: [{ name: "shared-model", capability: "text" }] }],
                imageModel: "shared::shared-model",
            },
        });
        expect(migrated.config.capabilityConfigs.image).toMatchObject({ apiBase: "", apiKey: "", modelId: "" });
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

    it("assigns a legacy flat model to its guessed capability without polluting other capabilities", () => {
        const migrated = migrateConfigState({
            config: {
                baseUrl: "https://legacy.example",
                apiKey: "legacy-key",
                apiFormat: "openai",
                model: "default::gpt-image-2",
            },
        });
        expect(migrated.config.capabilityConfigs.image).toMatchObject({ apiBase: "https://legacy.example", apiKey: "legacy-key", modelId: "gpt-image-2" });
        expect(migrated.config.capabilityConfigs.text.modelId).toBe("");
        expect(migrated.config.capabilityConfigs.video.modelId).toBe("");
        expect(migrated.config.capabilityConfigs.audio.modelId).toBe("");
    });

    it("keeps a partially damaged new config isolated instead of borrowing defaults or credentials", () => {
        const migrated = migrateConfigState({ config: { capabilityConfigs: { image: { apiKey: "only-image-key" } } } });
        expect(migrated.config.capabilityConfigs.image).toMatchObject({ apiBase: "", apiKey: "only-image-key", modelId: "" });
        expect(migrated.config.capabilityConfigs.video).toMatchObject({ apiBase: "", apiKey: "", modelId: "" });
    });

    it("treats an explicit empty capabilityConfigs object as new format", () => {
        const migrated = migrateConfigState({
            config: { capabilityConfigs: {}, baseUrl: "https://legacy", apiKey: "legacy-key", imageModel: "legacy-image" },
        });
        expect(migrated.config.capabilityConfigs.image).toMatchObject({ apiBase: "", apiKey: "", modelId: "" });
    });

    it("preserves an unknown persisted protocol so validation reports it", () => {
        const migrated = migrateConfigState({
            config: { capabilityConfigs: { video: { apiBase: "https://video", apiKey: "key", apiFormat: "future-protocol", modelId: "video-model" } } },
        });
        expect(migrated.config.capabilityConfigs.video.apiFormat).toBe("unknown");
        expect(validateCapabilityConfig("video", migrated.config.capabilityConfigs.video).map((error) => error.code)).toContain("unsupported_protocol");
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
        const imageRequest = resolveCapabilityRequestConfig(config, "image");
        expect(imageRequest).not.toHaveProperty("capabilityConfigs");
        expect(JSON.stringify(imageRequest)).not.toContain("video-key");
    });

    it("checks readiness against the explicit capability only", () => {
        const config: AiConfig = {
            ...defaultConfig,
            capabilityConfigs: {
                ...defaultConfig.capabilityConfigs,
                image: { ...defaultConfig.capabilityConfigs.image, apiBase: "", apiKey: "", modelId: "" },
                video: { ...defaultConfig.capabilityConfigs.video, apiBase: "https://video", apiKey: "video-key", modelId: "video-model" },
            },
        };
        expect(isAiConfigReady(config, "image", "video-model")).toBe(false);
        expect(isAiConfigReady(config, "video")).toBe(true);
    });

    it("reports capability-specific missing fields and unsupported protocols", () => {
        const invalid = { ...defaultConfig.capabilityConfigs.audio, apiBase: "", apiKey: "", modelId: "", apiFormat: "gemini" as const };
        const errors = validateCapabilityConfig("audio", invalid);
        expect(errors.map((error) => error.code)).toEqual(expect.arrayContaining(["missing_api_base", "missing_api_key", "missing_model_id", "unsupported_protocol"]));
        expect(errors.every((error) => error.capability === "audio")).toBe(true);
    });
});
