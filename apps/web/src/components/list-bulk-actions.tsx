"use client";

import { ReactNode } from "react";
import { X } from "lucide-react";

import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";

export function ListBulkActions({ children, onClear, selectedCount }: { children: ReactNode; onClear: () => void; selectedCount: number }) {
  if (selectedCount === 0) return null;

  return (
    <div className="flex flex-wrap items-center justify-between gap-2 border-y bg-muted/30 px-3 py-2" role="toolbar" aria-label="批量操作">
      <span className="text-sm font-medium">已选择 {selectedCount} 项</span>
      <div className="flex flex-wrap items-center gap-2">
        {children}
        <Button aria-label="清空选择" onClick={onClear} size="icon-sm" title="清空选择" type="button" variant="ghost"><X /></Button>
      </div>
    </div>
  );
}

type BulkActionDialogProps = {
  busy: boolean;
  confirmLabel: string;
  description: ReactNode;
  destructive?: boolean;
  onConfirm: () => Promise<void>;
  onOpenChange: (open: boolean) => void;
  open: boolean;
  title: string;
};

export function BulkActionDialog({ busy, confirmLabel, description, destructive = false, onConfirm, onOpenChange, open, title }: BulkActionDialogProps) {
  return (
    <AlertDialog onOpenChange={onOpenChange} open={open}>
      <AlertDialogContent>
        <AlertDialogHeader><AlertDialogTitle>{title}</AlertDialogTitle><AlertDialogDescription>{description}</AlertDialogDescription></AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>取消</AlertDialogCancel>
          <AlertDialogAction disabled={busy} onClick={(event) => { event.preventDefault(); void onConfirm(); }} variant={destructive ? "destructive" : "default"}>{busy ? "正在处理..." : confirmLabel}</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
