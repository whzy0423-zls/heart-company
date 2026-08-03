import { useEffect, useId, useMemo, useState } from "react";
import { Cpu } from "lucide-react";

import { Select, SelectContent, SelectItem, SelectTrigger } from "@/components/ui/select";
import { cn } from "@/lib/utils";
import { isAiConfigReady, modelOptionLabel, modelOptionName, selectableModelsByCapability, type AiConfig, type ModelCapability } from "@/stores/use-config-store";

type ModelPickerProps = {
    config: AiConfig;
    value?: string;
    onChange: (model?: string) => void;
    capability?: ModelCapability;
    className?: string;
    fullWidth?: boolean;
    placeholder?: string;
    onMissingConfig?: () => void;
};

const CAPABILITY_DEFAULT_OPTION = "__capability_default__";

export function ModelPicker({ config, value, onChange, capability, className, fullWidth = false, placeholder = "选择模型", onMissingConfig }: ModelPickerProps) {
    const pickerId = useId();
    const [open, setOpen] = useState(false);
    const configuredModel = capability ? config.capabilityConfigs?.[capability]?.modelId?.trim() || "" : "";
    const options = useMemo(() => {
        if (capability) {
            return Array.from(new Set([...(configuredModel ? [CAPABILITY_DEFAULT_OPTION] : []), value].filter((model): model is string => Boolean(model))));
        }
        return Array.from(new Set([...(config.channelMode === "local" ? [value] : []), ...selectableModelsByCapability(config)].filter((model): model is string => Boolean(model))));
    }, [capability, config, configuredModel, value]);
    const current = value || configuredModel;
    const selectedValue = capability ? value || CAPABILITY_DEFAULT_OPTION : current;
    const displayLabel = current ? (capability ? modelOptionName(current) : modelOptionLabel(config, current)) : placeholder;

    useEffect(() => {
        const closeOtherPicker = (event: Event) => {
            if ((event as CustomEvent<string>).detail !== pickerId) setOpen(false);
        };
        window.addEventListener("model-picker-open", closeOtherPicker);
        return () => window.removeEventListener("model-picker-open", closeOtherPicker);
    }, [pickerId]);

    return (
        <Select
            open={open}
            value={selectedValue}
            onOpenChange={(nextOpen) => {
                if (nextOpen && (capability ? !isAiConfigReady(config, capability, current) : !options.length && config.channelMode === "local")) onMissingConfig?.();
                if (nextOpen) window.dispatchEvent(new CustomEvent("model-picker-open", { detail: pickerId }));
                setOpen(nextOpen);
            }}
            onValueChange={(model) => onChange(capability && model === CAPABILITY_DEFAULT_OPTION ? undefined : model)}
        >
            <SelectTrigger
                className={cn(
                    "canvas-composer-model-picker h-8 w-fit max-w-full gap-2 rounded-full border border-input bg-transparent px-3 text-sm font-normal shadow-sm transition-colors",
                    fullWidth ? "w-full min-w-0 justify-start" : "min-w-[9rem] justify-start",
                    "data-[state=open]:border-ring data-[state=open]:ring-2 data-[state=open]:ring-ring/20",
                    className,
                )}
                onMouseDown={(event) => event.stopPropagation()}
                onPointerDown={(event) => event.stopPropagation()}
                title={displayLabel}
            >
                <ModelIcon model={current} />
                <span className="canvas-model-picker-text min-w-0 flex-1 truncate text-left">{displayLabel}</span>
            </SelectTrigger>
            <SelectContent
                data-canvas-no-zoom
                className="z-[1200] w-80 max-w-[calc(100vw-24px)] rounded-xl border border-border/70 bg-popover p-1 shadow-xl"
                position="popper"
                align="start"
                side="bottom"
                sideOffset={6}
                onPointerDown={(event) => event.stopPropagation()}
                onMouseDown={(event) => event.stopPropagation()}
            >
                {options.length ? (
                    options.map((option) => {
                        const model = option === CAPABILITY_DEFAULT_OPTION ? configuredModel : option;
                        return (
                            <SelectItem key={option} value={option} textValue={capability ? modelOptionName(model) : modelOptionLabel(config, model)}>
                                <ModelLabel config={config} model={model} capability={capability} />
                            </SelectItem>
                        );
                    })
                ) : (
                    <SelectItem value="__empty__" disabled>
                        {emptyModelLabel(config, capability)}
                    </SelectItem>
                )}
            </SelectContent>
        </Select>
    );
}

function emptyModelLabel(config: AiConfig, capability?: ModelCapability) {
    const label = capability === "image" ? "生图" : capability === "video" ? "视频" : capability === "text" ? "文本" : capability === "audio" ? "音频" : "";
    if (capability && config.capabilityConfigs?.[capability]?.modelId) return `暂无匹配的${label}模型`;
    return config.models.length ? `暂无匹配的${label}模型` : "请先到配置里添加渠道和模型";
}

function ModelLabel({ config, model, capability }: { config: AiConfig; model: string; capability?: ModelCapability }) {
    return (
        <span className="flex min-w-0 items-center gap-2">
            <ModelIcon model={model} />
            <span className="truncate">{capability ? modelOptionName(model) : modelOptionLabel(config, model)}</span>
        </span>
    );
}

function ModelIcon({ model }: { model: string }) {
    const icon = resolveModelIcon(modelOptionName(model));
    return icon ? <img src={icon} alt="" className="size-4 shrink-0 dark:invert" /> : <Cpu className="size-4 shrink-0 opacity-70" />;
}

function resolveModelIcon(model: string) {
    const name = model.toLowerCase();
    const icon =
        name.includes("claude") || name.includes("anthropic")
            ? "claude"
            : name.includes("gemini") || name.includes("google")
              ? "gemini"
              : name.includes("gpt") || name.includes("openai")
                ? "openai"
                : name.includes("grok")
                  ? "grok"
                  : name.includes("deepseek")
                    ? "deepseek"
                    : name.includes("glm")
                      ? "glm"
                      : "";
    if (icon) return `${import.meta.env.BASE_URL}icons/${icon}.svg`;
    return "";
}
