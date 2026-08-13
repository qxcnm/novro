"use client"

import * as React from "react"
import { Dialog as DialogPrimitive } from "radix-ui"
import { XIcon } from "lucide-react"

import { cn } from "@/lib/utils"
import { Button } from "@/components/ui/button"

/**
 * Dialog 渲染对应的 React 界面组件。
 * @param props React 组件接收的属性。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function Dialog(props: React.ComponentProps<typeof DialogPrimitive.Root>) {
  return <DialogPrimitive.Root data-slot="dialog" {...props} />
}

/**
 * DialogTrigger 渲染对应的 React 界面组件。
 * @param props React 组件接收的属性。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function DialogTrigger(props: React.ComponentProps<typeof DialogPrimitive.Trigger>) {
  return <DialogPrimitive.Trigger data-slot="dialog-trigger" {...props} />
}

/**
 * DialogClose 渲染对应的 React 界面组件。
 * @param props React 组件接收的属性。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function DialogClose(props: React.ComponentProps<typeof DialogPrimitive.Close>) {
  return <DialogPrimitive.Close data-slot="dialog-close" {...props} />
}

/**
 * DialogPortal 渲染对应的 React 界面组件。
 * @param props React 组件接收的属性。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function DialogPortal(props: React.ComponentProps<typeof DialogPrimitive.Portal>) {
  return <DialogPrimitive.Portal data-slot="dialog-portal" {...props} />
}

/**
 * DialogOverlay 渲染对应的 React 界面组件。
 * @param className 用于标识或筛选目标的文本值。
 * @param ...props 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function DialogOverlay({ className, ...props }: React.ComponentProps<typeof DialogPrimitive.Overlay>) {
  return (
    <DialogPrimitive.Overlay
      className={cn("fixed inset-0 z-50 bg-black/20 backdrop-blur-xs data-open:animate-in data-open:fade-in-0 data-closed:animate-out data-closed:fade-out-0", className)}
      data-slot="dialog-overlay"
      {...props}
    />
  )
}

/**
 * DialogContent 渲染对应的 React 界面组件。
 * @param className 用于标识或筛选目标的文本值。
 * @param children React 组件包含的子元素。
 * @param showCloseButton  本次操作需要使用的输入参数。
 * @param ...props 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function DialogContent({ className, children, showCloseButton = true, ...props }: React.ComponentProps<typeof DialogPrimitive.Content> & { showCloseButton?: boolean }) {
  return (
    <DialogPortal>
      <DialogOverlay />
      <DialogPrimitive.Content
        className={cn("fixed left-1/2 top-1/2 z-50 grid w-[calc(100%-2rem)] max-w-lg -translate-x-1/2 -translate-y-1/2 gap-5 rounded-lg border bg-background p-6 shadow-lg data-open:animate-in data-open:fade-in-0 data-open:zoom-in-95 data-closed:animate-out data-closed:fade-out-0 data-closed:zoom-out-95", className)}
        data-slot="dialog-content"
        {...props}
      >
        {children}
        {showCloseButton ? (
          <DialogPrimitive.Close asChild>
            <Button className="absolute right-3 top-3" size="icon-sm" variant="ghost">
              <XIcon />
              <span className="sr-only">关闭</span>
            </Button>
          </DialogPrimitive.Close>
        ) : null}
      </DialogPrimitive.Content>
    </DialogPortal>
  )
}

/**
 * DialogHeader 渲染对应的 React 界面组件。
 * @param className 用于标识或筛选目标的文本值。
 * @param ...props 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function DialogHeader({ className, ...props }: React.ComponentProps<"div">) {
  return <div className={cn("flex flex-col gap-1.5 text-left", className)} data-slot="dialog-header" {...props} />
}

/**
 * DialogFooter 渲染对应的 React 界面组件。
 * @param className 用于标识或筛选目标的文本值。
 * @param ...props 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function DialogFooter({ className, ...props }: React.ComponentProps<"div">) {
  return <div className={cn("flex flex-col-reverse gap-2 sm:flex-row sm:justify-end", className)} data-slot="dialog-footer" {...props} />
}

/**
 * DialogTitle 渲染对应的 React 界面组件。
 * @param className 用于标识或筛选目标的文本值。
 * @param ...props 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function DialogTitle({ className, ...props }: React.ComponentProps<typeof DialogPrimitive.Title>) {
  return <DialogPrimitive.Title className={cn("text-base font-semibold", className)} data-slot="dialog-title" {...props} />
}

/**
 * DialogDescription 渲染对应的 React 界面组件。
 * @param className 用于标识或筛选目标的文本值。
 * @param ...props 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function DialogDescription({ className, ...props }: React.ComponentProps<typeof DialogPrimitive.Description>) {
  return <DialogPrimitive.Description className={cn("text-sm leading-6 text-muted-foreground", className)} data-slot="dialog-description" {...props} />
}

export { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger }
