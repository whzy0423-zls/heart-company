import type { ReactNode } from "react";
import { useEffect, useMemo, useState } from "react";
import { ProConfigProvider } from "@ant-design/pro-components";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { App, ConfigProvider } from "antd";
import zhCN from "antd/locale/zh_CN";

import { getAntThemeConfig, parseCssRadius, type AntAdminThemeTokens } from "@/lib/app-theme";
import { ADMIN_THEME_APPLIED_EVENT, type AdminThemeSnapshot } from "@/lib/admin-theme-bridge";
import { useThemeStore } from "@/stores/use-theme-store";

const queryClient = new QueryClient({
    defaultOptions: {
        queries: {
            staleTime: 30_000,
            retry: false,
            refetchOnWindowFocus: false,
        },
    },
});

export function AppProviders({ children }: { children: ReactNode }) {
    const theme = useThemeStore((state) => state.theme);
    const setTheme = useThemeStore((state) => state.setTheme);
    const dark = theme === "dark";
    const [adminTokens, setAdminTokens] = useState<AntAdminThemeTokens>(() => readAntTokens());

    useEffect(() => {
        document.documentElement.classList.toggle("dark", dark);
        document.documentElement.style.colorScheme = theme;
    }, [dark, theme]);

    useEffect(() => {
        const onAdminThemeApplied = (event: Event) => {
            const snapshot = (event as CustomEvent<AdminThemeSnapshot>).detail;
            setTheme(snapshot.theme);
            setAdminTokens(readAntTokens());
        };

        document.documentElement.addEventListener(ADMIN_THEME_APPLIED_EVENT, onAdminThemeApplied);
        return () => document.documentElement.removeEventListener(ADMIN_THEME_APPLIED_EVENT, onAdminThemeApplied);
    }, [setTheme]);

    const antTheme = useMemo(() => getAntThemeConfig(dark, adminTokens), [adminTokens, dark]);

    return (
        <ConfigProvider locale={zhCN} theme={antTheme}>
            <ProConfigProvider dark={dark}>
                <App>
                    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
                </App>
            </ProConfigProvider>
        </ConfigProvider>
    );
}

function readAntTokens(): AntAdminThemeTokens {
    const root = document.documentElement;
    const styles = getComputedStyle(root);
    const rootFontSize = Number.parseFloat(styles.fontSize) || 16;
    return {
        primary: styles.getPropertyValue("--primary").trim() || "hsl(212 100% 45%)",
        primaryForeground: styles.getPropertyValue("--primary-foreground").trim() || "hsl(0 0% 98%)",
        radius: parseCssRadius(styles.getPropertyValue("--radius").trim() || "0.5rem", rootFontSize),
    };
}
