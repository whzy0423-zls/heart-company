import { useEffect, useState } from "react";
import { AudioLines, Eye, EyeOff, FileText, Image, KeyRound, Settings2, Video } from "lucide-react";
import { Alert, Button, Modal, message } from "antd";

import { useConfigStore, validateCapabilityConfig, type AiConfig, type ApiCallFormat, type CapabilityConfigs, type CapabilityModelConfig, type ModelCapability, type ReasoningEffort } from "@/stores/use-config-store";

const CAPABILITIES: ModelCapability[] = ["image", "video", "text", "audio"];
const INPUT_CLASS =
    "box-border block min-h-10 w-full rounded-lg border border-[var(--border)] bg-[var(--card)] px-3 py-2 text-sm text-[var(--foreground)] shadow-sm outline-none transition placeholder:text-[var(--muted-foreground)] focus:border-[var(--primary)]";

const CAPABILITY_META: Record<ModelCapability, { label: string; description: string; icon: typeof Image; protocols: ApiCallFormat[] }> = {
    image: { label: "图片", description: "生图、图片编辑与图片理解", icon: Image, protocols: ["openai", "gemini", "ark"] },
    video: { label: "视频", description: "视频生成与任务查询", icon: Video, protocols: ["openai", "ark"] },
    text: { label: "文本", description: "文本生成与流式对话", icon: FileText, protocols: ["openai", "gemini", "ark"] },
    audio: { label: "音频", description: "语音与音频生成", icon: AudioLines, protocols: ["openai", "ark"] },
};

type CapabilityParameterDrafts = {
    image: Pick<AiConfig, "quality" | "size" | "background" | "count">;
    video: Pick<AiConfig, "videoSeconds" | "vquality" | "videoGenerateAudio" | "videoWatermark">;
    text: Pick<AiConfig, "systemPrompt" | "reasoningEffort">;
    audio: Pick<AiConfig, "audioVoice" | "audioFormat" | "audioSpeed" | "audioInstructions">;
};

type ValidationIssue = {
    code: "missing_api_base" | "missing_api_key" | "missing_model_id" | "unsupported_protocol";
    message: string;
};

const ERROR_FIELD_BY_CODE: Record<ValidationIssue["code"], string> = {
    missing_api_base: "api-base",
    missing_api_key: "api-key",
    missing_model_id: "model-id",
    unsupported_protocol: "api-format",
};

function copyConfigs(configs: CapabilityConfigs): CapabilityConfigs {
    return {
        image: { ...configs.image },
        video: { ...configs.video },
        text: { ...configs.text },
        audio: { ...configs.audio },
    };
}

function copyParameterDrafts(config: AiConfig): CapabilityParameterDrafts {
    return {
        image: { quality: config.quality, size: config.size, background: config.background, count: config.count },
        video: { videoSeconds: config.videoSeconds, vquality: config.vquality, videoGenerateAudio: config.videoGenerateAudio, videoWatermark: config.videoWatermark },
        text: { systemPrompt: config.systemPrompt, reasoningEffort: config.reasoningEffort },
        audio: { audioVoice: config.audioVoice, audioFormat: config.audioFormat, audioSpeed: config.audioSpeed, audioInstructions: config.audioInstructions },
    };
}

