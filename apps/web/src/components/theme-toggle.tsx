"use client";

import { Moon, Sun } from "lucide-react";
import { useTheme } from "next-themes";
import { useSyncExternalStore } from "react";

import { Button } from "@/components/ui/button";

/**
 * ThemeToggle 渲染对应的 React 界面组件。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
export function ThemeToggle() {
  const { resolvedTheme, setTheme } = useTheme();
  const mounted = useSyncExternalStore(
    () => () => {},
    () => true,
    () => false,
  );

  if (!mounted) {
    return <Button aria-label="切换主题" size="icon" variant="ghost" />;
  }

  const isDark = resolvedTheme === "dark";

  return (
    <Button
      aria-label={isDark ? "切换到浅色主题" : "切换到深色主题"}
      onClick={() => setTheme(isDark ? "light" : "dark")}
      size="icon"
      title={isDark ? "浅色主题" : "深色主题"}
      variant="ghost"
    >
      {isDark ? <Sun /> : <Moon />}
      <span className="sr-only">{isDark ? "浅色主题" : "深色主题"}</span>
    </Button>
  );
}
