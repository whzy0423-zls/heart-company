export type CanvasColorTheme = "light" | "dark";
export type CanvasBackgroundMode = "dots" | "lines" | "blank";

const semanticCanvasTheme = {
    canvas: {
        background: "var(--canvas-background)",
        dot: "var(--canvas-dot)",
        line: "var(--canvas-line)",
        selectionStroke: "var(--primary)",
        selectionFill: "var(--primary-soft)",
    },
    node: {
        label: "var(--muted-foreground)",
        fill: "var(--muted)",
        panel: "var(--card)",
        stroke: "var(--border)",
        activeStroke: "var(--primary)",
        placeholder: "var(--muted-foreground)",
        text: "var(--foreground)",
        muted: "var(--muted-foreground)",
        faint: "var(--canvas-faint)",
    },
    toolbar: {
        panel: "var(--toolbar-panel)",
        border: "var(--border)",
        item: "var(--muted-foreground)",
        itemHover: "var(--accent)",
        activeBg: "var(--primary-soft)",
        activeText: "var(--primary)",
    },
} as const;

export const canvasThemes = {
    light: semanticCanvasTheme,
    dark: semanticCanvasTheme,
} as const;

export type CanvasTheme = (typeof canvasThemes)[CanvasColorTheme];
