import { Copy, FileText, FolderPlus } from "lucide-react";
import { Button, Modal, Space, Tag } from "antd";

import { formatPromptDate, type Prompt } from "@/services/api/prompts";

export function PromptDetailDialog({ prompt, onClose, onCopy, onSaveAsset }: { prompt: Prompt | null; onClose: () => void; onCopy: (prompt: string) => void; onSaveAsset?: (prompt: Prompt) => void }) {
    return (
        <Modal title={prompt?.title} open={Boolean(prompt)} onCancel={onClose} footer={null} width={720} centered styles={{ body: { height: "calc(85vh - 55px)", overflow: "hidden" } }}>
            {prompt ? (
                <div className="flex h-full min-h-0 flex-col">
                    <div className="shrink-0 space-y-3 pb-4">
                        {prompt.coverUrl ? <img src={prompt.coverUrl} alt={prompt.title} className="h-48 w-full rounded-lg object-cover sm:h-56" /> : <div className="grid h-48 w-full place-items-center rounded-lg bg-muted text-muted-foreground dark:bg-primary dark:text-muted-foreground sm:h-56"><FileText className="size-9" /></div>}
                        {prompt.referenceImageUrls.length > 1 ? <div className="grid grid-cols-6 gap-2">{prompt.referenceImageUrls.filter((url) => url !== prompt.coverUrl).slice(0, 6).map((url) => <img key={url} src={url} alt="" className="aspect-square w-full rounded-md object-cover" loading="lazy" />)}</div> : null}
                    </div>
                    <div className="min-h-0 min-w-0 flex-1 overflow-y-auto border-y border-border py-4 pr-2 dark:border-border">
                        <div className="flex flex-wrap gap-1.5">
                            {prompt.tags.map((tag) => (
                                <Tag key={tag} className="m-0">
                                    {tag}
                                </Tag>
                            ))}
                        </div>
                        {prompt.description ? <p className="mt-4 text-sm leading-6 text-muted-foreground dark:text-muted-foreground">{prompt.description}</p> : null}
                        {prompt.preview ? <pre className="mt-4 whitespace-pre-wrap rounded-lg bg-muted p-3 text-xs leading-5 text-muted-foreground dark:bg-primary dark:text-muted-foreground">{prompt.preview}</pre> : null}
                        <p className="mt-4 whitespace-pre-wrap text-sm leading-7 text-foreground dark:text-muted-foreground">{prompt.prompt}</p>
                        {prompt.createdAt || prompt.updatedAt ? <div className="mt-4 text-xs text-muted-foreground dark:text-muted-foreground">{prompt.createdAt ? `创建：${formatPromptDate(prompt.createdAt)}` : null}{prompt.createdAt && prompt.updatedAt ? " · " : null}{prompt.updatedAt ? `更新：${formatPromptDate(prompt.updatedAt)}` : null}</div> : null}
                    </div>
                    <div className="shrink-0 pt-4">
                        <Space wrap>
                            <Button type="primary" icon={<Copy className="size-4" />} onClick={() => onCopy(prompt.prompt)}>
                                复制提示词
                            </Button>
                            {onSaveAsset ? (
                                <Button icon={<FolderPlus className="size-4" />} onClick={() => onSaveAsset(prompt)}>
                                    加入我的资产
                                </Button>
                            ) : null}
                        </Space>
                    </div>
                </div>
            ) : null}
        </Modal>
    );
}
