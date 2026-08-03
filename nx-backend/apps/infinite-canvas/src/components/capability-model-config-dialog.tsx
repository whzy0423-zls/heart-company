import { useEffect, useState } from "react";
import { AudioLines, FileText, Image, KeyRound, Settings2, Video } from "lucide-react";
import { Button, Modal } from "antd";

import { useConfigStore, type ApiCallFormat, type CapabilityConfigs, type CapabilityModelConfig, type ModelCapability } from "@/stores/use-config-store";

const CAPABILITIES: ModelCapability[] = ["image", "video", "text", "audio"];
const INPUT_CLASS = "box-border block min-h-10 w-full rounded-lg border border-[var(--border)] bg-[var(--card)] px-3 py-2 text-sm text-[var(--foreground)] shadow-sm outline-none transition placeholder:text-[var(--muted-foreground)] focus:border-[var(--primary)]";

const CAPABILITY_META: Record<ModelCapability, { label: string; description: string; icon: typeof Image; protocols: ApiCallFormat[] }> = {
    image: { label: "图片", description: "生图、图片编辑与图片理解", icon: Image, protocols: ["openai", "gemini", "ark"] },
    video: { label: "视频", description: "视频生成与任务查询", icon: Video, protocols: ["openai", "ark"] },
    text: { label: "文本", description: "文本生成与流式对话", icon: FileText, protocols: ["openai", "gemini", "ark"] },
    audio: { label: "音频", description: "语音与音频生成", icon: AudioLines, protocols: ["openai", "ark"] },
};

function copyConfigs(configs: CapabilityConfigs): CapabilityConfigs {
    return {
        image: { ...configs.image },
        video: { ...configs.video },
        text: { ...configs.text },
        audio: { ...configs.audio },
    };
}

export function CapabilityModelConfigDialog() {
    const open = useConfigStore((state) => state.isConfigOpen);
    const configs = useConfigStore((state) => state.config.capabilityConfigs);
    const targetCapability = useConfigStore((state) => state.targetCapability);
    const updateCapabilityConfig = useConfigStore((state) => state.updateCapabilityConfig);
    const setTargetCapability = useConfigStore((state) => state.setTargetCapability);
    const setConfigDialogOpen = useConfigStore((state) => state.setConfigDialogOpen);
    const [drafts, setDrafts] = useState<CapabilityConfigs>(() => copyConfigs(configs));

    useEffect(() => {
        if (open) setDrafts(copyConfigs(configs));
    }, [open, configs]);

    const activeDraft = drafts[targetCapability];
    const meta = CAPABILITY_META[targetCapability];

    const updateDraft = (patch: Partial<CapabilityModelConfig>) => {
        setDrafts((current) => ({
            ...current,
            [targetCapability]: { ...current[targetCapability], ...patch },
        }));
    };

    const save = () => {
        updateCapabilityConfig(targetCapability, {
            ...activeDraft,
            apiBase: activeDraft.apiBase.trim(),
            modelId: activeDraft.modelId.trim(),
            script: activeDraft.script?.trim() || undefined,
        });
        setConfigDialogOpen(false);
    };

    return (
        <Modal
            title={
                <span className="flex items-center gap-2">
                    <Settings2 className="size-5" />
                    无限画布模型配置
                </span>
            }
            open={open}
            onCancel={() => setConfigDialogOpen(false)}
            footer={
                <div className="flex items-center justify-between gap-3">
                    <span className="text-xs text-[var(--muted-foreground)]">配置仅保存在当前浏览器，各能力互不影响</span>
                    <div className="flex gap-2">
                        <Button onClick={() => setConfigDialogOpen(false)}>取消</Button>
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
                <nav className="grid content-start gap-2" aria-label="模型能力">
                    {CAPABILITIES.map((capability) => {
                        const item = CAPABILITY_META[capability];
                        const Icon = item.icon;
                        const active = capability === targetCapability;
                        return (
                            <button
                                key={capability}
                                type="button"
                                data-capability-tab={capability}
                                aria-selected={active}
                                onClick={() => setTargetCapability(capability)}
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

                <section aria-label={`${meta.label}模型配置`} className="min-w-0 space-y-4">
                    <header>
                        <h3 className="m-0 text-base font-semibold">{meta.label}模型</h3>
                        <p className="mb-0 mt-1 text-sm opacity-55">{meta.description}</p>
                    </header>

                    <Field label="API Base">
                        <input
                            data-testid={`${targetCapability}-api-base`}
                            value={activeDraft.apiBase}
                            onChange={(event) => updateDraft({ apiBase: event.target.value })}
                            placeholder="https://api.example.com/v1"
                            className={INPUT_CLASS}
                            autoComplete="url"
                        />
                    </Field>

                    <Field label="API Key" icon={<KeyRound className="size-3.5" />}>
                        <input
                            data-testid={`${targetCapability}-api-key`}
                            type="password"
                            value={activeDraft.apiKey}
                            onChange={(event) => updateDraft({ apiKey: event.target.value })}
                            placeholder="输入当前能力专用的 API Key"
                            className={INPUT_CLASS}
                            autoComplete="new-password"
                        />
                    </Field>

                    <div className="grid gap-4 sm:grid-cols-[160px_minmax(0,1fr)]">
                        <Field label="协议">
                            <select data-testid={`${targetCapability}-api-format`} value={activeDraft.apiFormat} onChange={(event) => updateDraft({ apiFormat: event.target.value as ApiCallFormat })} className={INPUT_CLASS}>
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
                            />
                        </Field>
                    </div>

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
    );
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