export function CapabilityModelConfigDialog() {
    const [messageApi, messageContext] = message.useMessage();
    const open = useConfigStore((state) => state.isConfigOpen);
    const config = useConfigStore((state) => state.config);
    const configs = config.capabilityConfigs;
    const targetCapability = useConfigStore((state) => state.targetCapability);
    const shouldPromptContinue = useConfigStore((state) => state.shouldPromptContinue);
    const updateCapabilityConfig = useConfigStore((state) => state.updateCapabilityConfig);
    const updateConfig = useConfigStore((state) => state.updateConfig);
    const setTargetCapability = useConfigStore((state) => state.setTargetCapability);
    const setConfigDialogOpen = useConfigStore((state) => state.setConfigDialogOpen);
    const clearPromptContinue = useConfigStore((state) => state.clearPromptContinue);
    const [drafts, setDrafts] = useState<CapabilityConfigs>(() => copyConfigs(configs));
    const [parameterDrafts, setParameterDrafts] = useState<CapabilityParameterDrafts>(() => copyParameterDrafts(config));
    const [visibleApiKeys, setVisibleApiKeys] = useState<Record<ModelCapability, boolean>>({ image: false, video: false, text: false, audio: false });
    const [validationErrors, setValidationErrors] = useState<ValidationIssue[]>([]);

    useEffect(() => {
        if (!open) return;
        setDrafts(copyConfigs(configs));
        setParameterDrafts(copyParameterDrafts(config));
        setVisibleApiKeys({ image: false, video: false, text: false, audio: false });
        setValidationErrors([]);
    }, [open, config, configs]);

    useEffect(() => {
        const firstError = validationErrors[0];
        if (!open || !firstError) return;
        const field = document.querySelector(`[data-testid="${targetCapability}-${ERROR_FIELD_BY_CODE[firstError.code]}"]`);
        if (field instanceof HTMLElement) field.focus();
    }, [open, targetCapability, validationErrors]);

    const activeDraft = drafts[targetCapability];
    const meta = CAPABILITY_META[targetCapability];

    const updateDraft = (patch: Partial<CapabilityModelConfig>) => {
        setValidationErrors([]);
        setDrafts((current) => ({
            ...current,
            [targetCapability]: { ...current[targetCapability], ...patch },
        }));
    };

    const updateParameters = <C extends ModelCapability>(capability: C, patch: Partial<CapabilityParameterDrafts[C]>) => {
        setParameterDrafts((current) => ({ ...current, [capability]: { ...current[capability], ...patch } }));
    };

    const selectCapability = (capability: ModelCapability) => {
        setValidationErrors([]);
        setTargetCapability(capability);
    };

    const handleCapabilityKeyDown = (event: React.KeyboardEvent<HTMLButtonElement>, index: number) => {
        let nextIndex: number | undefined;
        if (event.key === "ArrowRight" || event.key === "ArrowDown") nextIndex = (index + 1) % CAPABILITIES.length;
        if (event.key === "ArrowLeft" || event.key === "ArrowUp") nextIndex = (index - 1 + CAPABILITIES.length) % CAPABILITIES.length;
        if (event.key === "Home") nextIndex = 0;
        if (event.key === "End") nextIndex = CAPABILITIES.length - 1;
        if (nextIndex === undefined) return;
        event.preventDefault();
        const nextCapability = CAPABILITIES[nextIndex];
        selectCapability(nextCapability);
        document.getElementById(`capability-tab-${nextCapability}`)?.focus();
    };

    const cancel = () => {
        clearPromptContinue();
        setValidationErrors([]);
        setConfigDialogOpen(false);
    };

    const save = () => {
        const nextCapabilityConfig = {
            ...activeDraft,
            apiBase: activeDraft.apiBase.trim(),
            modelId: activeDraft.modelId.trim(),
            script: activeDraft.script?.trim() || undefined,
        };
        const errors = validateCapabilityConfig(targetCapability, nextCapabilityConfig);
        if (errors.length) {
            setValidationErrors(errors.map((error) => ({ code: error.code, message: error.message })));
            return;
        }

        updateCapabilityConfig(targetCapability, nextCapabilityConfig);
        if (targetCapability === "image") {
            const values = parameterDrafts.image;
            updateConfig("quality", values.quality);
            updateConfig("size", values.size);
            updateConfig("background", values.background);
            updateConfig("count", values.count);
        } else if (targetCapability === "video") {
            const values = parameterDrafts.video;
            updateConfig("videoSeconds", values.videoSeconds);
            updateConfig("vquality", values.vquality);
            updateConfig("videoGenerateAudio", values.videoGenerateAudio);
            updateConfig("videoWatermark", values.videoWatermark);
        } else if (targetCapability === "text") {
            const values = parameterDrafts.text;
            updateConfig("systemPrompt", values.systemPrompt);
            updateConfig("reasoningEffort", values.reasoningEffort);
        } else {
            const values = parameterDrafts.audio;
            updateConfig("audioVoice", values.audioVoice);
            updateConfig("audioFormat", values.audioFormat);
            updateConfig("audioSpeed", values.audioSpeed);
            updateConfig("audioInstructions", values.audioInstructions);
        }

        const feedback = shouldPromptContinue ? "配置已保存，请重新执行刚才的生成操作" : `${meta.label}模型配置已保存`;
        clearPromptContinue();
        setConfigDialogOpen(false);
        messageApi.success(feedback);
    };

    return (
        <>
            {messageContext}
            <Modal
                title={
                    <span className="flex items-center gap-2">
                        <Settings2 className="size-5" />
                        无限画布模型配置
                    </span>
                }
                open={open}
                onCancel={cancel}
                footer={
                    <div className="flex items-center justify-between gap-3">
                        <span className="text-xs text-[var(--muted-foreground)]">配置仅保存在当前浏览器，各能力互不影响</span>
                        <div className="flex gap-2">
                            <Button data-testid="cancel-capability-config" onClick={cancel}>
                                取消
                            </Button>
                            <Button type="primary" data-testid="save-capability-config" onClick={save}>
                                保存{meta.label}配置
                            </Button>
                        </div>
                    </div>
                }
                width={760}
                centered
                destroyOnHidden
            >
                <div className="grid gap-5 border-t pt-5 md:grid-cols-[180px_minmax(0,1fr)]">
                    <nav className="grid content-start gap-2" aria-label="模型能力" role="tablist">
                        {CAPABILITIES.map((capability, index) => {
                            const item = CAPABILITY_META[capability];
                            const Icon = item.icon;
                            const active = capability === targetCapability;
                            return (
                                <button
                                    key={capability}
                                    type="button"
                                    id={`capability-tab-${capability}`}
                                    role="tab"
                                    data-capability-tab={capability}
                                    aria-selected={active}
                                    aria-controls={`capability-panel-${capability}`}
                                    tabIndex={active ? 0 : -1}
                                    onClick={() => selectCapability(capability)}
                                    onKeyDown={(event) => handleCapabilityKeyDown(event, index)}
                                    className={`flex items-center gap-3 rounded-xl border px-3 py-3 text-left transition ${active ? "border-[var(--primary)] bg-[var(--accent)]" : "border-transparent hover:bg-[var(--muted)]"}`}
                                >
                                    <span className="grid size-9 shrink-0 place-items-center rounded-lg bg-[var(--card)] shadow-sm">
                                        <Icon className="size-4" />
                                    </span>
                                    <span>
                                        <strong className="block text-sm">{item.label}</strong>
                                        <span className="text-xs opacity-55">独立配置</span>
                                    </span>
                                </button>
                            );
                        })}
                    </nav>

                    <section id={`capability-panel-${targetCapability}`} role="tabpanel" aria-labelledby={`capability-tab-${targetCapability}`} className="min-w-0 space-y-4">
                        <header>
                            <h3 className="m-0 text-base font-semibold">{meta.label}模型</h3>
                            <p className="mb-0 mt-1 text-sm opacity-55">{meta.description}</p>
                        </header>

                        {validationErrors.length ? (
                            <Alert
                                type="error"
                                showIcon
                                role="alert"
                                aria-live="assertive"
                                title={`${meta.label}模型配置未保存`}
                                description={
                                    <ul className="mb-0 mt-1 list-disc pl-5">
                                        {validationErrors.map((error) => (
                                            <li key={error.code} id={`${targetCapability}-${ERROR_FIELD_BY_CODE[error.code]}-error`}>
                                                {error.message}
                                            </li>
                                        ))}
                                    </ul>
                                }
                            />
                        ) : null}

                        <Field label="API Base">
                            <input
                                data-testid={`${targetCapability}-api-base`}
                                value={activeDraft.apiBase}
                                onChange={(event) => updateDraft({ apiBase: event.target.value })}
                                placeholder="https://api.example.com/v1"
                                className={INPUT_CLASS}
                                autoComplete="url"
                                aria-invalid={validationErrors.some((error) => error.code === "missing_api_base") || undefined}
                                aria-describedby={validationErrors.some((error) => error.code === "missing_api_base") ? `${targetCapability}-api-base-error` : undefined}
                            />
                        </Field>

                        <Field label="API Key" icon={<KeyRound className="size-3.5" />}>
                            <div className="relative">
                                <input
                                    data-testid={`${targetCapability}-api-key`}
                                    type={visibleApiKeys[targetCapability] ? "text" : "password"}
                                    value={activeDraft.apiKey}
                                    onChange={(event) => updateDraft({ apiKey: event.target.value })}
                                    placeholder="输入当前能力专用的 API Key"
                                    className={`${INPUT_CLASS} pr-11`}
                                    autoComplete="new-password"
                                    aria-invalid={validationErrors.some((error) => error.code === "missing_api_key") || undefined}
                                    aria-describedby={validationErrors.some((error) => error.code === "missing_api_key") ? `${targetCapability}-api-key-error` : undefined}
                                />
                                <button
                                    type="button"
                                    data-testid={`${targetCapability}-api-key-visibility`}
                                    aria-label={visibleApiKeys[targetCapability] ? "隐藏 API Key" : "显示 API Key"}
                                    onClick={() => setVisibleApiKeys((current) => ({ ...current, [targetCapability]: !current[targetCapability] }))}
                                    className="absolute right-1 top-1 grid size-8 place-items-center rounded-md opacity-55 transition hover:bg-[var(--muted)] hover:opacity-100"
                                >
                                    {visibleApiKeys[targetCapability] ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
                                </button>
                            </div>
                        </Field>

                        <div className="grid gap-4 sm:grid-cols-[160px_minmax(0,1fr)]">
                            <Field label="协议">
                                <select
                                    data-testid={`${targetCapability}-api-format`}
                                    value={activeDraft.apiFormat}
                                    onChange={(event) => updateDraft({ apiFormat: event.target.value as ApiCallFormat })}
                                    className={INPUT_CLASS}
                                    aria-invalid={validationErrors.some((error) => error.code === "unsupported_protocol") || undefined}
                                    aria-describedby={validationErrors.some((error) => error.code === "unsupported_protocol") ? `${targetCapability}-api-format-error` : undefined}
                                >
                                    {!meta.protocols.includes(activeDraft.apiFormat) ? <option value={activeDraft.apiFormat}>不支持：{activeDraft.apiFormat}</option> : null}
                                    {meta.protocols.map((protocol) => (
                                        <option key={protocol} value={protocol}>
                                            {protocol === "openai" ? "OpenAI 兼容" : protocol === "gemini" ? "Gemini" : "火山方舟 / Ark"}
                                        </option>
                                    ))}
                                </select>
                            </Field>
                            <Field label="模型 ID">
                                <input
                                    data-testid={`${targetCapability}-model-id`}
                                    value={activeDraft.modelId}
                                    onChange={(event) => updateDraft({ modelId: event.target.value })}
                                    placeholder="例如 model-id"
                                    className={INPUT_CLASS}
                                    autoComplete="off"
                                    aria-invalid={validationErrors.some((error) => error.code === "missing_model_id") || undefined}
                                    aria-describedby={validationErrors.some((error) => error.code === "missing_model_id") ? `${targetCapability}-model-id-error` : undefined}
                                />
                            </Field>
                        </div>

                        <CapabilityParameters capability={targetCapability} drafts={parameterDrafts} onChange={updateParameters} />

                        <details data-testid={`${targetCapability}-advanced`} className="group rounded-xl border border-[var(--border)] bg-[var(--muted)]/35 px-4 py-3">
                            <summary className="cursor-pointer select-none text-sm font-medium">高级：自定义请求脚本（可选）</summary>
                            <div className="pt-3">
                                <textarea
                                    data-testid={`${targetCapability}-script`}
                                    value={activeDraft.script || ""}
                                    onChange={(event) => updateDraft({ script: event.target.value })}
                                    placeholder="仅在当前能力请求时使用"
                                    rows={6}
                                    spellCheck={false}
                                    className={`${INPUT_CLASS} resize-y font-mono text-xs`}
                                />
                                <p className="mb-0 mt-2 text-xs opacity-50">脚本与密钥只属于当前能力，不会复用到其他模型类型。</p>
                            </div>
                        </details>
                    </section>
                </div>
            </Modal>
        </>
    );
}

function CapabilityParameters({ capability, drafts, onChange }: { capability: ModelCapability; drafts: CapabilityParameterDrafts; onChange: <C extends ModelCapability>(capability: C, patch: Partial<CapabilityParameterDrafts[C]>) => void }) {
    if (capability === "image") {
        return (
            <div className="grid gap-4 sm:grid-cols-2">
                <Field label="图片质量">
                    <select data-testid="image-quality" value={drafts.image.quality} onChange={(event) => onChange("image", { quality: event.target.value })} className={INPUT_CLASS}>
                        {optionList(["auto", "low", "medium", "high"])}
                    </select>
                </Field>
                <Field label="图片尺寸">
                    <input data-testid="image-size" value={drafts.image.size} onChange={(event) => onChange("image", { size: event.target.value })} className={INPUT_CLASS} placeholder="例如 1:1 或 1024x1024" />
                </Field>
                <Field label="背景">
                    <select data-testid="image-background" value={drafts.image.background} onChange={(event) => onChange("image", { background: event.target.value })} className={INPUT_CLASS}>
                        <option value="">默认</option>
                        <option value="opaque">不透明</option>
                        <option value="transparent">透明</option>
                    </select>
                </Field>
                <Field label="生成数量">
                    <input data-testid="image-count" type="number" min="1" max="10" value={drafts.image.count} onChange={(event) => onChange("image", { count: event.target.value })} className={INPUT_CLASS} />
                </Field>
            </div>
        );
    }
    if (capability === "video") {
        return (
            <div className="grid gap-4 sm:grid-cols-2">
                <Field label="视频时长（秒）">
                    <input data-testid="video-seconds" type="number" min="1" value={drafts.video.videoSeconds} onChange={(event) => onChange("video", { videoSeconds: event.target.value })} className={INPUT_CLASS} />
                </Field>
                <Field label="视频质量">
                    <input data-testid="video-quality" value={drafts.video.vquality} onChange={(event) => onChange("video", { vquality: event.target.value })} className={INPUT_CLASS} placeholder="例如 720 或 1080" />
                </Field>
                <Field label="生成音频">
                    <select data-testid="video-generate-audio" value={drafts.video.videoGenerateAudio} onChange={(event) => onChange("video", { videoGenerateAudio: event.target.value })} className={INPUT_CLASS}>
                        <option value="true">是</option>
                        <option value="false">否</option>
                    </select>
                </Field>
                <Field label="添加水印">
                    <select data-testid="video-watermark" value={drafts.video.videoWatermark} onChange={(event) => onChange("video", { videoWatermark: event.target.value })} className={INPUT_CLASS}>
                        <option value="false">否</option>
                        <option value="true">是</option>
                    </select>
                </Field>
            </div>
        );
    }
    if (capability === "text") {
        return (
            <div className="space-y-4">
                <Field label="系统提示词">
                    <textarea data-testid="text-system-prompt" rows={3} value={drafts.text.systemPrompt} onChange={(event) => onChange("text", { systemPrompt: event.target.value })} className={`${INPUT_CLASS} resize-y`} />
                </Field>
                <Field label="推理强度">
                    <select data-testid="text-reasoning-effort" value={drafts.text.reasoningEffort} onChange={(event) => onChange("text", { reasoningEffort: event.target.value as ReasoningEffort })} className={INPUT_CLASS}>
                        {optionList(["auto", "low", "medium", "high", "xhigh"])}
                    </select>
                </Field>
            </div>
        );
    }
    return (
        <div className="grid gap-4 sm:grid-cols-2">
            <Field label="音色">
                <input data-testid="audio-voice" value={drafts.audio.audioVoice} onChange={(event) => onChange("audio", { audioVoice: event.target.value })} className={INPUT_CLASS} placeholder="例如 alloy" />
            </Field>
            <Field label="音频格式">
                <select data-testid="audio-format" value={drafts.audio.audioFormat} onChange={(event) => onChange("audio", { audioFormat: event.target.value })} className={INPUT_CLASS}>
                    {optionList(["mp3", "wav", "opus", "aac", "flac"])}
                </select>
            </Field>
            <Field label="语速">
                <input data-testid="audio-speed" type="number" min="0.25" max="4" step="0.05" value={drafts.audio.audioSpeed} onChange={(event) => onChange("audio", { audioSpeed: event.target.value })} className={INPUT_CLASS} />
            </Field>
            <Field label="朗读指令">
                <textarea data-testid="audio-instructions" rows={2} value={drafts.audio.audioInstructions} onChange={(event) => onChange("audio", { audioInstructions: event.target.value })} className={`${INPUT_CLASS} resize-y`} />
            </Field>
        </div>
    );
}

function optionList(values: string[]) {
    return values.map((value) => (
        <option key={value} value={value}>
            {value}
        </option>
    ));
}

function Field({ label, icon, children }: { label: string; icon?: React.ReactNode; children: React.ReactNode }) {
    return (
        <label className="block space-y-1.5">
            <span className="flex items-center gap-1.5 text-xs font-medium opacity-65">
                {icon}
                {label}
            </span>
            {children}
        </label>
    );
}
