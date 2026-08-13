"use client";

import type { ReactNode } from "react";

import { useOptionalCurrentUser } from "@/components/console-shell";

/**
 * DocsFrame 渲染对应的 React 界面组件。
 * @param children React 组件包含的子元素。
 * @param publicFooter 本次操作需要使用的输入参数。
 * @param publicHeader 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
export function DocsFrame({
  children,
  publicFooter,
  publicHeader,
}: {
  children: ReactNode;
  publicFooter: ReactNode;
  publicHeader: ReactNode;
}) {
  const embedded = useOptionalCurrentUser() !== null;

  return (
    <div className={embedded ? "bg-background" : "min-h-screen bg-background"}>
      {embedded ? null : publicHeader}
      <main>
        <div className={embedded
          ? "grid w-full gap-10 py-2 lg:grid-cols-[12rem_minmax(0,1fr)]"
          : "mx-auto grid w-full max-w-7xl gap-12 px-5 py-12 sm:px-8 lg:grid-cols-[13rem_minmax(0,1fr)] lg:px-10 lg:py-16"
        }>
          {children}
        </div>
      </main>
      {embedded ? null : publicFooter}
    </div>
  );
}
