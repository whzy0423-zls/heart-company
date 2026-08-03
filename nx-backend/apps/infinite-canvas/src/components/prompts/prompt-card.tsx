import { Copy, FileText } from "lucide-react";
import type { ReactNode } from "react";
import { Button, Card, Tag } from "antd";

import { formatPromptDate, type Prompt } from "@/services/api/prompts";

export function PromptCard({
    item,
    onOpen,
    onCopy,
    actionLabel = "复制",
    actionIcon = <Copy className="size-3.5" />,
    actionType = "text",
    extraAction,
}: {
    item: Prompt;
    onOpen: () => void;
    onCopy: () => void;
    actionLabel?: string;
    actionIcon?: ReactNode;
    actionType?: "text" | "primary";
    extraAction?: ReactNode;
}) {
    return (
        <Card
            hoverable
            className="overflow-hidden"
            styles={{ body: { padding: 0 } }}
            cover={
                <button type="button" className="block w-full text-left" onClick={onOpen}>
                    {item.coverUrl ? <img src={item.coverUrl} alt={item.title} className="aspect-[4/3] w-full object-cover" loading="lazy" /> : <span className="grid aspect-[4/3] w-full place-items-center bg-muted text-muted-foreground dark:bg-primary dark:text-muted-foreground"><FileText className="size-8" /></span>}
                </button>
            }
        >
            <button type="button" className="block w-full text-left" onClick={onOpen}>
                <div className="p-4">
                    <div className="flex items-start justify-between gap-3">
                        <h2 className="line-clamp-1 text-sm font-semibold text-foreground dark:text-foreground">{item.title}</h2>
                        <span className="shrink-0 text-xs text-muted-foreground dark:text-muted-foreground">{formatPromptDate(item.updatedAt)}</span>
                    </div>
                    <p className="mt-2 line-clamp-3 text-xs leading-5 text-muted-foreground dark:text-muted-foreground">{item.description || item.prompt}</p>
                    <div className="mt-3 flex flex-wrap gap-1.5">
                        {item.tags.map((tag) => (
                            <Tag key={tag} className="m-0 text-[11px]">
                                {tag}
                            </Tag>
                        ))}
                    </div>
                </div>
            </button>
            <div className="flex items-center gap-2 px-4 pb-4">
                <Button block={actionType === "primary"} type={actionType} size="small" icon={actionIcon} onClick={onCopy}>
                    {actionLabel}
                </Button>
                {extraAction}
            </div>
        </Card>
    );
}
