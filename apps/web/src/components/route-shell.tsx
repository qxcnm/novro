"use client";

import { usePathname } from "next/navigation";
import type { ReactNode } from "react";

import { ConsoleShell, isConsoleRoute } from "@/components/console-shell";

/**
 * RouteShell 渲染对应的 React 界面组件。
 * @param children React 组件包含的子元素。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
export function RouteShell({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  return isConsoleRoute(pathname) ? <ConsoleShell>{children}</ConsoleShell> : children;
}
