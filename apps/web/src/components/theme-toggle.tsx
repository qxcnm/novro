"use client";

import { Moon, Sun } from "lucide-react";
import { useTheme } from "next-themes";
import { useSyncExternalStore } from "react";

import { Button } from "@/components/ui/button";

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
