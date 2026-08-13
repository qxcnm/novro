"use client";

import * as React from "react";
import { ThemeProvider as NextThemesProvider } from "next-themes";

/**
 * ThemeProvider 渲染对应的 React 界面组件。
 * @param children React 组件包含的子元素。
 * @param ...props 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
export function ThemeProvider({
  children,
  ...props
}: React.ComponentProps<typeof NextThemesProvider>) {
  return <NextThemesProvider {...props}>{children}</NextThemesProvider>;
}
