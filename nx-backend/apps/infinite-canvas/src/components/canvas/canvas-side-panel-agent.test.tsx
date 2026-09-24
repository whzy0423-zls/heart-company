import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { CanvasSidePanel } from "./canvas-side-panel";
import { useCanvasSidePanelStore } from "@/stores/use-canvas-side-panel-store";

vi.hoisted(() => {
    Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
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

function renderPanel(props?: Partial<React.ComponentProps<typeof CanvasSidePanel>>) {
    const container = document.createElement("div");
    document.body.appendChild(container);
    const root = createRoot(container);
    const onSendAgentMessage = props?.onSendAgentMessage || vi.fn(async () => undefined);
    act(() => {
        root.render(
            <CanvasSidePanel
                nodes={[]}
                selectedNodeIds={new Set()}
                chatSessions={[]}
                activeChatId={null}
                agentLoading={false}
                onFocusNode={() => undefined}
                onPreviewNode={() => undefined}
                onInsertAsset={() => undefined}
                onCreateAgentSession={() => undefined}
                onSelectAgentSession={() => undefined}
                onDeleteAgentSession={() => undefined}
                onSendAgentMessage={onSendAgentMessage}
                onInsertAgentText={() => undefined}
                {...props}
            />,
        );
    });
    return { container, root, onSendAgentMessage };
}

function cleanup(root: Root, container: HTMLDivElement) {
    act(() => root.unmount());
    container.remove();
    document.body.innerHTML = "";
}

function changeTextarea(textarea: HTMLTextAreaElement, value: string) {
    act(() => {
        const setter = Object.getOwnPropertyDescriptor(Object.getPrototypeOf(textarea), "value")?.set;
        setter?.call(textarea, value);
        textarea.dispatchEvent(new Event("input", { bubbles: true }));
        textarea.dispatchEvent(new Event("change", { bubbles: true }));
    });
}

function click(element: Element) {
    act(() => element.dispatchEvent(new MouseEvent("click", { bubbles: true })));
}

describe("CanvasSidePanel Agent tab", () => {
    beforeEach(() => {
        useCanvasSidePanelStore.setState({ width: 280, panelOpen: true, panelMounted: true, panelClosing: false });
    });

    afterEach(() => {
        document.body.innerHTML = "";
    });

    it("shows Agent as the default side panel concept", () => {
        const { container, root } = renderPanel();

        expect(container.textContent).toContain("Agent");
        expect(container.textContent).toContain("画布 Agent");
        expect(container.textContent).toContain("回复可直接插入画布");

        cleanup(root, container);
    });

    it("sends prompts through the Agent tab", () => {
        const send = vi.fn(async () => undefined);
        const { container, root } = renderPanel({ onSendAgentMessage: send });
        const textarea = container.querySelector("textarea") as HTMLTextAreaElement;
        changeTextarea(textarea, "帮我生成一组提示词");
        click(Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.includes("发送"))!);

        expect(send).toHaveBeenCalledWith("帮我生成一组提示词");

        cleanup(root, container);
    });
});
