import { dispatchCanvasGenerationRoute } from "@/lib/canvas/canvas-generation-helpers";
import { requestAudioGeneration } from "@/services/api/audio";
import { requestEdit, requestGeneration, requestImageQuestion, type AiTextMessage } from "@/services/api/image";
import { requestVideoGeneration } from "@/services/api/video";
import type { AiConfig } from "@/stores/use-config-store";
import type { ReferenceImage } from "@/types/image";
import type { ReferenceAudio, ReferenceVideo } from "@/types/media";

type RequestOptions = { signal?: AbortSignal };

export function requestCanvasImageBatch(config: AiConfig, prompt: string, options?: RequestOptions) {
    return dispatchCanvasGenerationRoute("image-batch", config, (snapshot) => requestGeneration(snapshot, prompt, options), config.model);
}

export function requestCanvasImageEdit(config: AiConfig, prompt: string, references: ReferenceImage[], mask?: ReferenceImage, options?: RequestOptions) {
    return dispatchCanvasGenerationRoute("image-edit", config, (snapshot) => requestEdit(snapshot, prompt, references, mask, options), config.model);
}

export function requestCanvasTextStream(config: AiConfig, messages: AiTextMessage[], onDelta: (text: string) => void, options?: RequestOptions) {
    return dispatchCanvasGenerationRoute("text-stream", config, (snapshot) => requestImageQuestion(snapshot, messages, onDelta, options), config.model);
}

export function requestCanvasImageQuestion(config: AiConfig, messages: AiTextMessage[], onDelta: (text: string) => void, options?: RequestOptions) {
    return dispatchCanvasGenerationRoute("image-question", config, (snapshot) => requestImageQuestion(snapshot, messages, onDelta, options), config.model);
}

export function requestCanvasVideoGeneration(config: AiConfig, prompt: string, references: ReferenceImage[] = [], videoReferences: ReferenceVideo[] = [], audioReferences: ReferenceAudio[] = [], options?: RequestOptions) {
    return dispatchCanvasGenerationRoute("video-generate", config, (snapshot) => requestVideoGeneration(snapshot, prompt, references, videoReferences, audioReferences, options), config.model);
}

export function requestCanvasVideoRetry(config: AiConfig, prompt: string, references: ReferenceImage[] = [], videoReferences: ReferenceVideo[] = [], audioReferences: ReferenceAudio[] = [], options?: RequestOptions) {
    return dispatchCanvasGenerationRoute("video-retry", config, (snapshot) => requestVideoGeneration(snapshot, prompt, references, videoReferences, audioReferences, options), config.model);
}

export function requestCanvasAudioGeneration(config: AiConfig, prompt: string, options?: RequestOptions) {
    return dispatchCanvasGenerationRoute("audio-generate", config, (snapshot) => requestAudioGeneration(snapshot, prompt, options), config.model);
}
