"use client";

import { Bell } from "lucide-react";
import { useEffect, useState } from "react";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { emptyAnnouncement, normalizeAnnouncement, type Announcement } from "@/lib/announcement";

/**
 * HomeAnnouncement 渲染对应的 React 界面组件。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
export function HomeAnnouncement() {
  const [announcement, setAnnouncement] = useState<Announcement>(emptyAnnouncement);

  useEffect(() => {
    const controller = new AbortController();

    /**
     * loadAnnouncement 封装该名称对应的业务处理逻辑。
     * @param none 无参数。
     * @author Gao Hongshun
     * @date 2026-08-13
     */
    async function loadAnnouncement() {
      try {
        const response = await fetch("/api/public/announcement", {
          cache: "no-store",
          signal: controller.signal,
        });
        if (!response.ok) return;
        /**
         * body 封装该名称对应的业务处理逻辑。
         * @param await 本次操作需要使用的输入参数。
         * @author Gao Hongshun
         * @date 2026-08-13
         */
        const body = (await response.json()) as { announcement?: Partial<Announcement> };
        setAnnouncement(normalizeAnnouncement(body.announcement));
      } catch {
        // A public announcement is optional; the rest of the homepage remains usable.
      }
    }

    void loadAnnouncement();
    return () => controller.abort();
  }, []);

  if (!announcement.available) return null;

  return (
    <Card className="rounded-lg lg:col-span-2">
      <CardHeader className="border-b">
        <CardTitle className="flex items-center gap-2 text-base">
          <Bell aria-hidden="true" className="size-4" />
          公告
        </CardTitle>
        <CardDescription>{announcement.title}</CardDescription>
      </CardHeader>
      <CardContent>
        <p className="whitespace-pre-wrap break-words text-sm leading-7 text-muted-foreground">
          {announcement.body}
        </p>
      </CardContent>
    </Card>
  );
}
