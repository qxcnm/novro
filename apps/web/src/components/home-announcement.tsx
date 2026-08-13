"use client";

import { Bell } from "lucide-react";
import { useEffect, useState } from "react";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { emptyAnnouncement, normalizeAnnouncement, type Announcement } from "@/lib/announcement";

export function HomeAnnouncement() {
  const [announcement, setAnnouncement] = useState<Announcement>(emptyAnnouncement);

  useEffect(() => {
    const controller = new AbortController();

    async function loadAnnouncement() {
      try {
        const response = await fetch("/api/public/announcement", {
          cache: "no-store",
          signal: controller.signal,
        });
        if (!response.ok) return;
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
