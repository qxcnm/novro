"use client";

import { usePathname } from "next/navigation";
import type { ReactNode } from "react";

import { ConsoleShell, isConsoleRoute } from "@/components/console-shell";

export function RouteShell({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  return isConsoleRoute(pathname) ? <ConsoleShell>{children}</ConsoleShell> : children;
}
