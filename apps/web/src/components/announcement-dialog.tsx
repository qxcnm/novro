"use client";

import { Bell, RefreshCw } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty";
import { Spinner } from "@/components/ui/spinner";
import type { Announcement } from "@/lib/announcement";

type AnnouncementDialogProps = {
  announcement: Announcement;
  error: string;
  loading: boolean;
  onDismissForToday?: () => void;
  onOpenChange: (open: boolean) => void;
  onRetry: () => void;
  open: boolean;
};

/**
 * AnnouncementDialog 渲染对应的 React 界面组件。
 * @param announcement 本次操作需要使用的输入参数。
 * @param error 本次操作需要使用的输入参数。
 * @param loading 本次操作需要使用的输入参数。
 * @param onDismissForToday 本次操作需要使用的输入参数。
 * @param onOpenChange 本次操作需要使用的输入参数。
 * @param onRetry 本次操作需要使用的输入参数。
 * @param open 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
export function AnnouncementDialog({ announcement, error, loading, onDismissForToday, onOpenChange, onRetry, open }: AnnouncementDialogProps) {
  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="flex max-h-[min(720px,calc(100dvh-2rem))] flex-col gap-0 overflow-hidden p-0 sm:top-[10dvh] sm:w-[52vw] sm:max-w-5xl sm:translate-y-0" showCloseButton>
        <DialogHeader className="shrink-0 border-b px-5 py-4 pr-14 sm:px-6">
          <div className="flex items-center gap-2">
            <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
              <Bell aria-hidden="true" className="size-4" />
            </span>
            <div className="min-w-0">
              <DialogTitle>系统公告</DialogTitle>
              <DialogDescription>最新平台更新和通知</DialogDescription>
            </div>
          </div>
        </DialogHeader>

        <div className="min-h-64 flex-1 overflow-y-auto px-5 py-6 sm:min-h-80 sm:px-6">
          {loading ? (
            <div className="flex min-h-56 items-center justify-center" role="status">
              <span className="flex items-center gap-2 text-sm text-muted-foreground"><Spinner />正在加载公告...</span>
            </div>
          ) : error ? (
            <Empty className="min-h-56 border-none p-0">
              <EmptyHeader>
                <EmptyMedia variant="icon"><Bell /></EmptyMedia>
                <EmptyTitle>公告加载失败</EmptyTitle>
                <EmptyDescription>{error}</EmptyDescription>
              </EmptyHeader>
              <EmptyContent><Button onClick={onRetry} type="button" variant="outline"><RefreshCw data-icon="inline-start" />重新加载</Button></EmptyContent>
            </Empty>
          ) : announcement.available ? (
            <article className="mx-auto flex w-full max-w-3xl flex-col gap-5">
              <h2 className="text-lg font-semibold sm:text-xl">{announcement.title}</h2>
              <div className="whitespace-pre-wrap break-words text-sm leading-7 text-foreground/85 sm:text-base sm:leading-8">{announcement.body}</div>
            </article>
          ) : (
            <Empty className="min-h-56 border-none p-0">
              <EmptyHeader>
                <EmptyMedia variant="icon"><Bell /></EmptyMedia>
                <EmptyTitle>目前暂无公告</EmptyTitle>
                <EmptyDescription>有新的平台更新时会显示在这里。</EmptyDescription>
              </EmptyHeader>
            </Empty>
          )}
        </div>

        <DialogFooter className="shrink-0 border-t px-5 py-3 sm:px-6">
          {announcement.available && onDismissForToday ? (
            <Button onClick={onDismissForToday} type="button" variant="secondary">今日关闭</Button>
          ) : null}
          <Button onClick={() => onOpenChange(false)} type="button">关闭公告</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
